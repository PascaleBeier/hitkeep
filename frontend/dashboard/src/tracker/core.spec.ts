import { vi } from 'vitest';

import {
    bootstrapTracker,
    classifyFormSubmit,
    classifyLinkEvent,
    createTracker,
    hasExplicitTrackingTag,
    isTrackerBlocked,
    isTrackingOptedOut,
    readTrackerOptions,
    resolveTrackerUrl,
    resolveWebVitalsBundleUrl,
    sanitizeTrackedUrl,
    setTrackingOptOut
} from './core';
import type { HitKeepWindow, TrackerRuntimeConfig } from './core';

type ListenerMap = Record<string, EventListener[]>;
type TrackerTestWindow = Window &
    typeof globalThis & {
        hk?: {
            event?: (name: string) => void;
            _webVitals?: {
                emit: (payload: Record<string, unknown>) => void;
            };
        };
    };

function okResponse(): Response {
    return new Response('', { status: 202 });
}

function flushPromises(): Promise<void> {
    return Promise.resolve().then(() => undefined);
}

function fetchMockOk() {
    return vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        void input;
        void init;
        return Promise.resolve(okResponse());
    });
}

function trackerHarness(path = '/signup?utm_source=launch&utm_medium=email', scriptSrc = 'https://analytics.example.com/hk.js') {
    const script = document.createElement('script');
    script.src = scriptSrc;
    const appendedScripts: HTMLScriptElement[] = [];

    let currentUrl = new URL(path, 'https://app.example.com');
    const location = {
        get href() {
            return currentUrl.href;
        },
        get hostname() {
            return currentUrl.hostname;
        },
        get pathname() {
            return currentUrl.pathname;
        },
        get search() {
            return currentUrl.search;
        },
        get origin() {
            return currentUrl.origin;
        }
    } as Location;

    const windowListeners: ListenerMap = {};
    const documentListeners: ListenerMap = {};
    let visibilityState = 'visible';
    const stored = new Map<string, string>();
    const sessionStorage = {
        getItem: vi.fn((key: string) => stored.get(key) ?? null),
        setItem: vi.fn((key: string, value: string) => {
            stored.set(key, value);
        })
    };
    const persisted = new Map<string, string>();
    const localStorage = {
        getItem: vi.fn((key: string) => persisted.get(key) ?? null),
        setItem: vi.fn((key: string, value: string) => {
            persisted.set(key, value);
        }),
        removeItem: vi.fn((key: string) => {
            persisted.delete(key);
        })
    };
    const sendBeacon = vi.fn((url: string | URL, data?: BodyInit | null) => {
        void url;
        void data;
        return true;
    });
    const history = {
        pushState: vi.fn((_state: unknown, _title: string, url?: string | URL | null) => {
            if (url) {
                currentUrl = new URL(url, currentUrl.href);
            }
        }),
        replaceState: vi.fn((_state: unknown, _title: string, url?: string | URL | null) => {
            if (url) {
                currentUrl = new URL(url, currentUrl.href);
            }
        })
    };
    const fakeDocument = {
        currentScript: script,
        referrer: 'https://referrer.example.com/article?secret=1',
        get visibilityState() {
            return visibilityState;
        },
        createElement: vi.fn((tagName: string) => document.createElement(tagName)),
        head: {
            appendChild: vi.fn((element: HTMLScriptElement) => {
                appendedScripts.push(element);
                return element;
            })
        },
        querySelector: vi.fn(() => script),
        addEventListener: vi.fn((name: string, listener: EventListener) => {
            documentListeners[name] = [...(documentListeners[name] ?? []), listener];
        }),
        removeEventListener: vi.fn((name: string, listener: EventListener) => {
            documentListeners[name] = (documentListeners[name] ?? []).filter((existing) => existing !== listener);
        })
    };
    const win = {
        document: fakeDocument,
        location,
        navigator: {
            userAgent: 'Mozilla/5.0',
            doNotTrack: '0',
            language: 'en-US',
            sendBeacon
        },
        screen: { width: 1440, height: 900 },
        history,
        sessionStorage,
        localStorage,
        crypto: {
            randomUUID: vi.fn(() => '10000000-0000-4000-8000-000000000001'),
            getRandomValues: crypto.getRandomValues.bind(crypto)
        },
        innerWidth: 1280,
        innerHeight: 720,
        addEventListener: vi.fn((name: string, listener: EventListener) => {
            windowListeners[name] = [...(windowListeners[name] ?? []), listener];
        }),
        removeEventListener: vi.fn((name: string, listener: EventListener) => {
            windowListeners[name] = (windowListeners[name] ?? []).filter((existing) => existing !== listener);
        }),
        hk: undefined
    } as unknown as Window & typeof globalThis;

    return {
        documentListeners,
        appendedScripts,
        fakeDocument,
        history,
        localStorage,
        script,
        sendBeacon,
        sessionStorage,
        setVisibilityState: (next: string) => {
            visibilityState = next;
        },
        win,
        windowListeners
    };
}

describe('tracker core', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    it('classifies outbound links with privacy-safe properties', () => {
        const link = document.createElement('a');
        link.href = 'https://external.example.com/docs/pricing?plan=pro#cta';

        const trackedEvent = classifyLinkEvent(link, new URL('https://app.example.com/pricing'));

        expect(trackedEvent).toEqual({
            name: 'outbound_click',
            properties: {
                target_host: 'external.example.com',
                target_path: '/docs/pricing',
                target_protocol: 'https'
            }
        });
    });

    it('classifies same-origin downloads and strips query strings', () => {
        const link = document.createElement('a');
        link.href = 'https://app.example.com/files/report.csv?token=abc#download';

        const trackedEvent = classifyLinkEvent(link, new URL('https://app.example.com/dashboard'));

        expect(trackedEvent).toEqual({
            name: 'file_download',
            properties: {
                file_host: 'app.example.com',
                file_path: '/files/report.csv',
                file_ext: 'csv'
            }
        });
    });

    it('prefers outbound tracking over download tracking for external files', () => {
        const link = document.createElement('a');
        link.href = 'https://cdn.example.net/files/report.pdf?token=abc';
        link.setAttribute('download', '');

        const trackedEvent = classifyLinkEvent(link, new URL('https://app.example.com/dashboard'));

        expect(trackedEvent?.name).toBe('outbound_click');
    });

    it('does not auto-track links inside forms', () => {
        const form = document.createElement('form');
        const link = document.createElement('a');
        link.href = 'https://external.example.com/docs';
        form.append(link);

        const trackedEvent = classifyLinkEvent(link, new URL('https://app.example.com/contact'));

        expect(trackedEvent).toBeNull();
    });

    it('extracts privacy-safe form submit properties', () => {
        const form = document.createElement('form');
        form.id = 'newsletter-form';
        form.method = 'POST';
        form.action = 'https://forms.example.com/submit?source=landing#done';

        const trackedEvent = classifyFormSubmit(form, new URL('https://app.example.com/blog/post'));

        expect(trackedEvent).toEqual({
            name: 'form_submit',
            properties: {
                action_host: 'forms.example.com',
                action_path: '/submit',
                method: 'post',
                same_origin: false,
                form_id: 'newsletter-form'
            }
        });
    });

    it('defaults form actions to the current page when action is missing', () => {
        const form = document.createElement('form');

        const trackedEvent = classifyFormSubmit(form, new URL('https://app.example.com/signup?step=2#account'));

        expect(trackedEvent).toEqual({
            name: 'form_submit',
            properties: {
                action_host: 'app.example.com',
                action_path: '/signup',
                method: 'get',
                same_origin: true
            }
        });
    });

    it('skips auto-tracking when an explicit tracking tag is present', () => {
        const wrapper = document.createElement('div');
        wrapper.setAttribute('data-hk-event', 'purchase_started');

        const link = document.createElement('a');
        link.href = 'https://external.example.com/docs';
        wrapper.append(link);

        const trackedEvent = classifyLinkEvent(link, new URL('https://app.example.com/docs'));

        expect(hasExplicitTrackingTag(link)).toBe(true);
        expect(trackedEvent).toBeNull();
    });

    it('reads auto-tracking opt-out flags from the snippet element', () => {
        const script = document.createElement('script');
        script.setAttribute('data-collect-dnt', 'true');
        script.setAttribute('data-disable-beacon', 'true');
        script.setAttribute('data-disable-spa-tracking', 'true');
        script.setAttribute('data-disable-outbound-tracking', 'true');
        script.setAttribute('data-disable-download-tracking', 'true');
        script.setAttribute('data-disable-form-tracking', 'true');
        script.setAttribute('data-enable-web-vitals', 'true');

        expect(readTrackerOptions(script)).toEqual({
            collectDnt: true,
            disableBeacon: true,
            disableSpaTracking: true,
            disableOutboundTracking: true,
            disableDownloadTracking: true,
            disableFormTracking: true,
            enableWebVitals: true,
            trackerSource: 'hk.js',
            trackerVersion: ''
        });
    });

    it('resolves tracker-owned URLs from the tracker script directory', () => {
        expect(resolveTrackerUrl(new URL('https://analytics.example.com/hk.js'), 'ingest')).toBe('https://analytics.example.com/ingest');
        expect(resolveTrackerUrl(new URL('https://www.example.net/hitkeep/hk.js'), 'ingest/event')).toBe('https://www.example.net/hitkeep/ingest/event');
        expect(resolveTrackerUrl(new URL('https://www.example.net/tools/hitkeep/hk.js'), 'ingest/event')).toBe('https://www.example.net/tools/hitkeep/ingest/event');
        expect(resolveTrackerUrl(new URL('https://www.example.net/hitkeep/assets/hk.js'), 'ingest/web-vitals')).toBe('https://www.example.net/hitkeep/assets/ingest/web-vitals');
    });

    it('resolves the Web Vitals bundle beside the tracker script', () => {
        expect(resolveWebVitalsBundleUrl(new URL('https://analytics.example.com/hk.js'), 'hk-vitals.js')).toBe('https://analytics.example.com/hk-vitals.js');
        expect(resolveWebVitalsBundleUrl(new URL('https://analytics.example.com/assets/hk.js'), 'hk-vitals.js')).toBe('https://analytics.example.com/assets/hk-vitals.js');
        expect(resolveWebVitalsBundleUrl(new URL('https://www.example.net/hitkeep/hk.js'), 'hk-vitals.js')).toBe('https://www.example.net/hitkeep/hk-vitals.js');
    });

    it('reads tracker source and version metadata from the snippet element', () => {
        const script = document.createElement('script');
        script.setAttribute('data-hitkeep-source', 'wordpress');
        script.setAttribute('data-hitkeep-version', '2.3.0');

        expect(readTrackerOptions(script).trackerSource).toBe('wordpress');
        expect(readTrackerOptions(script).trackerVersion).toBe('2.3.0');
    });

    it('ignores unsupported link protocols', () => {
        const link = document.createElement('a');
        link.href = 'mailto:hello@example.com';

        expect(classifyLinkEvent(link, new URL('https://app.example.com'))).toBeNull();
        expect(sanitizeTrackedUrl('javascript:void(0)', 'https://app.example.com')).toBeNull();
    });

    it('falls back to keepalive fetch when sendBeacon cannot queue the hit', () => {
        const fetchMock = fetchMockOk();
        vi.stubGlobal('fetch', fetchMock);
        const harness = trackerHarness();
        harness.sendBeacon.mockReturnValue(false);

        bootstrapTracker(harness.win);

        expect(harness.sendBeacon).toHaveBeenCalledTimes(1);
        expect(fetchMock).toHaveBeenCalledTimes(1);
        expect(fetchMock.mock.calls[0]?.[0]).toBe('https://analytics.example.com/ingest');
        const fetchInit = fetchMock.mock.calls[0]?.[1];
        expect(fetchInit?.keepalive).toBe(true);
        expect(fetchInit?.credentials).toBe('omit');
    });

    it('does not load the Web Vitals bundle unless the snippet opts in', () => {
        const harness = trackerHarness('/signup');

        bootstrapTracker(harness.win);

        expect(harness.fakeDocument.head.appendChild).not.toHaveBeenCalled();
        expect(harness.appendedScripts.length).toBe(0);
    });

    it('loads the same-origin Web Vitals bundle when the snippet opts in', () => {
        const harness = trackerHarness('/signup');
        harness.script.setAttribute('data-enable-web-vitals', 'true');

        bootstrapTracker(harness.win);

        expect(harness.appendedScripts.length).toBe(1);
        expect(harness.appendedScripts[0]?.src).toBe('https://analytics.example.com/hk-vitals.js');
        expect(harness.appendedScripts[0]?.async).toBe(true);
    });

    it('posts tracker requests under the script subdirectory', () => {
        const harness = trackerHarness('/signup', 'https://www.example.net/hitkeep/hk.js');
        harness.script.setAttribute('data-enable-web-vitals', 'true');

        bootstrapTracker(harness.win);
        const win = harness.win as TrackerTestWindow;
        win.hk?.event?.('signup_clicked');
        win.hk?._webVitals?.emit({
            n: 'LCP',
            v: 1200,
            p: '/signup',
            sid: 'session-id',
            pid: 'page-id',
            tsrc: 'hk.js',
            tv: ''
        });

        expect(harness.sendBeacon.mock.calls.map(([url]) => url)).toEqual(['https://www.example.net/hitkeep/ingest', 'https://www.example.net/hitkeep/ingest/event', 'https://www.example.net/hitkeep/ingest/web-vitals']);
        expect(harness.appendedScripts[0]?.src).toBe('https://www.example.net/hitkeep/hk-vitals.js');
    });

    it('adds transient path and user-agent context to browser events', async () => {
        const harness = trackerHarness('/signup?plan=pro');

        bootstrapTracker(harness.win);
        const win = harness.win as TrackerTestWindow;
        win.hk?.event?.('signup_clicked');

        const eventBody = harness.sendBeacon.mock.calls[1]?.[1] as unknown as Blob;
        const payload = JSON.parse(await eventBody.text()) as Record<string, unknown>;
        expect(payload['path']).toBe('/signup');
        expect(payload['ua']).toBe('Mozilla/5.0');
    });

    it('queues failed requests in memory and flushes them on pagehide', async () => {
        const fetchMock = fetchMockOk();
        fetchMock.mockRejectedValueOnce(new Error('offline'));
        vi.stubGlobal('fetch', fetchMock);
        const harness = trackerHarness();
        harness.script.setAttribute('data-disable-beacon', 'true');

        bootstrapTracker(harness.win);
        await flushPromises();

        expect(fetchMock).toHaveBeenCalledTimes(1);
        harness.windowListeners['pagehide']?.[0]?.(new Event('pagehide'));
        await flushPromises();

        expect(fetchMock).toHaveBeenCalledTimes(2);
    });

    it('flushes queued requests when the document becomes hidden', async () => {
        const fetchMock = fetchMockOk();
        fetchMock.mockRejectedValueOnce(new Error('offline'));
        vi.stubGlobal('fetch', fetchMock);
        const harness = trackerHarness();
        harness.script.setAttribute('data-disable-beacon', 'true');

        bootstrapTracker(harness.win);
        await flushPromises();

        harness.setVisibilityState('hidden');
        harness.documentListeners['visibilitychange']?.[0]?.(new Event('visibilitychange'));
        await flushPromises();

        expect(fetchMock).toHaveBeenCalledTimes(2);
    });

    it('keeps the in-memory retry queue bounded', async () => {
        const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
            void input;
            void init;
            return Promise.reject(new Error('offline'));
        });
        vi.stubGlobal('fetch', fetchMock);
        const harness = trackerHarness();
        harness.script.setAttribute('data-disable-beacon', 'true');

        bootstrapTracker(harness.win);
        for (let index = 0; index < 12; index += 1) {
            (harness.win as Window & typeof globalThis & { hk?: { event?: (name: string) => void } }).hk?.event?.(`queued_${index}`);
        }
        await flushPromises();

        expect(fetchMock).toHaveBeenCalledTimes(13);
        fetchMock.mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
            void input;
            void init;
            return Promise.resolve(okResponse());
        });
        harness.windowListeners['pagehide']?.[0]?.(new Event('pagehide'));
        await flushPromises();

        expect(fetchMock).toHaveBeenCalledTimes(23);
    });

    it('suppresses duplicate pageviews for the same path in a short window', () => {
        const harness = trackerHarness('/signup');

        bootstrapTracker(harness.win);
        harness.history.pushState({}, '', '/signup');

        expect(harness.sendBeacon).toHaveBeenCalledTimes(1);
    });

    it('does not bootstrap twice', () => {
        const harness = trackerHarness('/signup');

        bootstrapTracker(harness.win);
        bootstrapTracker(harness.win);

        expect(harness.sendBeacon).toHaveBeenCalledTimes(1);
        expect(harness.windowListeners['pagehide']?.length).toBe(1);
    });

    it('keeps attribution in memory and limits sessionStorage to the session tuple', () => {
        const harness = trackerHarness('/signup?utm_source=launch&utm_medium=email&hk_qr=qr-123');

        bootstrapTracker(harness.win);
        harness.history.pushState({}, '', '/dashboard?utm_source=changed&utm_medium=social');

        const secondBeaconBody = harness.sendBeacon.mock.calls[1]?.[1] as unknown as Blob;

        expect(harness.sessionStorage.setItem.mock.calls.every(([key]) => key === 'hk_session')).toBe(true);
        expect(harness.sessionStorage.setItem.mock.calls[0]?.[1]).toMatch(/^10000000-0000-4000-8000-000000000001\|\d+$/);
        return secondBeaconBody.text().then((body) => {
            const payload = JSON.parse(body) as Record<string, unknown>;
            expect(payload['u_src']).toBe('launch');
            expect(payload['u_med']).toBe('email');
            expect(payload['qr']).toBe('qr-123');
        });
    });

    function runtimeConfig(overrides: Partial<TrackerRuntimeConfig> = {}): TrackerRuntimeConfig {
        return {
            collectDnt: false,
            disableBeacon: false,
            disableSpaTracking: false,
            disableOutboundTracking: false,
            disableDownloadTracking: false,
            disableFormTracking: false,
            enableWebVitals: false,
            trackerSource: '@hitkeep/tracker',
            trackerVersion: '2.12.0',
            pageEndpoint: 'https://stats.example.com/ingest',
            eventEndpoint: 'https://stats.example.com/ingest/event',
            webVitalsEndpoint: 'https://stats.example.com/ingest/web-vitals',
            webVitalsBundleUrl: 'https://stats.example.com/hk-vitals.js',
            capturePageviews: true,
            captureOnLocalhost: false,
            bindToWindow: true,
            ...overrides
        };
    }

    it('creates a tracker from explicit endpoints without a script element', async () => {
        const harness = trackerHarness('/signup');
        harness.fakeDocument.currentScript = null as unknown as HTMLScriptElement;

        const handle = createTracker(harness.win as HitKeepWindow, runtimeConfig());

        expect(handle).not.toBeNull();
        expect(harness.sendBeacon).toHaveBeenCalledTimes(1);
        expect(harness.sendBeacon.mock.calls[0]?.[0]).toBe('https://stats.example.com/ingest');

        handle?.track('signup_clicked', { plan: 'pro' });
        expect(harness.sendBeacon.mock.calls[1]?.[0]).toBe('https://stats.example.com/ingest/event');
        const eventBody = harness.sendBeacon.mock.calls[1]?.[1] as unknown as Blob;
        const payload = JSON.parse(await eventBody.text()) as Record<string, unknown>;
        expect(payload['tsrc']).toBe('@hitkeep/tracker');
        expect(payload['tv']).toBe('2.12.0');
    });

    it('skips the initial pageview when pageview capture is disabled', () => {
        const harness = trackerHarness('/signup');

        const handle = createTracker(harness.win as HitKeepWindow, runtimeConfig({ capturePageviews: false }));

        expect(harness.sendBeacon).not.toHaveBeenCalled();
        handle?.trackPageview();
        expect(harness.sendBeacon).toHaveBeenCalledTimes(1);
    });

    it('blocks localhost unless capture on localhost is enabled', () => {
        expect(isTrackerBlocked('localhost', 'Mozilla/5.0', '0', false)).toBe(true);
        expect(isTrackerBlocked('localhost', 'Mozilla/5.0', '0', false, true)).toBe(false);
        expect(isTrackerBlocked('127.0.0.1', 'Mozilla/5.0', '0', false, true)).toBe(false);
        expect(isTrackerBlocked('localhost', 'Googlebot', '0', false, true)).toBe(true);
    });

    it('honors the persistent opt-out flag', () => {
        const harness = trackerHarness('/signup');
        setTrackingOptOut(harness.win, true);

        expect(isTrackingOptedOut(harness.win)).toBe(true);
        const handle = createTracker(harness.win as HitKeepWindow, runtimeConfig());

        expect(handle).toBeNull();
        expect(harness.sendBeacon).not.toHaveBeenCalled();
        const win = harness.win as TrackerTestWindow;
        win.hk?.event?.('ignored_event');
        expect(harness.sendBeacon).not.toHaveBeenCalled();

        setTrackingOptOut(harness.win, false);
        expect(isTrackingOptedOut(harness.win)).toBe(false);
    });

    it('cleans up listeners, history patches, and the bootstrap guard', () => {
        const harness = trackerHarness('/signup');
        const originalPushState = harness.history.pushState;
        const originalReplaceState = harness.history.replaceState;

        const handle = createTracker(harness.win as HitKeepWindow, runtimeConfig());
        expect(harness.history.pushState).not.toBe(originalPushState);
        expect(harness.windowListeners['pagehide']?.length).toBe(1);

        handle?.cleanup();

        expect(harness.history.pushState).toBe(originalPushState);
        expect(harness.history.replaceState).toBe(originalReplaceState);
        expect(harness.windowListeners['pagehide']?.length).toBe(0);
        expect(harness.windowListeners['popstate']?.length).toBe(0);
        expect(harness.documentListeners['visibilitychange']?.length).toBe(0);
        expect(harness.documentListeners['click']?.length).toBe(0);
        expect(harness.documentListeners['submit']?.length).toBe(0);
        expect((harness.win as TrackerTestWindow).hk?.event).toBeUndefined();

        const beaconCallsAfterCleanup = harness.sendBeacon.mock.calls.length;
        handle?.track('after_cleanup');
        handle?.trackPageview();
        expect(harness.sendBeacon.mock.calls.length).toBe(beaconCallsAfterCleanup);
    });

    it('supports re-initialization after cleanup', () => {
        const harness = trackerHarness('/signup');

        const first = createTracker(harness.win as HitKeepWindow, runtimeConfig());
        expect(createTracker(harness.win as HitKeepWindow, runtimeConfig())).toBeNull();
        first?.cleanup();

        const second = createTracker(harness.win as HitKeepWindow, runtimeConfig());
        expect(second).not.toBeNull();
        expect(harness.windowListeners['pagehide']?.length).toBe(1);
        second?.cleanup();
    });
});
