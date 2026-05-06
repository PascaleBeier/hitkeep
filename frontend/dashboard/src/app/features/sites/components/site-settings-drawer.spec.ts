import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { provideNoopAnimations } from '@angular/platform-browser/animations';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { TeamService } from '@services/team.service';
import { SiteService } from '@features/sites/services/site.service';
import { SiteSettingsDrawer } from './site-settings-drawer';

describe('SiteSettingsDrawer', () => {
    let fixture: ComponentFixture<SiteSettingsDrawer>;
    let httpMock: HttpTestingController;

    const site = {
        id: 'site-1',
        user_id: 'user-1',
        domain: 'example.com',
        created_at: '2026-01-01T00:00:00Z'
    };

    const siteServiceMock = {
        sites: signal([site]),
        activeSite: signal(site),
        loadSites: () => undefined
    };

    const teamServiceMock = {
        activeTeamId: signal('team-1'),
        teams: signal([
            {
                id: 'team-1',
                name: 'Current team',
                logo_url: '',
                role: 'owner' as const,
                created_at: '2026-01-01T00:00:00Z'
            },
            {
                id: 'team-2',
                name: 'Destination team',
                logo_url: '',
                role: 'admin' as const,
                created_at: '2026-01-02T00:00:00Z'
            }
        ])
    };

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                SiteSettingsDrawer,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            sites: {
                                settings: {
                                    title: 'Site settings',
                                    breadcrumb: {
                                        sites: 'Sites',
                                        settings: 'Settings'
                                    },
                                    tabs: {
                                        general: 'General',
                                        tracking: 'Tracking',
                                        filtering: 'Filtering',
                                        retention: 'Retention',
                                        team: 'Team',
                                        dangerZone: 'Danger zone'
                                    }
                                },
                                team: {
                                    transfer: {
                                        title: 'Transfer site',
                                        description: 'Move this site and its analytics data into another team you can administer.',
                                        teamLabel: 'Destination team',
                                        teamPlaceholder: 'Select a destination team',
                                        action: 'Transfer site'
                                    }
                                }
                            },
                            common: {
                                emailAddress: 'Email address',
                                columns: {
                                    role: 'Role',
                                    actions: 'Actions',
                                    email: 'Email',
                                    added: 'Added'
                                },
                                searchPlaceholder: 'Search...'
                            },
                            roles: {
                                owner: 'Owner',
                                admin: 'Admin',
                                editor: 'Editor',
                                viewer: 'Viewer'
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                provideHttpClient(),
                provideHttpClientTesting(),
                provideRouter([]),
                provideNoopAnimations(),
                { provide: TeamService, useValue: teamServiceMock },
                { provide: SiteService, useValue: siteServiceMock },
                provideTranslocoLocale({
                    langToLocaleMapping: {
                        en: 'en-US'
                    }
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(SiteSettingsDrawer);
        fixture.componentRef.setInput('visible', true);
        fixture.componentRef.setInput('site', site);
        fixture.detectChanges();

        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('switches to the team tab and renders the transfer panel', async () => {
        const teamTab = fixture.nativeElement.querySelector('[role="tab"][aria-controls$="_4"]') as HTMLElement | null;
        expect(teamTab?.textContent).toContain('Team');

        teamTab?.click();
        fixture.detectChanges();

        httpMock.expectOne('/api/sites/site-1/members').flush([]);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Transfer site');
        expect(fixture.nativeElement.textContent).toContain('Destination team');
    });
});
