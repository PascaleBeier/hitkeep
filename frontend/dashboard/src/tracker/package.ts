import { createTracker, isTrackingOptedOut, resolveTrackerEndpoints, setTrackingOptOut } from './core';
import { TRACKER_VERSION } from './version';

import type { EventProperties, HitKeepWindow, TrackerHandle, TrackerRuntimeConfig } from './core';

export type { EventProperties, TrackerHandle } from './core';
export { TRACKER_VERSION } from './version';

export interface TrackerConfig {
    /** HitKeep origin the tracker reports to, e.g. `https://stats.example.com` or `https://example.net/hitkeep`. */
    host: string;
    /** Send a pageview on init. Default `true`. */
    autoCapturePageviews?: boolean;
    /** Track history-based SPA navigations as pageviews. Default `true`. */
    autoTrackSpaNavigation?: boolean;
    /** Track clicks on links to other hostnames as `outbound_click`. Default `true`. */
    outboundLinks?: boolean;
    /** Track same-origin file downloads as `file_download`. Default `true`. */
    fileDownloads?: boolean;
    /** Track form submissions as `form_submit`. Default `true`. */
    formSubmissions?: boolean;
    /** Load the Web Vitals bundle from `${host}/hk-vitals.js`. Default `false`. */
    webVitals?: boolean;
    /** Prefer `navigator.sendBeacon` over keepalive fetch. Default `true`. */
    useBeacon?: boolean;
    /** Drop tracking when the browser sends `DNT: 1`. Default `true`. */
    respectDoNotTrack?: boolean;
    /** Track on `localhost`/`127.0.0.1`, which is blocked by default. Default `false`. */
    captureOnLocalhost?: boolean;
    /** Expose `window.hk.event` for snippet-compatible callers. Default `true`. */
    bindToWindow?: boolean;
    /** Attribution source reported with every request. Default `@hitkeep/tracker`. */
    trackerSource?: string;
}

export interface EcommerceItem {
    item_id?: string;
    item_name?: string;
    quantity?: number;
    price?: number;
}

export interface ItemEventProperties {
    item_id?: string;
    item_name?: string;
    category?: string;
    price?: number;
    currency?: string;
    [key: string]: unknown;
}

export interface PurchaseProperties {
    transaction_id: string;
    value: number;
    currency: string;
    items?: EcommerceItem[];
    items_count?: number;
    coupon?: string;
    tax?: number;
    shipping?: number;
    [key: string]: unknown;
}

const MAX_PRE_INIT_EVENTS = 32;
const DEFAULT_TRACKER_SOURCE = '@hitkeep/tracker';

let activeHandle: TrackerHandle | null = null;
/** True once {@link init} has run, whether or not a tracker started. Reset by {@link cleanup}. */
let initialized = false;
const preInitEvents: { name: string; properties?: EventProperties }[] = [];

function debugLog(message: string): void {
    if (console?.debug) {
        console.debug('[HitKeep]', message);
    }
}

function resolveWindow(win?: Window): HitKeepWindow | null {
    if (win) {
        return win as HitKeepWindow;
    }
    return typeof window === 'undefined' ? null : (window as HitKeepWindow);
}

function resolveRuntimeConfig(config: TrackerConfig): TrackerRuntimeConfig {
    const host = (config.host ?? '').trim();
    if (!host) {
        throw new Error('@hitkeep/tracker: init requires a host, e.g. init({ host: "https://stats.example.com" })');
    }
    const enableWebVitals = config.webVitals === true;
    const endpoints = resolveTrackerEndpoints(new URL(`${host.replace(/\/+$/, '')}/`), enableWebVitals);
    return {
        ...endpoints,
        collectDnt: config.respectDoNotTrack === false,
        disableBeacon: config.useBeacon === false,
        disableSpaTracking: config.autoTrackSpaNavigation === false,
        disableOutboundTracking: config.outboundLinks === false,
        disableDownloadTracking: config.fileDownloads === false,
        disableFormTracking: config.formSubmissions === false,
        enableWebVitals,
        trackerSource: (config.trackerSource || DEFAULT_TRACKER_SOURCE).trim().slice(0, 64) || DEFAULT_TRACKER_SOURCE,
        trackerVersion: TRACKER_VERSION,
        capturePageviews: config.autoCapturePageviews !== false,
        captureOnLocalhost: config.captureOnLocalhost === true,
        bindToWindow: config.bindToWindow !== false
    };
}

/**
 * Stable façade returned by {@link init}. Every entry point routes through
 * `activeHandle`, so the returned object stays correct across cleanup and re-init.
 */
const publicHandle: TrackerHandle = {
    track: (name, properties) => track(name, properties),
    trackPageview: () => trackPageview(),
    cleanup: () => cleanup()
};

export function init(config: TrackerConfig, win?: HitKeepWindow): TrackerHandle {
    if (initialized) {
        debugLog('tracker already initialized; returning the existing handle');
        return publicHandle;
    }

    // Validate before claiming the init slot, so a rejected config can be corrected and retried.
    const runtimeConfig = resolveRuntimeConfig(config);
    initialized = true;

    // Events buffered before init can only ever be delivered by this call, so the
    // queue is drained on every outcome rather than leaking into a later init.
    const queued = preInitEvents.splice(0, preInitEvents.length);

    const targetWindow = resolveWindow(win);
    if (!targetWindow) {
        debugLog('tracker skipped outside a browser environment');
        return publicHandle;
    }

    activeHandle = createTracker(targetWindow, runtimeConfig);
    if (!activeHandle) {
        debugLog('tracker not started (already bootstrapped, blocked, or opted out)');
        return publicHandle;
    }

    for (const event of queued) {
        activeHandle.track(event.name, event.properties);
    }

    return publicHandle;
}

export function track(name: string, properties?: EventProperties): void {
    if (activeHandle) {
        activeHandle.track(name, properties);
        return;
    }
    if (initialized) {
        return;
    }

    preInitEvents.push({ name, properties });
    if (preInitEvents.length > MAX_PRE_INIT_EVENTS) {
        preInitEvents.shift();
    }
}

export function trackPageview(): void {
    activeHandle?.trackPageview();
}

export function cleanup(): void {
    activeHandle?.cleanup();
    activeHandle = null;
    initialized = false;
}

export function blockTrackingForMe(win?: Window): void {
    const targetWindow = resolveWindow(win);
    if (targetWindow) {
        setTrackingOptOut(targetWindow, true);
    }
}

export function enableTrackingForMe(win?: Window): void {
    const targetWindow = resolveWindow(win);
    if (targetWindow) {
        setTrackingOptOut(targetWindow, false);
    }
}

export function isTrackingEnabled(win?: Window): boolean {
    const targetWindow = resolveWindow(win);
    return targetWindow ? !isTrackingOptedOut(targetWindow) : false;
}

export function trackViewItem(properties: ItemEventProperties): void {
    track('view_item', properties);
}

export function trackAddToCart(properties: ItemEventProperties): void {
    track('add_to_cart', properties);
}

export function trackBeginCheckout(properties: ItemEventProperties): void {
    track('begin_checkout', properties);
}

export function trackPurchase(properties: PurchaseProperties): void {
    track('purchase', properties);
}
