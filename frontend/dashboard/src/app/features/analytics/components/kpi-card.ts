import { ChangeDetectionStrategy, Component, computed, input, OnChanges, OnDestroy, signal, SimpleChanges } from '@angular/core';
import { AnimatedDuration } from '@components/animated-duration/animated-duration';
import { AnimatedNumber } from '@components/animated-number/animated-number';
import { CardModule } from '@openng/optimus-ui/card';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';

export const KPI_PERCENT_FORMAT: Intl.NumberFormatOptions = Object.freeze({
    minimumFractionDigits: 1,
    maximumFractionDigits: 1
});

export const KPI_ONE_DECIMAL_FORMAT: Intl.NumberFormatOptions = Object.freeze({
    minimumFractionDigits: 1,
    maximumFractionDigits: 1
});

export const KPI_SHORT_DECIMAL_FORMAT: Intl.NumberFormatOptions = Object.freeze({
    minimumFractionDigits: 1,
    maximumFractionDigits: 2
});

export const KPI_MONEY_FALLBACK_FORMAT: Intl.NumberFormatOptions = Object.freeze({
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
});

const KPI_DELTA_FORMAT: Intl.NumberFormatOptions = Object.freeze({
    style: 'percent',
    signDisplay: 'always',
    minimumFractionDigits: 1,
    maximumFractionDigits: 1
});

export interface KpiCardModel {
    label: string;
    value: string | number;
    loading: boolean;
    valueClass?: string;
    format?: Intl.NumberFormatOptions;
    prefix?: string;
    suffix?: string;
    duration?: boolean;
    delta?: number | null;
    invertDelta?: boolean;
    updateKey?: number;
}

@Component({
    selector: 'app-kpi-card',
    standalone: true,
    imports: [CardModule, SkeletonModule, AnimatedDuration, AnimatedNumber],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <p-card class="shadow-sm h-full border border-surface-200 dark:border-surface-700 surface-card">
            <div class="hk-kpi-card__body flex flex-col gap-2">
                @for (cue of updateCues(); track cue) {
                    <span class="hk-kpi-card__update-cue" [attr.data-update-cue]="cue" aria-hidden="true"></span>
                }
                <span class="hk-kpi-card__content text-sm font-medium text-muted-color">{{ label() }}</span>
                <div [class]="displayClass()">
                    @if (loading()) {
                        <p-skeleton width="60%" height="2rem" />
                    } @else {
                        @let numeric = numericValue();
                        @if (numeric !== null) {
                            @if (duration()) {
                                <app-animated-duration [value]="numeric" />
                            } @else {
                                <app-animated-number [value]="numeric" [format]="format()" [prefix]="prefix()" [suffix]="suffix()" />
                            }
                        } @else {
                            {{ value() }}
                        }
                    }
                </div>
                @let deltaValue = normalizedDelta();
                @if (!loading() && deltaValue !== null) {
                    <div class="hk-kpi-card__content flex items-center gap-1">
                        <span [class]="deltaClass()"><app-animated-number [value]="deltaValue" [format]="deltaFormat" /></span>
                    </div>
                }
            </div>
        </p-card>
    `,
    styles: [
        `
            .hk-kpi-card__body {
                border-radius: 0.5rem;
                isolation: isolate;
                margin: -0.25rem;
                overflow: hidden;
                padding: 0.25rem;
                position: relative;
            }

            .hk-kpi-card__content {
                position: relative;
                z-index: 1;
            }

            .hk-kpi-card__update-cue {
                background: color-mix(in srgb, var(--p-text-color) 5%, transparent);
                border-radius: inherit;
                display: none;
                inset: 0;
                pointer-events: none;
                position: absolute;
                z-index: 0;
            }

            @media (prefers-reduced-motion: reduce) {
                .hk-kpi-card__update-cue {
                    display: block;
                }
            }
        `
    ]
})
export class KpiCard implements OnChanges, OnDestroy {
    label = input.required<string>();
    value = input.required<string | number>();
    loading = input<boolean>(false);
    updateKey = input<number>(0);
    valueClass = input<string>('');
    format = input<Intl.NumberFormatOptions>();
    prefix = input<string>();
    suffix = input<string>();
    duration = input<boolean>(false);
    delta = input<number | null>(null);
    invertDelta = input<boolean>(false);

    private cueSequence = 0;
    private cueTimer: ReturnType<typeof setTimeout> | null = null;

    protected readonly updateCues = signal<readonly number[]>([]);
    protected readonly deltaFormat = KPI_DELTA_FORMAT;
    protected displayClass = computed(() => `hk-kpi-card__content ${this.valueClass() || 'text-2xl xl:text-3xl font-bold'}`);
    protected numericValue = computed(() => {
        const value = this.value();
        return typeof value === 'number' && Number.isFinite(value) ? value : null;
    });
    protected normalizedDelta = computed(() => {
        const delta = this.delta();
        if (delta === null || !Number.isFinite(delta)) return null;
        return (this.invertDelta() ? -delta : delta) / 100;
    });

    protected deltaClass = computed(() => {
        const normalized = this.normalizedDelta();
        if (normalized === null) return '';
        const positive = normalized >= 0;
        return positive
            ? 'text-xs font-medium px-1.5 py-0.5 rounded-full bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
            : 'text-xs font-medium px-1.5 py-0.5 rounded-full bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400';
    });

    ngOnChanges(changes: SimpleChanges): void {
        const updateKey = changes['updateKey'];
        const value = changes['value'];
        if (!updateKey || updateKey.firstChange || Number(updateKey.currentValue) <= Number(updateKey.previousValue) || !value || value.firstChange || Object.is(value.previousValue, value.currentValue) || this.loading()) {
            return;
        }

        this.showUpdateCue();
    }

    ngOnDestroy(): void {
        if (this.cueTimer) {
            clearTimeout(this.cueTimer);
        }
    }

    private showUpdateCue(): void {
        if (this.cueTimer) {
            clearTimeout(this.cueTimer);
        }

        this.updateCues.set([++this.cueSequence]);
        this.cueTimer = setTimeout(() => {
            this.updateCues.set([]);
            this.cueTimer = null;
        }, 600);
    }
}
