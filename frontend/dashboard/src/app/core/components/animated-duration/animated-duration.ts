import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { TranslocoService } from '@jsverse/transloco';

import { AnimatedNumber } from '@components/animated-number/animated-number';
import { injectActiveLang } from '@core/i18n/active-lang';

const MINUTES_TOKEN = '__HK_DURATION_MINUTES__';
const SECONDS_TOKEN = '__HK_DURATION_SECONDS__';
const TOKEN_PATTERN = /(__HK_DURATION_MINUTES__|__HK_DURATION_SECONDS__)/g;
const INTEGER_FORMAT: Intl.NumberFormatOptions = Object.freeze({ maximumFractionDigits: 0 });

type DurationPart = { id: string; kind: 'number'; value: number } | { id: string; kind: 'literal'; value: string };

@Component({
    selector: 'app-animated-duration',
    imports: [AnimatedNumber],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <span class="hk-animated-duration" role="img" [attr.aria-label]="duration().label">
            <span class="hk-animated-duration__visual" aria-hidden="true">
                @for (part of duration().parts; track part.id) {
                    @if (part.kind === 'number') {
                        <app-animated-number [value]="part.value" [format]="integerFormat" />
                    } @else {
                        <span>{{ part.value }}</span>
                    }
                }
            </span>
        </span>
    `,
    styles: `
        :host,
        .hk-animated-duration {
            color: inherit;
            display: inline-block;
            font: inherit;
            line-height: inherit;
            min-width: 0;
        }

        .hk-animated-duration__visual {
            white-space: pre;
        }
    `
})
export class AnimatedDuration {
    readonly value = input.required<number>();

    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = injectActiveLang();

    protected readonly integerFormat = INTEGER_FORMAT;
    protected readonly duration = computed(() => {
        this.activeLanguage();

        const totalSeconds = Math.max(0, Math.floor(Number.isFinite(this.value()) ? this.value() : 0));
        const minutes = Math.floor(totalSeconds / 60);
        const seconds = totalSeconds % 60;
        const hasMinutes = minutes > 0;
        const key = hasMinutes ? 'common.durationMinutesSeconds' : 'common.durationSeconds';
        const values = hasMinutes ? { minutes, seconds } : { seconds };
        const tokenValues = hasMinutes ? { minutes: MINUTES_TOKEN, seconds: SECONDS_TOKEN } : { seconds: SECONDS_TOKEN };
        const template = this.transloco.translate(key, tokenValues);
        const numberValues = new Map([
            [MINUTES_TOKEN, minutes],
            [SECONDS_TOKEN, seconds]
        ]);

        return {
            label: this.transloco.translate(key, values),
            parts: template
                .split(TOKEN_PATTERN)
                .filter((part) => part.length > 0)
                .map<DurationPart>((part, index) => {
                    const number = numberValues.get(part);
                    return number === undefined ? { id: `literal-${index}`, kind: 'literal', value: part } : { id: part, kind: 'number', value: number };
                })
        };
    });
}
