import { ChangeDetectionStrategy, Component, computed, inject, input, output } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { TranslocoService } from '@jsverse/transloco';
import { SelectButtonModule } from 'primeng/selectbutton';
import type { HitkeepChartDesign } from '@core/charts/hitkeep-chart-options';

interface ChartDesignToggleOption {
    value: HitkeepChartDesign;
    label: string;
}

/** Horizontal icon toggle for switching the chart design (area / line / bars). */
@Component({
    selector: 'app-chart-design-toggle',
    imports: [FormsModule, SelectButtonModule],
    template: `
        <p-selectbutton [options]="options()" [ngModel]="value()" (ngModelChange)="onChange($event)" optionValue="value" optionLabel="label" [allowEmpty]="false" size="small" [attr.aria-label]="groupLabel()">
            <ng-template #item let-option>
                <span class="inline-flex" [attr.title]="option.label">
                    @switch (option.value) {
                        @case ('area') {
                            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                                <path d="M1.5 13V9.6L5.6 5.4L8.9 8.3L14.5 3.8V13H1.5Z" fill="currentColor" fill-opacity="0.45" />
                                <path d="M1.5 9.6L5.6 5.4L8.9 8.3L14.5 3.8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
                            </svg>
                        }
                        @case ('line') {
                            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                                <path d="M1.5 11.5L5.6 6.2L8.9 9.2L14.5 3.5" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" />
                            </svg>
                        }
                        @case ('bar') {
                            <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                                <rect x="2" y="8.5" width="3" height="5.5" rx="0.75" />
                                <rect x="6.5" y="3.5" width="3" height="10.5" rx="0.75" />
                                <rect x="11" y="6" width="3" height="8" rx="0.75" />
                            </svg>
                        }
                    }
                    <span class="sr-only">{{ option.label }}</span>
                </span>
            </ng-template>
        </p-selectbutton>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ChartDesignToggle {
    readonly value = input.required<HitkeepChartDesign>();
    readonly valueChange = output<HitkeepChartDesign>();

    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    protected readonly groupLabel = computed(() => {
        this.activeLanguage();
        return this.transloco.translate('common.chartDesign.label');
    });

    protected readonly options = computed<ChartDesignToggleOption[]>(() => {
        this.activeLanguage();
        return [
            { value: 'area', label: this.transloco.translate('common.chartDesign.area') },
            { value: 'line', label: this.transloco.translate('common.chartDesign.line') },
            { value: 'bar', label: this.transloco.translate('common.chartDesign.bar') }
        ];
    });

    protected onChange(value: HitkeepChartDesign | null): void {
        if (value) {
            this.valueChange.emit(value);
        }
    }
}
