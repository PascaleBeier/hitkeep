type EventProperties = Record<string, unknown>;

interface HitKeepEvent {
    name: string;
    properties: EventProperties;
}

type EventSender = (name: string, properties?: EventProperties) => void;

type HistoryMethod = 'pushState' | 'replaceState';

export interface TrackerOptions {
    collectDnt: boolean;
    disableBeacon: boolean;
    disableSpaTracking: boolean;
    disableOutboundTracking: boolean;
    disableDownloadTracking: boolean;
    disableFormTracking: boolean;
    enableWebVitals: boolean;
    trackerSource: string;
    trackerVersion: string;
}

interface PendingRequest {
    endpoint: string;
    body: string;
}

type HitKeepWindow = Window &
    typeof globalThis & {
        hk?: {
            event?: EventSender;
            _bootstrapped?: boolean;
            _webVitals?: WebVitalsTrackerContext;
            _webVitalsLoaded?: boolean;
        };
    };

export interface WebVitalsPayload {
    n: string;
    v: number;
    p: string;
    nt?: string;
    mid?: string;
    sid: string;
    pid: string;
    tsrc: string;
    tv: string;
    ua?: string;
}

export interface WebVitalsTrackerContext {
    emit: (payload: WebVitalsPayload) => void;
    getPath: () => string;
    sessionId: string;
    pageId: () => string;
    trackerSource: string;
    trackerVersion: string;
    userAgent: string;
}

const SESSION_KEY = 'hk_session';
const SESSION_EXPIRY = 30 * 60 * 1000;
const MAX_PENDING_REQUESTS = 10;
const RETRY_DELAY_MS = 2000;
const DUPLICATE_PAGEVIEW_WINDOW_MS = 1500;
const DOWNLOAD_EXTENSIONS = new Set([
    '7z',
    'avi',
    'csv',
    'doc',
    'docx',
    'epub',
    'gz',
    'ics',
    'jpeg',
    'jpg',
    'json',
    'key',
    'mov',
    'mp3',
    'mp4',
    'pdf',
    'png',
    'ppt',
    'pptx',
    'rar',
    'rtf',
    'svg',
    'tar',
    'tgz',
    'txt',
    'webp',
    'xls',
    'xlsx',
    'xml',
    'zip'
]);
const EXPLICIT_TRACKING_SELECTORS = ['[data-hk-event]', '[data-hitkeep-event]'].join(', ');
const noop = () => undefined;
const TRACKER_SOURCE = 'hk.js';

function ignoreError(error?: unknown): void {
    // Best-effort tracker operations should fail closed without noisy console output.
    void error;
}

export function readTrackerOptions(scriptEl: Element): TrackerOptions {
    const source = (scriptEl.getAttribute('data-hitkeep-source') || TRACKER_SOURCE).trim().slice(0, 64) || TRACKER_SOURCE;
    const version = (scriptEl.getAttribute('data-hitkeep-version') || '').trim().slice(0, 64);
    return {
        collectDnt: scriptEl.getAttribute('data-collect-dnt') === 'true',
        disableBeacon: scriptEl.getAttribute('data-disable-beacon') === 'true',
        disableSpaTracking: scriptEl.getAttribute('data-disable-spa-tracking') === 'true',
        disableOutboundTracking: scriptEl.getAttribute('data-disable-outbound-tracking') === 'true',
        disableDownloadTracking: scriptEl.getAttribute('data-disable-download-tracking') === 'true',
        disableFormTracking: scriptEl.getAttribute('data-disable-form-tracking') === 'true',
        enableWebVitals: scriptEl.getAttribute('data-enable-web-vitals') === 'true',
        trackerSource: source,
        trackerVersion: version
    };
}

export function isTrackerBlocked(hostname: string, userAgent: string, doNotTrack: string | null | undefined, collectDnt: boolean): boolean {
    const isBot = /bot|spider|crawl|slurp|ia_archiver/i.test(userAgent);
    const isLocal = hostname === 'localhost' || hostname === '127.0.0.1';
    const dntEnabled = doNotTrack === '1';
    return isLocal || isBot || (dntEnabled && !collectDnt);
}

export function sanitizeTrackedUrl(rawUrl: string, baseUrl: string | URL): URL | null {
    try {
        const url = new URL(rawUrl, baseUrl);
        if (url.protocol !== 'http:' && url.protocol !== 'https:') {
            return null;
        }
        url.search = '';
        url.hash = '';
        return url;
    } catch {
        return null;
    }
}

export function sanitizeTrackedPath(url: URL): string {
    return url.pathname || '/';
}

export function resolveTrackerUrl(scriptUrl: URL, relativePath: string): string {
    const scriptDirectory = new URL('.', scriptUrl);
    const normalizedPath = relativePath.replace(/^\/+/, '');
    return new URL(normalizedPath, scriptDirectory).toString();
}

export function resolveWebVitalsBundleUrl(scriptUrl: URL, bundleName = 'hk-vitals.js'): string {
    return resolveTrackerUrl(scriptUrl, bundleName);
}

function loadWebVitalsBundle(win: HitKeepWindow, scriptUrl: URL): void {
    if (win.hk?._webVitalsLoaded) {
        return;
    }
    const script = win.document.createElement('script');
    script.async = true;
    script.src = resolveWebVitalsBundleUrl(scriptUrl);
    win.hk = win.hk || {};
    win.hk._webVitalsLoaded = true;
    win.document.head.appendChild(script);
}

export function extractDownloadExtension(url: URL): string | null {
    const lastSegment = sanitizeTrackedPath(url).split('/').pop() ?? '';
    const parts = lastSegment.split('.');
    if (parts.length < 2) {
        return null;
    }

    const extension = parts[parts.length - 1]?.toLowerCase() ?? '';
    return DOWNLOAD_EXTENSIONS.has(extension) ? extension : null;
}

export function hasExplicitTrackingTag(element: Element | null): boolean {
    for (let current = element; current; current = current.parentElement) {
        if (current.matches(EXPLICIT_TRACKING_SELECTORS)) {
            return true;
        }
    }
    return false;
}

export function classifyLinkEvent(link: HTMLAnchorElement | HTMLAreaElement, currentUrl: URL): HitKeepEvent | null {
    if (link.closest('form') || hasExplicitTrackingTag(link)) {
        return null;
    }

    const href = link.getAttribute('href');
    if (!href) {
        return null;
    }

    const targetUrl = sanitizeTrackedUrl(href, currentUrl);
    if (!targetUrl) {
        return null;
    }

    if (targetUrl.hostname !== currentUrl.hostname) {
        return {
            name: 'outbound_click',
            properties: {
                target_host: targetUrl.hostname,
                target_path: sanitizeTrackedPath(targetUrl),
                target_protocol: targetUrl.protocol.replace(':', '')
            }
        };
    }

    const fileExtension = link.hasAttribute('download') ? (extractDownloadExtension(targetUrl) ?? 'unknown') : extractDownloadExtension(targetUrl);
    if (!fileExtension) {
        return null;
    }

    return {
        name: 'file_download',
        properties: {
            file_host: targetUrl.hostname,
            file_path: sanitizeTrackedPath(targetUrl),
            file_ext: fileExtension
        }
    };
}

export function classifyFormSubmit(form: HTMLFormElement, currentUrl: URL, submitter?: Element | null): HitKeepEvent | null {
    if (hasExplicitTrackingTag(submitter ?? null) || hasExplicitTrackingTag(form)) {
        return null;
    }

    const rawAction = form.getAttribute('action')?.trim() || currentUrl.toString();
    const actionUrl = sanitizeTrackedUrl(rawAction, currentUrl);
    if (!actionUrl) {
        return null;
    }

    const method = (form.getAttribute('method')?.trim() || 'get').toLowerCase();
    const formId = form.getAttribute('id')?.trim();
    const properties: EventProperties = {
        action_host: actionUrl.hostname,
        action_path: sanitizeTrackedPath(actionUrl),
        method,
        same_origin: actionUrl.origin === currentUrl.origin
    };

    if (formId) {
        properties['form_id'] = formId;
    }

    return {
        name: 'form_submit',
        properties
    };
}

export function bootstrapTracker(win: HitKeepWindow = window): void {
    const { document, location, navigator, screen, history, sessionStorage, crypto } = win;

    const scriptEl = resolveTrackerScript(document);
    if (!scriptEl) {
        return;
    }

    win.hk = win.hk || {};
    if (win.hk._bootstrapped) {
        return;
    }
    win.hk._bootstrapped = true;

    const options = readTrackerOptions(scriptEl);
    if (isTrackerBlocked(location.hostname, navigator.userAgent, navigator.doNotTrack, options.collectDnt)) {
        win.hk.event = noop;
        return;
    }

    const scriptUrl = new URL(scriptEl.src, location.href);
    const pageEndpoint = resolveTrackerUrl(scriptUrl, 'ingest');
    const eventEndpoint = resolveTrackerUrl(scriptUrl, 'ingest/event');
    const webVitalsEndpoint = resolveTrackerUrl(scriptUrl, 'ingest/web-vitals');
    const generateUUID = () => {
        if (crypto?.randomUUID) {
            return crypto.randomUUID();
        }
        return '10000000-1000-4000-8000-100000000000'.replace(/[018]/g, (value) => (Number(value) ^ (crypto.getRandomValues(new Uint8Array(1))[0]! & (15 >> (Number(value) / 4)))).toString(16));
    };

    const getSessionId = () => {
        const now = Date.now();
        let sessionId: string | null = null;

        try {
            const stored = sessionStorage.getItem(SESSION_KEY);
            if (stored) {
                const [id, lastActive] = stored.split('|');
                if (id && lastActive && now - parseInt(lastActive, 10) < SESSION_EXPIRY) {
                    sessionId = id;
                }
            }
        } catch (error) {
            ignoreError(error);
        }

        if (!sessionId) {
            sessionId = generateUUID();
        }

        try {
            sessionStorage.setItem(SESSION_KEY, `${sessionId}|${now}`);
        } catch (error) {
            ignoreError(error);
        }

        return sessionId;
    };

    const sessionId = getSessionId();
    const initialReferrer = document.referrer;
    const initialHost = location.hostname;
    const readUtmValue = (params: URLSearchParams, key: string) => {
        const value = params.get(key);
        if (!value) {
            return null;
        }
        const trimmed = value.trim();
        return trimmed.length > 0 ? trimmed : null;
    };
    const initialSearchParams = new URLSearchParams(location.search);
    const initialAttribution = {
        u_src: readUtmValue(initialSearchParams, 'utm_source'),
        u_med: readUtmValue(initialSearchParams, 'utm_medium'),
        u_cmp: readUtmValue(initialSearchParams, 'utm_campaign'),
        u_trm: readUtmValue(initialSearchParams, 'utm_term'),
        u_cnt: readUtmValue(initialSearchParams, 'utm_content'),
        qr: readUtmValue(initialSearchParams, 'hk_qr')
    };
    const pendingRequests: PendingRequest[] = [];
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let lastPath = location.pathname;
    let lastPageviewPath = '';
    let lastPageviewAt = 0;
    let currentPageId = generateUUID();

    const queueRequest = (request: PendingRequest) => {
        pendingRequests.push(request);
        if (pendingRequests.length > MAX_PENDING_REQUESTS) {
            pendingRequests.splice(0, pendingRequests.length - MAX_PENDING_REQUESTS);
        }
        scheduleFlush();
    };

    const flushQueue = () => {
        if (pendingRequests.length === 0) {
            return;
        }
        if (retryTimer !== null) {
            clearTimeout(retryTimer);
            retryTimer = null;
        }

        const requests = pendingRequests.splice(0, pendingRequests.length);
        for (const request of requests) {
            sendRequest(request, true);
        }
    };

    const scheduleFlush = () => {
        if (retryTimer !== null || pendingRequests.length === 0) {
            return;
        }

        retryTimer = setTimeout(() => {
            retryTimer = null;
            flushQueue();
        }, RETRY_DELAY_MS);
    };

    const sendRequest = (request: PendingRequest, fromQueue = false) => {
        const headers = { 'Content-Type': 'application/json' };

        if (navigator.sendBeacon && !options.disableBeacon) {
            const blob = new Blob([request.body], { type: 'application/json' });
            if (navigator.sendBeacon(request.endpoint, blob)) {
                return;
            }
        }

        fetch(request.endpoint, {
            method: 'POST',
            body: request.body,
            headers,
            keepalive: true,
            credentials: 'omit'
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error(`tracker_request_failed_${response.status}`);
                }
            })
            .catch((error) => {
                ignoreError(error);
                if (!fromQueue) {
                    queueRequest(request);
                } else {
                    pendingRequests.unshift(request);
                    if (pendingRequests.length > MAX_PENDING_REQUESTS) {
                        pendingRequests.length = MAX_PENDING_REQUESTS;
                    }
                    scheduleFlush();
                }
            });
    };

    const sendJson = (endpoint: string, payload: object) => {
        sendRequest({
            endpoint,
            body: JSON.stringify(payload)
        });
    };

    const currentReferrer = () => {
        if (lastPath !== location.pathname) {
            return `${location.origin}${lastPath}`;
        }
        return initialReferrer || null;
    };

    const emitEvent = (name: string, properties: EventProperties = {}) => {
        sendJson(eventEndpoint, {
            n: name,
            p: properties,
            r: currentReferrer(),
            sid: sessionId,
            path: sanitizeTrackedPath(new URL(location.href)),
            ua: navigator.userAgent,
            tsrc: options.trackerSource,
            tv: options.trackerVersion
        });
    };

    if (options.enableWebVitals) {
        win.hk._webVitals = {
            emit: (payload) => sendJson(webVitalsEndpoint, payload),
            getPath: () => sanitizeTrackedPath(new URL(location.href)),
            sessionId,
            pageId: () => currentPageId,
            trackerSource: options.trackerSource,
            trackerVersion: options.trackerVersion,
            userAgent: navigator.userAgent
        };
        loadWebVitalsBundle(win, scriptUrl);
    }

    const sendPageView = () => {
        const currentPath = location.pathname;
        const now = Date.now();
        currentPageId = generateUUID();

        try {
            sessionStorage.setItem(SESSION_KEY, `${sessionId}|${now}`);
        } catch (error) {
            ignoreError(error);
        }

        if (currentPath === lastPageviewPath && now - lastPageviewAt < DUPLICATE_PAGEVIEW_WINDOW_MS) {
            lastPath = currentPath;
            return;
        }

        const referrer = currentReferrer();
        const isUnique = lastPath === currentPath && referrer ? new URL(referrer, location.href).hostname !== initialHost : false;

        sendJson(pageEndpoint, {
            path: currentPath,
            referrer: referrer || null,
            ua: navigator.userAgent,
            vp_w: win.innerWidth,
            vp_h: win.innerHeight,
            sc_w: screen.width,
            sc_h: screen.height,
            lang: navigator.language,
            ...initialAttribution,
            unique: Boolean(isUnique),
            session_id: sessionId,
            page_id: currentPageId,
            tsrc: options.trackerSource,
            tv: options.trackerVersion
        });

        lastPageviewPath = currentPath;
        lastPageviewAt = now;
        lastPath = currentPath;
    };

    const patchHistory = (method: HistoryMethod) => {
        const original = history[method];
        return function patchedHistory(this: History, ...args: Parameters<History[HistoryMethod]>) {
            original.apply(this, args);
            sendPageView();
        };
    };

    if (!options.disableSpaTracking) {
        history.pushState = patchHistory('pushState');
        history.replaceState = patchHistory('replaceState');

        win.addEventListener('popstate', sendPageView);
        win.addEventListener('hashchange', sendPageView);
    }

    win.addEventListener('online', flushQueue);
    win.addEventListener('pagehide', flushQueue);
    document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') {
            flushQueue();
        }
    });

    if ((document.visibilityState as string) === 'prerender') {
        document.addEventListener(
            'visibilitychange',
            () => {
                if (document.visibilityState === 'visible') {
                    sendPageView();
                }
            },
            { once: true }
        );
    } else {
        sendPageView();
    }

    bindAutoTracking(document, () => new URL(location.href), options, emitEvent);

    win.hk.event = (name, properties) => emitEvent(name, properties ?? {});
}

function resolveTrackerScript(document: Document): HTMLScriptElement | null {
    const currentScript = document.currentScript;
    if (currentScript instanceof HTMLScriptElement) {
        return currentScript;
    }

    const script = document.querySelector('script[src*="hk.js"]');
    return script instanceof HTMLScriptElement ? script : null;
}

function bindAutoTracking(document: Document, getCurrentUrl: () => URL, options: TrackerOptions, emitEvent: EventSender): void {
    if (!options.disableOutboundTracking || !options.disableDownloadTracking) {
        const handleLinkInteraction = (event: MouseEvent) => {
            if ((event.type === 'click' && event.button !== 0) || (event.type === 'auxclick' && event.button !== 1)) {
                return;
            }

            const target = event.target;
            if (!(target instanceof Element)) {
                return;
            }

            const link = target.closest('a[href], area[href]');
            if (!(link instanceof HTMLAnchorElement || link instanceof HTMLAreaElement)) {
                return;
            }

            const trackedEvent = classifyLinkEvent(link, getCurrentUrl());
            if (!trackedEvent) {
                return;
            }

            if (trackedEvent.name === 'outbound_click' && options.disableOutboundTracking) {
                return;
            }

            if (trackedEvent.name === 'file_download' && options.disableDownloadTracking) {
                return;
            }

            emitEvent(trackedEvent.name, trackedEvent.properties);
        };

        document.addEventListener('click', handleLinkInteraction, true);
        document.addEventListener('auxclick', handleLinkInteraction, true);
    }

    if (!options.disableFormTracking) {
        document.addEventListener(
            'submit',
            (event) => {
                const target = event.target;
                if (!(target instanceof HTMLFormElement)) {
                    return;
                }

                const submitter = event instanceof SubmitEvent && event.submitter instanceof Element ? event.submitter : null;
                const trackedEvent = classifyFormSubmit(target, getCurrentUrl(), submitter);
                if (!trackedEvent) {
                    return;
                }

                emitEvent(trackedEvent.name, trackedEvent.properties);
            },
            true
        );
    }
}
