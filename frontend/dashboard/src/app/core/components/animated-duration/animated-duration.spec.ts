import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { NumberFlowComponent } from 'ng-number-flow';

import { AnimatedDuration } from './animated-duration';

describe('AnimatedDuration', () => {
    let fixture: ComponentFixture<AnimatedDuration>;
    let transloco: TranslocoService;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                AnimatedDuration,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                durationMinutesSeconds: '{{minutes}}m {{seconds}}s',
                                durationSeconds: '{{seconds}}s'
                            }
                        },
                        nl: {
                            common: {
                                durationMinutesSeconds: '{{minutes}} m {{seconds}} s',
                                durationSeconds: "{{seconds}}'s"
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en', 'nl'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        transloco = TestBed.inject(TranslocoService);
        fixture = TestBed.createComponent(AnimatedDuration);
        fixture.componentRef.setInput('value', 83);
        await fixture.whenStable();
    });

    it('animates minutes and seconds as independent numeric slots', async () => {
        const flows = fixture.debugElement.queryAll(By.directive(NumberFlowComponent)).map((element) => element.componentInstance as NumberFlowComponent);

        expect(flows.map((flow) => flow.value())).toEqual([1, 23]);
        expect(fixture.nativeElement.querySelector('.hk-animated-duration').getAttribute('aria-label')).toBe('1m 23s');

        fixture.componentRef.setInput('value', 95);
        await fixture.whenStable();

        const updatedFlows = fixture.debugElement.queryAll(By.directive(NumberFlowComponent)).map((element) => element.componentInstance as NumberFlowComponent);
        expect(updatedFlows.map((flow) => flow.value())).toEqual([1, 35]);
    });

    it('uses the seconds-only translation below one minute', async () => {
        fixture.componentRef.setInput('value', 9);
        await fixture.whenStable();

        const flows = fixture.debugElement.queryAll(By.directive(NumberFlowComponent));
        expect(flows.length).toBe(1);
        expect((flows[0].componentInstance as NumberFlowComponent).value()).toBe(9);
        expect(fixture.nativeElement.querySelector('.hk-animated-duration').getAttribute('aria-label')).toBe('9s');
    });

    it('preserves localized unit spacing and exposes one accessible final value', async () => {
        transloco.setActiveLang('nl');
        await fixture.whenStable();

        const duration = fixture.nativeElement.querySelector('.hk-animated-duration');
        expect(duration.getAttribute('role')).toBe('img');
        expect(duration.getAttribute('aria-label')).toBe('1 m 23 s');
        expect(duration.querySelector('.hk-animated-duration__visual').getAttribute('aria-hidden')).toBe('true');
    });
});
