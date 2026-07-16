import { CSP_NONCE, ChangeDetectionStrategy, Component, ElementRef, afterNextRender, computed, inject, input, signal } from '@angular/core';
import { NumberFlowComponent, type Format } from 'ng-number-flow';

import { injectActiveLang } from '@core/i18n/active-lang';
import { DASHBOARD_LOCALE_MAPPING, SOURCE_LOCALE } from '@core/i18n/supported-locales';

@Component({
    selector: 'app-animated-number',
    imports: [NumberFlowComponent],
    template: `
        <number-flow [value]="ready() ? value() : undefined" [locales]="locale()" [format]="numberFlowFormat()" [prefix]="prefix()" [suffix]="suffix()" [animated]="true" [respectMotionPreference]="true" [willChange]="false" [nonce]="cspNonce" />
    `,
    styles: `
        :host {
            display: inline-block;
            min-width: 0;
            color: inherit;
            font: inherit;
            font-variant-numeric: tabular-nums;
            line-height: inherit;
        }
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AnimatedNumber {
    readonly value = input.required<number>();
    readonly format = input<Intl.NumberFormatOptions>();
    readonly prefix = input<string>();
    readonly suffix = input<string>();

    private readonly activeLanguage = injectActiveLang();
    private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

    protected readonly cspNonce = inject(CSP_NONCE, { optional: true }) ?? undefined;
    protected readonly ready = signal(false);
    protected readonly numberFlowFormat = computed(() => this.format() as Format | undefined);
    protected readonly locale = computed(() => DASHBOARD_LOCALE_MAPPING[this.activeLanguage() as keyof typeof DASHBOARD_LOCALE_MAPPING] ?? SOURCE_LOCALE);

    constructor() {
        afterNextRender(() => {
            const element = this.host.nativeElement.querySelector<HTMLElement>('number-flow-ng');
            if (element && this.cspNonce) {
                element.nonce = this.cspNonce;
            }

            // ng-number-flow 0.2.0 exposes `nonce` but does not copy it to its
            // custom element. Delay the first value until that boundary is set.
            this.ready.set(true);
        });
    }
}
