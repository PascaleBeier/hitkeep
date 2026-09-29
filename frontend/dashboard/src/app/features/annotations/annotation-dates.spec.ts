import { formatAnnotationWhen, fromDateTimeInputs, toDateTimeInputs } from '@features/annotations/annotation-dates';

describe('annotation dates', () => {
    it('round-trips day notes through UTC calendar days so they land on a bucket', () => {
        const bucket = new Date('2026-09-10T00:00:00Z');
        const inputs = toDateTimeInputs(bucket, 'day');
        expect(inputs).toEqual({ date: '2026-09-10', time: '' });
        expect(fromDateTimeInputs(inputs, 'day').toISOString()).toBe('2026-09-10T00:00:00.000Z');
    });

    it('round-trips hour notes through the local clock', () => {
        const bucket = new Date(2026, 8, 10, 14, 0);
        const inputs = toDateTimeInputs(bucket, 'hour');
        expect(inputs).toEqual({ date: '2026-09-10', time: '14:00' });
        expect(fromDateTimeInputs(inputs, 'hour').getTime()).toBe(bucket.getTime());
    });

    it('formats points and ranges', () => {
        expect(formatAnnotationWhen({ starts_at: '2026-09-10T00:00:00Z' }, 'day', 'en-US')).toBe('Sep 10');
        expect(formatAnnotationWhen({ starts_at: '2026-09-10T00:00:00Z', ends_at: '2026-09-14T00:00:00Z' }, 'day', 'en-US')).toMatch(/^Sep 10\s*–\s*14$/);
    });
});
