import type { Annotation } from '@models/analytics.types';
import type { AnnotationGranularity } from '@features/annotations/site-annotations.service';

export interface DateTimeInputs {
    date: string;
    time: string;
}

const pad = (value: number) => String(value).padStart(2, '0');

/**
 * Day buckets start at UTC midnight, so day notes are edited and shown as UTC
 * calendar days and land exactly on a bucket. Hour charts label buckets in
 * the reader's clock, so hour notes use local date and time.
 */
export function toDateTimeInputs(date: Date, granularity: AnnotationGranularity): DateTimeInputs {
    if (granularity === 'day') {
        return { date: date.toISOString().slice(0, 10), time: '' };
    }
    return {
        date: `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`,
        time: `${pad(date.getHours())}:${pad(date.getMinutes())}`
    };
}

export function fromDateTimeInputs(inputs: DateTimeInputs, granularity: AnnotationGranularity): Date {
    return granularity === 'day' ? new Date(`${inputs.date}T00:00:00Z`) : new Date(`${inputs.date}T${inputs.time || '00:00'}`);
}

export function formatAnnotationWhen(annotation: Pick<Annotation, 'starts_at' | 'ends_at'>, granularity: AnnotationGranularity, locale: string): string {
    const format = new Intl.DateTimeFormat(locale, granularity === 'day' ? { month: 'short', day: 'numeric', timeZone: 'UTC' } : { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
    const start = new Date(annotation.starts_at);
    if (!annotation.ends_at) {
        return format.format(start);
    }
    return format.formatRange(start, new Date(annotation.ends_at));
}
