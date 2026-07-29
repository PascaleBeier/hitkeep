import { Component, provideZonelessChangeDetection } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { NavigationCancellationCode, Router, RouterOutlet, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { EMPTY } from 'rxjs';

import { App, isTerminalInitialNavigationCancellation } from './app';

@Component({ standalone: true, template: 'stable' })
class StableRoute {}

@Component({ standalone: true, imports: [RouterOutlet], template: '<router-outlet />' })
class NestedRoute {}

describe('App', () => {
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                App,
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
            providers: [provideZonelessChangeDetection(), provideRouter([])]
        }).compileComponents();
    });

    it('renders a useful loading state before the first route activates', async () => {
        const fixture = TestBed.createComponent(App);
        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        expect(fixture.componentInstance).toBeTruthy();
        expect(element.querySelector('router-outlet')).toBeTruthy();
        expect(element.textContent).toContain('Loading HitKeep');
        expect(element.textContent).toContain('Preparing your workspace.');
        expect(element.querySelector('section')?.getAttribute('role')).toBe('status');
    });

    it('recognizes terminal initial-navigation cancellations but not follow-up navigations', () => {
        expect(isTerminalInitialNavigationCancellation(NavigationCancellationCode.GuardRejected)).toBe(true);
        expect(isTerminalInitialNavigationCancellation(NavigationCancellationCode.NoDataFromResolver)).toBe(true);
        expect(isTerminalInitialNavigationCancellation(NavigationCancellationCode.Aborted)).toBe(true);
        expect(isTerminalInitialNavigationCancellation(NavigationCancellationCode.Redirect)).toBe(false);
        expect(isTerminalInitialNavigationCancellation(NavigationCancellationCode.SupersededByNewNavigation)).toBe(false);
    });

    it('shows a recovery state when real router navigation is guard-rejected', async () => {
        const router = TestBed.inject(Router);
        router.resetConfig([{ path: '', canActivate: [() => false], component: StableRoute }]);
        const fixture = TestBed.createComponent(App);

        await router.navigateByUrl('/');
        await fixture.whenStable();

        expect(fixture.nativeElement.textContent).toContain('Page unavailable');
        expect(fixture.nativeElement.textContent).toContain('Reload HitKeep');
    });

    it('shows a recovery state when a real resolver completes without data', async () => {
        const router = TestBed.inject(Router);
        router.resetConfig([{ path: '', component: StableRoute, resolve: { value: () => EMPTY } }]);
        const fixture = TestBed.createComponent(App);

        await router.navigateByUrl('/');
        await fixture.whenStable();

        expect(fixture.nativeElement.textContent).toContain('Page unavailable');
    });

    it('keeps recovery visible when a nested route is rejected after its parent activates', async () => {
        const router = TestBed.inject(Router);
        router.resetConfig([{ path: '', component: NestedRoute, children: [{ path: '', canActivate: [() => false], component: StableRoute }] }]);
        const fixture = TestBed.createComponent(App);

        await router.navigateByUrl('/');
        await fixture.whenStable();

        expect(fixture.nativeElement.textContent).toContain('Page unavailable');
    });

    it('shows recovery when the error route itself fails after a completed navigation', async () => {
        const router = TestBed.inject(Router);
        router.resetConfig([
            { path: 'stable', component: StableRoute },
            { path: 'error', loadComponent: () => Promise.reject(new Error('error route unavailable')) }
        ]);
        const fixture = TestBed.createComponent(App);

        await router.navigateByUrl('/stable');
        await router.navigateByUrl('/error').catch(() => undefined);
        await fixture.whenStable();

        expect(fixture.nativeElement.textContent).toContain('Page unavailable');
    });
});
