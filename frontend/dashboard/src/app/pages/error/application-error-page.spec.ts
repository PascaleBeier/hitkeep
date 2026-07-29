import { Location } from '@angular/common';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { APPLICATION_ERROR_STATE_KEY } from '@services/application-error-navigation.service';

import { ApplicationErrorPage } from './application-error-page';

describe('ApplicationErrorPage', () => {
    let routeData: Record<string, unknown>;
    let historyState: Record<string, unknown>;
    const location = {
        getState: vi.fn(() => historyState),
        back: vi.fn(),
        replaceState: vi.fn(),
        historyGo: vi.fn()
    };

    beforeEach(async () => {
        routeData = {};
        historyState = {};
        vi.clearAllMocks();

        await TestBed.configureTestingModule({
            imports: [
                ApplicationErrorPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            applicationError: {
                                notFound: { title: 'Page not found', message: 'This page moved.' },
                                offline: { title: 'Offline', message: 'Check your connection.' },
                                client: { title: 'Request rejected', message: 'The request failed.' },
                                server: { title: 'Temporarily unavailable', message: 'Try again soon.' },
                                navigation: { title: 'Page failed to load', message: 'Reload this page.' },
                                generic: { title: 'Something went wrong', message: 'Reload HitKeep.' },
                                status: { http: 'HTTP {{status}}', offline: 'Offline', navigation: 'Navigation error' },
                                context: {
                                    setupStatus: 'Setup status unavailable.',
                                    dashboardBootstrap: 'Dashboard session unavailable.',
                                    cloudSignupStatus: 'Cloud signup status unavailable.',
                                    cloudSignupProfile: 'Cloud session unavailable.'
                                },
                                actions: { tryAgain: 'Try again', reloadPage: 'Reload page', dashboard: 'Go to dashboard', goBack: 'Go back' }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ],
            providers: [
                provideRouter([]),
                { provide: Location, useValue: location },
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: {
                            get data() {
                                return routeData;
                            }
                        }
                    }
                }
            ]
        }).compileComponents();
    });

    it('renders a real 404 page with recovery actions', async () => {
        routeData = { applicationErrorKind: 'not-found' };
        const fixture = TestBed.createComponent(ApplicationErrorPage);
        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        expect(element.textContent).toContain('HTTP 404');
        expect(element.textContent).toContain('Page not found');
        expect(element.textContent).toContain('Go to dashboard');
        expect(element.textContent).toContain('Go back');
        expect(element.querySelector('section')?.getAttribute('role')).toBe('alert');
        expect(element.querySelector('section')?.getAttribute('aria-live')).toBe('assertive');

        const buttons = element.querySelectorAll('button');
        buttons.item(1).click();
        expect(location.back).toHaveBeenCalledTimes(1);
    });

    it('shows only safe status and context for a fatal bootstrap response', async () => {
        historyState = {
            [APPLICATION_ERROR_STATE_KEY]: {
                version: 1,
                kind: 'server',
                context: 'dashboard-bootstrap',
                returnUrl: '/dashboard?range=7d',
                status: 503,
                error: 'private upstream response'
            }
        };
        const fixture = TestBed.createComponent(ApplicationErrorPage);
        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        expect(element.textContent).toContain('HTTP 503');
        expect(element.textContent).toContain('Temporarily unavailable');
        expect(element.textContent).toContain('Dashboard session unavailable.');
        expect(element.textContent).not.toContain('private upstream response');

        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        element.querySelector('button')?.click();
        expect(navigate).toHaveBeenCalledWith('/dashboard', { replaceUrl: true });
    });
});
