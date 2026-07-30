import { computed, inject, signal, Service } from '@angular/core';
import { DEFAULT_RANGE_OPTIONS, DateRange, isShortRange, RangeOption, RangeSelectEvent, RangeValue, resolveDateRange, selectDefaultRange } from '@components/range-toolbar/range-toolbar';
import { PreferenceStorage } from '@services/preference-storage';

interface StoredReportRange {
    customRange?: string[];
    value?: RangeValue;
}

export interface ReportRangeSelection {
    customRangeDates: Date[] | null;
    range: RangeOption;
}

const REPORT_RANGE_STORAGE_KEY = 'hitkeep.reportRange';

@Service()
export class ReportRangePreferencesService {
    private readonly storage = inject(PreferenceStorage);
    private readonly defaultRange = selectDefaultRange(DEFAULT_RANGE_OPTIONS);
    private readonly selectedRangeState = signal<RangeOption>(this.defaultRange);
    private readonly customRangeDatesState = signal<Date[] | null>(null);
    private initializedFromStorage = false;

    readonly selectedRange = this.selectedRangeState.asReadonly();
    readonly customRangeDates = this.customRangeDatesState.asReadonly();
    readonly isShortRange = computed(() => isShortRange(this.selectedRange(), this.customRangeDates()));

    initialize(fallback: RangeOption = this.defaultRange): void {
        if (!this.storage.available()) {
            this.applySelection({ range: fallback, customRangeDates: null });
            this.initializedFromStorage = false;
            return;
        }

        if (this.initializedFromStorage) {
            return;
        }

        this.applySelection(this.initialSelection(DEFAULT_RANGE_OPTIONS, fallback));
        this.initializedFromStorage = true;
    }

    selectRange(event: RangeSelectEvent): void {
        const customRangeDates = this.saveSelection(event, this.customRangeDates());
        this.customRangeDatesState.set(customRangeDates);
        this.selectedRangeState.set(event.value);
        this.initializedFromStorage = this.storage.available();
    }

    defaultDateRange(): DateRange {
        return resolveDateRange(this.defaultRange, null)!;
    }

    currentDateRange(): DateRange | null {
        return resolveDateRange(this.selectedRange(), this.customRangeDates());
    }

    initialSelection(ranges: readonly RangeOption[] = DEFAULT_RANGE_OPTIONS, fallback: RangeOption = selectDefaultRange(ranges)): ReportRangeSelection {
        const stored = this.storage.read<StoredReportRange>(REPORT_RANGE_STORAGE_KEY);
        if (!stored) {
            return { range: fallback, customRangeDates: null };
        }

        const range = ranges.find((option) => option.value === stored.value);
        if (!range) {
            return { range: fallback, customRangeDates: null };
        }

        if (range.value !== 'custom') {
            return { range, customRangeDates: null };
        }

        const customRangeDates = this.parseStoredCustomRange(stored.customRange);
        return customRangeDates ? { range, customRangeDates } : { range: fallback, customRangeDates: null };
    }

    saveSelection(event: RangeSelectEvent, previousCustomRangeDates: Date[] | null = null): Date[] | null {
        const customRangeDates = event.value.value === 'custom' ? (event.customRange ?? previousCustomRangeDates) : null;

        const stored: StoredReportRange = { value: event.value.value };
        if (event.value.value === 'custom' && this.isCompleteCustomRange(customRangeDates)) {
            stored.customRange = customRangeDates.map((date) => date.toISOString());
        }
        this.storage.write(REPORT_RANGE_STORAGE_KEY, stored);

        return customRangeDates;
    }

    private applySelection(selection: ReportRangeSelection): void {
        this.selectedRangeState.set(selection.range);
        this.customRangeDatesState.set(selection.customRangeDates);
    }

    private parseStoredCustomRange(value: string[] | undefined): Date[] | null {
        if (!value || value.length !== 2) {
            return null;
        }

        const dates = value.map((entry) => new Date(entry));
        if (!this.isCompleteCustomRange(dates) || dates[0]!.getTime() > dates[1]!.getTime()) {
            return null;
        }
        return dates;
    }

    private isCompleteCustomRange(value: Date[] | null): value is [Date, Date] {
        return !!value && value.length === 2 && !!value[0] && !!value[1] && value.every((date) => !Number.isNaN(date.getTime()));
    }
}

export function injectReportRange(fallback?: RangeOption): ReportRangePreferencesService {
    const reportRange = inject(ReportRangePreferencesService);
    reportRange.initialize(fallback);
    return reportRange;
}
