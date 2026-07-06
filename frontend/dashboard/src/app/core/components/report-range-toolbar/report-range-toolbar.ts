import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { DEFAULT_RANGE_OPTIONS, RangeOption, RangeSelectEvent, RangeToolbar } from '@components/range-toolbar/range-toolbar';
import { injectReportRange } from '@services/report-range-preferences.service';

@Component({
    selector: 'app-report-range-toolbar',
    imports: [RangeToolbar],
    template: `
        <app-range-toolbar [timeRanges]="timeRanges()" [selectedRange]="selectedRange()" [customRangeDates]="customRangeDates()" [loading]="loading()" (rangeChange)="selectRange($event)" (refresh)="refresh.emit()">
            <div toolbar-right class="contents">
                <ng-content select="[toolbar-right]"></ng-content>
            </div>
        </app-range-toolbar>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ReportRangeToolbar {
    private readonly reportRange = injectReportRange();

    readonly timeRanges = input<RangeOption[]>(DEFAULT_RANGE_OPTIONS);
    readonly loading = input(false);

    readonly rangeChange = output<RangeSelectEvent>();
    readonly refresh = output<void>();

    protected readonly selectedRange = this.reportRange.selectedRange;
    protected readonly customRangeDates = this.reportRange.customRangeDates;

    protected selectRange(event: RangeSelectEvent): void {
        this.reportRange.selectRange(event);
        this.rangeChange.emit(event);
    }
}
