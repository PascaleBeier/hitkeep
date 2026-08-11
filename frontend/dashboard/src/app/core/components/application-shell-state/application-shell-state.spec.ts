import { provideZonelessChangeDetection } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { ApplicationShellState } from './application-shell-state';

describe('ApplicationShellState', () => {
    let component: ApplicationShellState;
    let fixture: ComponentFixture<ApplicationShellState>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                ApplicationShellState,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            applicationError: {
                                loading: { title: 'Loading HitKeep', message: 'Preparing your workspace.' },
                                routeUnavailable: { title: 'Page unavailable', message: 'Reload HitKeep.' },
                                actions: { reloadApplication: 'Reload HitKeep' }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ],
            providers: [provideZonelessChangeDetection()]
        }).compileComponents();

        fixture = TestBed.createComponent(ApplicationShellState);
        component = fixture.componentInstance;
        fixture.componentRef.setInput('titleKey', 'applicationError.loading.title');
        fixture.componentRef.setInput('messageKey', 'applicationError.loading.message');
        await fixture.whenStable();
    });

    it('renders a lightweight accessible status state without rich UI primitives', async () => {
        fixture.componentRef.setInput('titleKey', 'applicationError.loading.title');
        fixture.componentRef.setInput('messageKey', 'applicationError.loading.message');
        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        const state = element.querySelector('section');

        expect(component).toBeTruthy();
        expect(state?.getAttribute('role')).toBe('status');
        expect(state?.getAttribute('aria-live')).toBe('polite');
        expect(element.textContent).toContain('Loading HitKeep');
        expect(element.textContent).toContain('Preparing your workspace.');
        expect(element.querySelector('app-auth-card')).toBeNull();
        expect(element.querySelector('p-button')).toBeNull();
    });

    it('emits its recovery action from a native button', async () => {
        fixture.componentRef.setInput('titleKey', 'applicationError.routeUnavailable.title');
        fixture.componentRef.setInput('messageKey', 'applicationError.routeUnavailable.message');
        fixture.componentRef.setInput('danger', true);
        fixture.componentRef.setInput('primaryActionLabelKey', 'applicationError.actions.reloadApplication');
        await fixture.whenStable();

        const action = vi.fn();
        component.primaryAction.subscribe(action);
        (fixture.nativeElement.querySelector('button') as HTMLButtonElement).click();

        expect(action).toHaveBeenCalledTimes(1);
        expect(fixture.nativeElement.querySelector('section')?.getAttribute('role')).toBe('alert');
    });
});
