import { describe, expect, it, vi } from 'vitest';

import type { Metric } from 'web-vitals';

const webVitalsMock = vi.hoisted(() => {
    const callbacks: Record<string, ((metric: Metric) => void)[]> = {
        CLS: [],
        FCP: [],
        INP: [],
        LCP: [],
        TTFB: []
    };

    return {
        callbacks,
        onCLS: vi.fn((callback: (metric: Metric) => void) => callbacks['CLS'].push(callback)),
        onFCP: vi.fn((callback: (metric: Metric) => void) => callbacks['FCP'].push(callback)),
        onINP: vi.fn((callback: (metric: Metric) => void) => callbacks['INP'].push(callback)),
        onLCP: vi.fn((callback: (metric: Metric) => void) => callbacks['LCP'].push(callback)),
        onTTFB: vi.fn((callback: (metric: Metric) => void) => callbacks['TTFB'].push(callback))
    };
});

vi.mock('web-vitals', () => ({
    onCLS: webVitalsMock.onCLS,
    onFCP: webVitalsMock.onFCP,
    onINP: webVitalsMock.onINP,
    onLCP: webVitalsMock.onLCP,
    onTTFB: webVitalsMock.onTTFB
}));

import { bootstrapWebVitals } from './vitals';

describe('web vitals tracker bundle', () => {
    it('emits the metric instance id without client-side rating', () => {
        const emit = vi.fn();
        const win = {
            hk: {
                _webVitals: {
                    emit,
                    getPath: () => '/pricing',
                    sessionId: '10000000-0000-4000-8000-000000000001',
                    pageId: () => '10000000-0000-4000-8000-000000000002',
                    trackerSource: 'hk.js',
                    trackerVersion: '2.6.0',
                    userAgent: 'Mozilla/5.0 Test'
                }
            }
        } as unknown as Window & typeof globalThis;

        bootstrapWebVitals(win);
        webVitalsMock.callbacks['LCP'][0]?.({
            name: 'LCP',
            value: 1842.3,
            rating: 'good',
            delta: 1842.3,
            id: 'v5-1234567890-1234567890123',
            entries: [],
            navigationType: 'navigate'
        });

        expect(emit).toHaveBeenCalledWith({
            n: 'LCP',
            v: 1842.3,
            p: '/pricing',
            nt: 'navigate',
            mid: 'v5-1234567890-1234567890123',
            sid: '10000000-0000-4000-8000-000000000001',
            pid: '10000000-0000-4000-8000-000000000002',
            tsrc: 'hk.js',
            tv: '2.6.0',
            ua: 'Mozilla/5.0 Test'
        });
        expect(emit.mock.calls[0]?.[0]).not.toHaveProperty('rating');
    });
});
