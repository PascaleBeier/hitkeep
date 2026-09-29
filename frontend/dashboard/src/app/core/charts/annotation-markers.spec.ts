import { annotationMarkers } from '@core/charts/annotation-markers';
import type { Annotation } from '@models/analytics.types';

const days = ['2026-09-01', '2026-09-02', '2026-09-03', '2026-09-04', '2026-09-05'].map((day) => ({ time: `${day}T00:00:00Z` }));

function note(id: string, startsAt: string, endsAt?: string): Annotation {
    return { id, site_id: 's', starts_at: startsAt, ends_at: endsAt, body: id, created_at: startsAt };
}

describe('annotationMarkers', () => {
    it('snaps a point to the bucket that contains it', () => {
        const markers = annotationMarkers(days, [note('launch', '2026-09-03T15:30:00Z')]);
        expect(markers.lines).toEqual([{ index: 2, annotations: [expect.objectContaining({ id: 'launch' })] }]);
        expect(markers.areas).toEqual([]);
    });

    it('groups points that land in the same bucket', () => {
        const markers = annotationMarkers(days, [note('a', '2026-09-02T01:00:00Z'), note('b', '2026-09-02T22:00:00Z')]);
        expect(markers.lines).toHaveLength(1);
        expect(markers.lines[0].annotations.map((a) => a.id)).toEqual(['a', 'b']);
    });

    it('treats the bucket edges as inclusive start, exclusive end', () => {
        const markers = annotationMarkers(days, [note('edge', '2026-09-04T00:00:00Z'), note('last', '2026-09-05T23:59:59Z')]);
        expect(markers.lines.map((l) => l.index)).toEqual([3, 4]);
    });

    it('drops notes entirely outside the window', () => {
        const markers = annotationMarkers(days, [
            note('before', '2026-08-30T00:00:00Z'),
            note('after', '2026-09-06T00:00:00Z'),
            note('range-before', '2026-08-20T00:00:00Z', '2026-08-31T12:00:00Z')
        ]);
        expect(markers).toEqual({ lines: [], areas: [], visible: [] });
    });

    it('clamps ranges that cross either end of the window', () => {
        const markers = annotationMarkers(days, [note('spans', '2026-08-20T00:00:00Z', '2026-09-20T00:00:00Z'), note('tail', '2026-09-04T08:00:00Z', '2026-10-01T00:00:00Z')]);
        expect(markers.areas.map(({ start, end, annotation }) => [annotation.id, start, end])).toEqual([
            ['spans', 0, 4],
            ['tail', 3, 4]
        ]);
    });

    it('draws a range inside a single bucket as a line', () => {
        const markers = annotationMarkers(days, [note('outage', '2026-09-02T10:00:00Z', '2026-09-02T11:00:00Z')]);
        expect(markers.areas).toEqual([]);
        expect(markers.lines[0].index).toBe(1);
    });

    it('lists visible notes chronologically', () => {
        const markers = annotationMarkers(days, [note('late', '2026-09-05T00:00:00Z'), note('early', '2026-09-01T00:00:00Z', '2026-09-02T00:00:00Z')]);
        expect(markers.visible.map((a) => a.id)).toEqual(['early', 'late']);
    });

    it('returns nothing for an empty chart', () => {
        expect(annotationMarkers([], [note('x', '2026-09-01T00:00:00Z')])).toEqual({ lines: [], areas: [], visible: [] });
    });
});
