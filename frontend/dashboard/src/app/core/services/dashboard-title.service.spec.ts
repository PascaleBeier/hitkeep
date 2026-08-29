import { Component } from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { TestBed } from '@angular/core/testing';
import { Title } from '@angular/platform-browser';
import { provideRouter, Router } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { SiteService } from '@features/sites/services/site.service';
import { DashboardTitleService } from '@services/dashboard-title.service';
import { ShareService } from '@services/share.service';
import { TeamService } from '@services/team.service';

import en from '../../../../public/i18n/en.json';

@Component({
    standalone: true,
    template: ''
})
class DummyComponent {}

const flushEffects = (): void => {
    const testBed = TestBed as unknown as { flushEffects?: () => void; tick?: () => void };
    testBed.flushEffects?.();
    testBed.tick?.();
};

describe('DashboardTitleService', () => {
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                DummyComponent,
                TranslocoTestingModule.forRoot({
                    langs: { en },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                provideHttpClient(),
                provideRouter([
                    { path: 'login', component: DummyComponent, data: { titleKey: 'login.signIn' } },
                    { path: 'dashboard', component: DummyComponent, data: { titleKey: 'nav.dashboard', titleScope: 'site' } },
                    { path: 'overview', component: DummyComponent, data: { titleKey: 'nav.overview', titleScope: 'team' } },
                    { path: 'settings', component: DummyComponent, data: { titleKey: 'settings.user.title' } }
                ])
            ]
        }).compileComponents();

        TestBed.inject(DashboardTitleService);
        flushEffects();
    });

    afterEach(() => {
        TestBed.inject(Title).setTitle('');
    });

    it('sets a localized page title without context for auth routes', async () => {
        await TestBed.inject(Router).navigateByUrl('/login');
        flushEffects();

        expect(TestBed.inject(Title).getTitle()).toBe('Sign in - HitKeep');
    });

    it('resolves the user settings title from the production locale', async () => {
        await TestBed.inject(Router).navigateByUrl('/settings');
        flushEffects();

        expect(TestBed.inject(Title).getTitle()).not.toBe('settings.user.title - HitKeep');
    });

    it('adds the active site and updates when the selected site changes', async () => {
        const siteService = TestBed.inject(SiteService);
        siteService.activeSite.set({ id: 'site-1', user_id: 'user-1', domain: 'alpha.example.com', created_at: '2026-01-01T00:00:00Z' });

        await TestBed.inject(Router).navigateByUrl('/dashboard');
        flushEffects();

        expect(TestBed.inject(Title).getTitle()).toBe('alpha.example.com - Dashboard - HitKeep');

        siteService.activeSite.set({ id: 'site-2', user_id: 'user-1', domain: 'beta.example.com', created_at: '2026-01-01T00:00:00Z' });
        flushEffects();

        expect(TestBed.inject(Title).getTitle()).toBe('beta.example.com - Dashboard - HitKeep');
    });

    it('uses the shared site while share mode is active', async () => {
        TestBed.inject(SiteService).activeSite.set({ id: 'site-1', user_id: 'user-1', domain: 'private.example.com', created_at: '2026-01-01T00:00:00Z' });
        const shareService = TestBed.inject(ShareService);
        shareService.token.set('share-token');
        shareService.site.set({ id: 'site-2', user_id: 'user-1', domain: 'shared.example.com', created_at: '2026-01-01T00:00:00Z' });

        await TestBed.inject(Router).navigateByUrl('/dashboard');
        flushEffects();

        expect(TestBed.inject(Title).getTitle()).toBe('shared.example.com - Dashboard - HitKeep');
    });

    it('adds the active team on team-scoped routes', async () => {
        const teamService = TestBed.inject(TeamService);
        teamService.teams.set([
            {
                id: 'team-1',
                name: 'Growth Team',
                logo_url: '',
                role: 'owner',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);
        teamService.activeTeamId.set('team-1');

        await TestBed.inject(Router).navigateByUrl('/overview');
        flushEffects();

        expect(TestBed.inject(Title).getTitle()).toBe('Growth Team - Overview - HitKeep');
    });
});
