import { onCLS, onFCP, onINP, onLCP, onTTFB, type Metric } from 'web-vitals';

interface WebVitalsPayload {
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

interface WebVitalsTrackerContext {
    emit: (payload: WebVitalsPayload) => void;
    getPath: () => string;
    sessionId: string;
    pageId: () => string;
    trackerSource: string;
    trackerVersion: string;
    userAgent: string;
}

type HitKeepVitalsWindow = Window &
    typeof globalThis & {
        hk?: {
            _webVitals?: WebVitalsTrackerContext;
        };
    };

function navigationType(metric: Metric): string | undefined {
    const value = metric.navigationType;
    return typeof value === 'string' && value.length > 0 ? value : undefined;
}

function emitMetric(context: WebVitalsTrackerContext, metric: Metric): void {
    context.emit({
        n: metric.name,
        v: metric.value,
        p: context.getPath(),
        nt: navigationType(metric),
        mid: metric.id,
        sid: context.sessionId,
        pid: context.pageId(),
        tsrc: context.trackerSource,
        tv: context.trackerVersion,
        ua: context.userAgent
    });
}

export function bootstrapWebVitals(win: HitKeepVitalsWindow = window): void {
    const context = win.hk?._webVitals;
    if (!context) {
        return;
    }

    const report = (metric: Metric) => emitMetric(context, metric);
    onCLS(report);
    onFCP(report);
    onINP(report);
    onLCP(report);
    onTTFB(report);
}

(() => {
    try {
        bootstrapWebVitals();
    } catch {
        // Keep the optional bundle silent. The main tracker must remain best-effort.
    }
})();
