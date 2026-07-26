import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { KpiCard } from '@features/analytics/components/kpi-card';
import { ReportSubjectService } from '@services/report-subject.service';
import { vi } from 'vitest';

describe('KpiCard', () => {
    let fixture: ComponentFixture<KpiCard>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                KpiCard,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                durationMinutesSeconds: '{{minutes}}m {{seconds}}s',
                                durationSeconds: '{{seconds}}s'
                            }
                        },
                        de: {
                            common: {
                                durationMinutesSeconds: '{{minutes}}m {{seconds}}s',
                                durationSeconds: '{{seconds}}s'
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en', 'de'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(KpiCard);
        fixture.componentRef.setInput('label', 'Bounce Rate');
        fixture.componentRef.setInput('value', '45.0%');
        fixture.componentRef.setInput('loading', false);
    });

    it('renders finite numbers with the animated facade and keeps formatted strings static', async () => {
        fixture.componentRef.setInput('value', 1234.5);
        fixture.componentRef.setInput('format', {
            minimumFractionDigits: 1,
            maximumFractionDigits: 1
        });
        fixture.componentRef.setInput('suffix', '%');
        await fixture.whenStable();

        const animated = fixture.nativeElement.querySelector('app-animated-number');
        expect(animated).not.toBeNull();
        expect(animated.querySelector('number-flow')).not.toBeNull();

        fixture.componentRef.setInput('value', '1m 23s');
        await fixture.whenStable();

        expect(fixture.nativeElement.querySelector('app-animated-number')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('1m 23s');
    });

    it('marks positive deltas as good by default', async () => {
        fixture.componentRef.setInput('delta', 10);
        await fixture.whenStable();

        const badge = fixture.nativeElement.querySelector('span.text-xs');
        const flow = badge.querySelector('number-flow-ng') as HTMLElement & { _internals?: ElementInternals };
        expect(flow._internals?.ariaLabel).toBe('+10.0%');
        expect(badge.className).toContain('bg-green-100');
    });

    it('marks negative deltas as good when invertDelta is true', async () => {
        fixture.componentRef.setInput('delta', -10);
        fixture.componentRef.setInput('invertDelta', true);
        await fixture.whenStable();

        const badge = fixture.nativeElement.querySelector('span.text-xs');
        const flow = badge.querySelector('number-flow-ng') as HTMLElement & { _internals?: ElementInternals };
        expect(flow._internals?.ariaLabel).toBe('+10.0%');
        expect(badge.className).toContain('bg-green-100');
    });

    it('formats deltas with the active locale', async () => {
        TestBed.inject(TranslocoService).setActiveLang('de');
        fixture.componentRef.setInput('delta', 10);
        await fixture.whenStable();

        const badge = fixture.nativeElement.querySelector('span.text-xs');
        const flow = badge.querySelector('number-flow-ng') as HTMLElement & { _internals?: ElementInternals };
        expect(flow._internals?.ariaLabel?.replace(/\s/g, ' ')).toBe('+10,0 %');
    });

    it('keeps the delta NumberFlow mounted across updates', async () => {
        fixture.componentRef.setInput('delta', 10);
        await fixture.whenStable();

        const initialFlow = fixture.nativeElement.querySelector('span.text-xs number-flow-ng') as HTMLElement & { _internals?: ElementInternals };
        fixture.componentRef.setInput('delta', 12.5);
        await fixture.whenStable();

        const updatedFlow = fixture.nativeElement.querySelector('span.text-xs number-flow-ng') as HTMLElement & { _internals?: ElementInternals };
        expect(updatedFlow).toBe(initialFlow);
        expect(updatedFlow._internals?.ariaLabel).toBe('+12.5%');
    });

    it('renders numeric durations through the localized animated facade', async () => {
        fixture.componentRef.setInput('value', 83);
        fixture.componentRef.setInput('duration', true);
        await fixture.whenStable();

        const duration = fixture.nativeElement.querySelector('app-animated-duration .hk-animated-duration');
        expect(duration.getAttribute('aria-label')).toBe('1m 23s');
        expect(duration.querySelectorAll('number-flow-ng').length).toBe(2);
    });

    it('keeps the initial loading skeleton instead of mounting an animated value', async () => {
        fixture.componentRef.setInput('value', 42);
        fixture.componentRef.setInput('loading', true);
        await fixture.whenStable();

        expect(fixture.nativeElement.querySelector('p-skeleton')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('app-animated-number')).toBeNull();
    });

    it('does not cue the initial value or expose a live region', async () => {
        fixture.componentRef.setInput('value', 42);
        fixture.componentRef.setInput('updateKey', 1);
        await fixture.whenStable();

        expect(fixture.nativeElement.querySelector('.hk-kpi-card__update-cue')).toBeNull();
        expect(fixture.nativeElement.querySelector('[aria-live]')).toBeNull();
    });

    it('cues a value that changes without an advancing update key', async () => {
        fixture.componentRef.setInput('value', 42);
        await fixture.whenStable();

        fixture.componentRef.setInput('loading', true);
        await fixture.whenStable();
        fixture.componentRef.setInput('loading', false);
        fixture.componentRef.setInput('value', 43);
        await fixture.whenStable();

        expect(fixture.nativeElement.querySelector('.hk-kpi-card__update-cue')).not.toBeNull();
    });

    it('cues an advancing update key that lands on the same value', async () => {
        fixture.componentRef.setInput('value', 42);
        fixture.componentRef.setInput('updateKey', 2);
        await fixture.whenStable();

        fixture.componentRef.setInput('updateKey', 3);
        await fixture.whenStable();

        expect(fixture.nativeElement.querySelector('.hk-kpi-card__update-cue')).not.toBeNull();
    });

    it('suppresses cues when neither the value nor the key moved', async () => {
        fixture.componentRef.setInput('value', 42);
        fixture.componentRef.setInput('updateKey', 2);
        await fixture.whenStable();

        fixture.componentRef.setInput('valueClass', 'text-xl');
        await fixture.whenStable();

        expect(fixture.nativeElement.querySelector('.hk-kpi-card__update-cue')).toBeNull();
    });

    it('suppresses cues while the skeleton is showing', async () => {
        const loadingFixture = TestBed.createComponent(KpiCard);
        loadingFixture.componentRef.setInput('label', 'Visitors');
        loadingFixture.componentRef.setInput('value', 42);
        loadingFixture.componentRef.setInput('loading', true);
        await loadingFixture.whenStable();

        loadingFixture.componentRef.setInput('value', 44);
        loadingFixture.componentRef.setInput('updateKey', 4);
        await loadingFixture.whenStable();

        expect(loadingFixture.nativeElement.querySelector('.hk-kpi-card__update-cue')).toBeNull();
        expect(loadingFixture.nativeElement.querySelector('p-skeleton')).not.toBeNull();
    });

    it('keeps the painted value on screen while a reload for the same subject runs', async () => {
        fixture.componentRef.setInput('value', 42);
        await fixture.whenStable();

        fixture.componentRef.setInput('loading', true);
        await fixture.whenStable();

        expect(fixture.nativeElement.querySelector('p-skeleton')).toBeNull();
        expect(fixture.nativeElement.querySelector('app-animated-number')).not.toBeNull();
    });

    it('falls back to the skeleton when the reload belongs to a different subject', async () => {
        fixture.componentRef.setInput('value', 42);
        await fixture.whenStable();

        TestBed.inject(ReportSubjectService).set('another-site');
        fixture.componentRef.setInput('loading', true);
        await fixture.whenStable();

        expect(fixture.nativeElement.querySelector('p-skeleton')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('app-animated-number')).toBeNull();
    });

    it('restarts the keyed cue for rapid updates and removes it 600ms after the latest change', async () => {
        vi.useFakeTimers();
        try {
            fixture.componentRef.setInput('value', 42);
            fixture.componentRef.setInput('updateKey', 1);
            fixture.detectChanges();

            fixture.componentRef.setInput('value', 43);
            fixture.componentRef.setInput('updateKey', 2);
            fixture.detectChanges();
            const firstCue = fixture.nativeElement.querySelector('.hk-kpi-card__update-cue')?.getAttribute('data-update-cue');

            vi.advanceTimersByTime(100);
            fixture.componentRef.setInput('value', 44);
            fixture.componentRef.setInput('updateKey', 3);
            fixture.detectChanges();
            const secondCue = fixture.nativeElement.querySelector('.hk-kpi-card__update-cue')?.getAttribute('data-update-cue');

            expect(secondCue).not.toBe(firstCue);

            vi.advanceTimersByTime(599);
            fixture.detectChanges();
            expect(fixture.nativeElement.querySelector('.hk-kpi-card__update-cue')).not.toBeNull();

            vi.advanceTimersByTime(1);
            fixture.detectChanges();
            expect(fixture.nativeElement.querySelector('.hk-kpi-card__update-cue')).toBeNull();
        } finally {
            vi.useRealTimers();
        }
    });

    it('cleans up a pending cue timer when destroyed', async () => {
        vi.useFakeTimers();
        try {
            fixture.componentRef.setInput('value', 42);
            fixture.componentRef.setInput('updateKey', 1);
            fixture.detectChanges();
            fixture.componentRef.setInput('value', 43);
            fixture.componentRef.setInput('updateKey', 2);
            fixture.detectChanges();

            fixture.destroy();
            expect(vi.getTimerCount()).toBe(0);
        } finally {
            vi.useRealTimers();
        }
    });

    it('keeps the surface cue hidden during normal motion and neutral under reduced motion', () => {
        const styles = (KpiCard as unknown as { ɵcmp: { styles: string[] } }).ɵcmp.styles.join('\n');

        expect(styles).toContain('background: color-mix(in srgb, var(--p-text-color) 5%, transparent)');
        expect(styles).toContain('display: none');
        expect(styles).toContain('@media (prefers-reduced-motion: reduce)');
        expect(styles).toContain('display: block');
        expect(styles).not.toContain('var(--p-primary-color)');
        expect(styles).not.toContain('@keyframes');
        expect(styles).not.toContain('animation:');
        expect(styles).not.toContain('box-shadow');
        expect(styles).not.toContain('transform:');
    });
});
