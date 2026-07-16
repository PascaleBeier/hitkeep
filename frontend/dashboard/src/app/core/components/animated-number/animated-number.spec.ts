import { CSP_NONCE } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { NumberFlowComponent } from 'ng-number-flow';

import { AnimatedNumber } from './animated-number';

describe('AnimatedNumber', () => {
    let fixture: ComponentFixture<AnimatedNumber>;
    let transloco: TranslocoService;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                AnimatedNumber,
                TranslocoTestingModule.forRoot({
                    langs: { en: {}, pt: {} },
                    translocoConfig: {
                        availableLangs: ['en', 'pt'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [{ provide: CSP_NONCE, useValue: 'test-nonce' }]
        }).compileComponents();

        transloco = TestBed.inject(TranslocoService);
        fixture = TestBed.createComponent(AnimatedNumber);
        fixture.componentRef.setInput('value', 1234.5);
        fixture.componentRef.setInput('format', { minimumFractionDigits: 1, maximumFractionDigits: 1 });
        fixture.componentRef.setInput('prefix', '~');
        fixture.componentRef.setInput('suffix', '%');
        await fixture.whenStable();
    });

    it('forwards locale-aware formatting, demo-default motion, and the CSP nonce', () => {
        const flow = fixture.debugElement.query(By.directive(NumberFlowComponent)).componentInstance as NumberFlowComponent;
        const element = fixture.nativeElement.querySelector('number-flow-ng') as HTMLElement & {
            transformTiming: EffectTiming;
            spinTiming?: EffectTiming;
            opacityTiming: EffectTiming;
        };

        expect(flow.value()).toBe(1234.5);
        expect(flow.locales()).toBe('en-US');
        expect(flow.format()).toEqual({ minimumFractionDigits: 1, maximumFractionDigits: 1 });
        expect(flow.prefix()).toBe('~');
        expect(flow.suffix()).toBe('%');
        expect(flow.animated()).toBe(true);
        expect(flow.respectMotionPreference()).toBe(true);
        expect(flow.transformTiming()).toBeUndefined();
        expect(flow.spinTiming()).toBeUndefined();
        expect(flow.opacityTiming()).toBeUndefined();
        expect(element.transformTiming.duration).toBe(900);
        expect(element.transformTiming.easing).toMatch(/^linear\(/);
        expect(element.spinTiming).toBeUndefined();
        expect(element.opacityTiming).toEqual({ duration: 450, easing: 'ease-out' });
        expect(flow.willChange()).toBe(false);
        expect(flow.nonce()).toBe('test-nonce');
    });

    it('exposes the formatted value to assistive technology and nonces the shadow style', () => {
        const element = fixture.nativeElement.querySelector('number-flow-ng') as HTMLElement & {
            _internals?: ElementInternals;
        };

        expect(element._internals?.role).toBe('img');
        expect(element._internals?.ariaLabel).toBe('~1,234.5%');
        expect(element.shadowRoot?.querySelector('style')?.nonce).toBe('test-nonce');
    });

    it('reformats with the full locale when the dashboard language changes', async () => {
        transloco.setActiveLang('pt');
        await fixture.whenStable();

        const flow = fixture.debugElement.query(By.directive(NumberFlowComponent)).componentInstance as NumberFlowComponent;
        expect(flow.locales()).toBe('pt-BR');
    });
});
