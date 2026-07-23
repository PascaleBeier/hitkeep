import { ChangeDetectionStrategy, Component, computed, effect, inject, input, output, signal } from '@angular/core';
import { FormControl, FormsModule, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { RouterLink } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { ButtonModule } from '@openng/optimus-ui/button';
import { DatePickerModule } from '@openng/optimus-ui/datepicker';
import { Popover, PopoverModule } from '@openng/optimus-ui/popover';
import { SelectModule } from '@openng/optimus-ui/select';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';
import { TooltipModule } from '@openng/optimus-ui/tooltip';
import { injectActiveLang } from '@core/i18n/active-lang';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { ShareService } from '@services/share.service';
import { TeamService } from '@services/team.service';
import { resolveDateRange, type RangeOption, type RangeValue } from './range-options';
import { translateRangeLabel, translateRangeShortLabel } from './range-labels';

export { DEFAULT_RANGE_OPTIONS, DEFAULT_RANGE_VALUE, isShortRange, resolveDateRange, selectDefaultRange } from './range-options';
export { translateRangeLabel, translateRangeShortLabel } from './range-labels';
export type { DateRange, RangeOption, RangeValue } from './range-options';

export interface RangeSelectEvent {
    value: RangeOption;
    customRange?: Date[] | null;
}

interface ToolbarRangeOption extends RangeOption {
    label: string;
    shortLabel: string;
}

const VISIBLE_PRESET_VALUES = new Set<RangeValue>(['today', 'yesterday', '24h', '7d', '30d']);

@Component({
    selector: 'app-range-toolbar',
    imports: [FormsModule, ReactiveFormsModule, RouterLink, DatePickerModule, PopoverModule, ButtonModule, SelectButtonModule, SelectModule, TooltipModule, TranslocoPipe],
    templateUrl: './range-toolbar.html',
    styleUrl: './range-toolbar.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class RangeToolbar {
    private readonly transloco = inject(TranslocoService);
    private readonly localeService = inject(TranslocoLocaleService);
    private readonly activeLanguage = injectActiveLang();
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly share = inject(ShareService);
    private readonly teamService = inject(TeamService);

    timeRanges = input.required<RangeOption[]>();
    selectedRange = input.required<RangeOption>();
    customRangeDates = input<Date[] | null>(null);
    loading = input<boolean>(false);

    selectedRangeChange = output<RangeOption>();
    customRangeDatesChange = output<Date[] | null>();
    rangeChange = output<RangeSelectEvent>();
    refresh = output<void>();

    private readonly rangeModel = signal({
        customStart: new FormControl<Date | null>(null),
        customEnd: new FormControl<Date | null>(null)
    });
    protected readonly rangeForm = compatForm(this.rangeModel);
    protected readonly translatedTimeRanges = computed(() => {
        this.activeLanguage();
        return this.timeRanges().map((option) => ({
            ...option,
            label: option.label ?? this.defaultLabel(option.value),
            shortLabel: this.shortLabel(option.value)
        })) satisfies ToolbarRangeOption[];
    });
    protected readonly presetRanges = computed(() => this.translatedTimeRanges().filter((option) => option.value !== 'custom'));
    protected readonly visiblePresetRanges = computed(() => this.presetRanges().filter((option) => VISIBLE_PRESET_VALUES.has(option.value)));
    protected readonly overflowPresetRanges = computed(() => this.presetRanges().filter((option) => !VISIBLE_PRESET_VALUES.has(option.value)));
    protected readonly selectedPresetValue = computed(() => {
        const value = this.selectedRange().value;
        return this.visiblePresetRanges().some((option) => option.value === value) ? value : null;
    });
    protected readonly overflowSelectValue = computed(() => {
        const value = this.selectedRange().value;
        return this.overflowPresetRanges().some((option) => option.value === value) ? value : null;
    });
    protected readonly customRangeSummary = computed(() => {
        this.activeLanguage();
        if (!this.isCustomActive()) {
            return null;
        }

        const dates = this.customRangeDates();
        if (!dates || !dates[0] || !dates[1]) {
            return null;
        }

        const from = this.localeService.localizeDate(dates[0], undefined, {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit'
        });
        const to = this.localeService.localizeDate(dates[1], undefined, {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit'
        });

        return this.transloco.translate('common.timeRanges.customRangeSummary', { start: from, end: to });
    });
    protected readonly customButtonLabel = computed(() => {
        this.activeLanguage();
        return this.transloco.translate('common.timeRanges.customShort');
    });
    protected readonly moreRangesLabel = computed(() => {
        this.activeLanguage();
        return this.transloco.translate('common.timeRanges.moreRanges');
    });
    protected readonly searchRangesLabel = computed(() => {
        this.activeLanguage();
        return this.transloco.translate('common.timeRanges.searchRanges');
    });
    protected readonly datePickerDateFormat = computed(() => {
        this.activeLanguage();
        return this.buildDatePickerDateFormat(this.localeService.getLocale());
    });
    protected readonly datePickerHourFormat = computed<'12' | '24'>(() => {
        this.activeLanguage();
        return this.uses12HourClock(this.localeService.getLocale()) ? '12' : '24';
    });
    protected readonly canApplyCustomRange = computed(() => {
        const start = this.rangeForm.customStart().value();
        const end = this.rangeForm.customEnd().value();
        return Boolean(start && end && start.getTime() <= end.getTime());
    });
    protected readonly isCustomActive = computed(() => this.selectedRange().value === 'custom');

    /** Free-plan retention cap in days when the selected range reaches past it, else null. */
    protected readonly retentionHintDays = computed(() => {
        const team = this.teamService.activeTeam();
        const retentionDays = team?.entitlements?.max_retention_days ?? 0;
        if (!this.bootstrap.cloudHosted() || this.share.isShareMode() || team?.plan?.code !== 'free' || retentionDays <= 0) {
            return null;
        }

        const range = resolveDateRange(this.selectedRange(), this.customRangeDates());
        if (!range) {
            return null;
        }

        const retainedFrom = new Date();
        retainedFrom.setDate(retainedFrom.getDate() - retentionDays);
        return new Date(range.from).getTime() < retainedFrom.getTime() ? retentionDays : null;
    });

    constructor() {
        effect(() => {
            const dates = this.customRangeDates();
            this.rangeForm
                .customStart()
                .control()
                .setValue(dates?.[0] ?? null, { emitEvent: false });
            this.rangeForm
                .customEnd()
                .control()
                .setValue(dates?.[1] ?? null, { emitEvent: false });
        });
    }

    protected selectPreset(option: ToolbarRangeOption) {
        if (this.selectedRange().value === option.value) {
            return;
        }
        this.selectedRangeChange.emit(option);
        this.rangeChange.emit({ value: option });
    }

    protected selectPresetValue(value: RangeValue | null) {
        if (!value) {
            return;
        }

        const option = this.visiblePresetRanges().find((range) => range.value === value);
        if (option) {
            this.selectPreset(option);
        }
    }

    protected selectOverflowPresetValue(value: RangeValue | null) {
        if (!value) {
            return;
        }

        const option = this.overflowPresetRanges().find((range) => range.value === value);
        if (option) {
            this.selectPreset(option);
        }
    }

    protected toggleCustomRange(event: Event, popover: Popover) {
        popover.toggle(event);
    }

    protected applyCustomRange(popover: Popover) {
        const start = this.rangeForm.customStart().value();
        const end = this.rangeForm.customEnd().value();
        if (!start || !end || start.getTime() > end.getTime()) {
            return;
        }

        const customRange = [start, end];
        const selected = this.translatedTimeRanges().find((option) => option.value === 'custom') ?? {
            value: 'custom',
            label: this.transloco.translate('common.timeRanges.customRange')
        };

        this.customRangeDatesChange.emit(customRange);
        this.selectedRangeChange.emit(selected);
        this.rangeChange.emit({ value: selected, customRange });
        popover.hide();
    }

    protected cancelCustomRange(popover: Popover) {
        const dates = this.customRangeDates();
        this.rangeForm
            .customStart()
            .control()
            .setValue(dates?.[0] ?? null, { emitEvent: false });
        this.rangeForm
            .customEnd()
            .control()
            .setValue(dates?.[1] ?? null, { emitEvent: false });
        popover.hide();
    }

    protected isPresetActive(value: string) {
        return this.selectedRange().value === value;
    }

    private defaultLabel(value: RangeValue): string {
        return translateRangeLabel(this.transloco, value);
    }

    private shortLabel(value: RangeValue): string {
        return translateRangeShortLabel(this.transloco, value);
    }

    private buildDatePickerDateFormat(locale: string): string {
        const parts = new Intl.DateTimeFormat(locale, {
            year: 'numeric',
            month: '2-digit',
            day: '2-digit'
        }).formatToParts(new Date(Date.UTC(2026, 2, 8, 12, 0, 0)));

        return parts
            .map((part) => {
                switch (part.type) {
                    case 'day':
                        return 'dd';
                    case 'month':
                        return 'mm';
                    case 'year':
                        return 'yy';
                    case 'literal':
                        return part.value;
                    default:
                        return '';
                }
            })
            .join('');
    }

    private uses12HourClock(locale: string): boolean {
        return new Intl.DateTimeFormat(locale, { hour: 'numeric' }).resolvedOptions().hour12 ?? false;
    }
}
