import type { Annotation } from '@models/analytics.types';

/** Point notes that share a bucket collapse into one line. */
export interface AnnotationLineMarker {
    index: number;
    annotations: Annotation[];
}

export interface AnnotationAreaMarker {
    start: number;
    end: number;
    annotation: Annotation;
}

export interface AnnotationMarkers {
    lines: AnnotationLineMarker[];
    areas: AnnotationAreaMarker[];
    /** Every note that touches the chart window, in chronological order. */
    visible: Annotation[];
}

const EMPTY: AnnotationMarkers = { lines: [], areas: [], visible: [] };
const DAY_MS = 86_400_000;

/**
 * Places notes on a category axis of ascending bucket start times. A time
 * belongs to the last bucket starting at or before it, so a note always sits
 * on the bucket whose value it explains. Ranges are clamped to the window;
 * notes entirely outside it are dropped.
 */
export function annotationMarkers(points: readonly { time: string }[], annotations: readonly Annotation[]): AnnotationMarkers {
    if (points.length === 0 || annotations.length === 0) {
        return EMPTY;
    }
    const times = points.map((point) => Date.parse(point.time));
    const last = times.length - 1;
    const step = last > 0 ? times[1] - times[0] : DAY_MS;
    const windowStart = times[0];
    const windowEnd = times[last] + step;

    // ponytail: linear scan per note; buckets stay in the hundreds, binary search if that changes.
    const bucketOf = (time: number): number => {
        let index = last;
        while (index > 0 && times[index] > time) {
            index--;
        }
        return index;
    };

    const byBucket = new Map<number, Annotation[]>();
    const areas: AnnotationAreaMarker[] = [];
    const visible: Annotation[] = [];

    for (const annotation of annotations) {
        const start = Date.parse(annotation.starts_at);
        const end = annotation.ends_at ? Date.parse(annotation.ends_at) : start;
        if (Number.isNaN(start) || end < windowStart || start >= windowEnd) {
            continue;
        }
        visible.push(annotation);

        const startIndex = start < windowStart ? 0 : bucketOf(start);
        const endIndex = end >= windowEnd ? last : bucketOf(end);
        // A range inside one bucket has no width on the axis; draw it as a line.
        if (annotation.ends_at && endIndex > startIndex) {
            areas.push({ start: startIndex, end: endIndex, annotation });
            continue;
        }
        const bucket = byBucket.get(startIndex);
        if (bucket) {
            bucket.push(annotation);
        } else {
            byBucket.set(startIndex, [annotation]);
        }
    }

    const lines = [...byBucket.entries()].sort(([a], [b]) => a - b).map(([index, notes]) => ({ index, annotations: notes }));
    visible.sort((a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at));
    return { lines, areas, visible };
}
