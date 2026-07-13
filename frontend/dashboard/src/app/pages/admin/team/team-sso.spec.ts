import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { Team, TeamSSOConfig, UpdateTeamSSORequest } from '@models/analytics.types';
import { TeamService } from '@services/team.service';
import { TeamSSOPage } from './team-sso';

describe('TeamSSOPage', () => {
    let fixture: ComponentFixture<TeamSSOPage>;
    let component: TeamSSOPage;
    const activeTeam = signal<Team>({ id: 'team-1', name: 'Acme', logo_url: '', role: 'owner', created_at: '2026-01-01T00:00:00Z' });
    const config: TeamSSOConfig = {
        provider_type: 'oidc',
        issuer_url: 'https://identity.example.com',
        client_id: 'hitkeep',
        client_secret_configured: true,
        allowed_domains: ['example.com'],
        email_claim: 'email',
        display_name_claim: 'name',
        enabled: true,
        callback_url: 'https://analytics.example.com/api/auth/sso/callback'
    };
    const teamServiceMock = {
        activeTeam,
        getTeamSSO: vi.fn((teamID: string) => {
            void teamID;
            return of(config);
        }),
        updateTeamSSO: vi.fn((teamID: string, payload: UpdateTeamSSORequest) => {
            void teamID;
            void payload;
            return of(config);
        }),
        testTeamSSO: vi.fn((teamID: string) => {
            void teamID;
            return of({ status: 'ok' });
        })
    };

    beforeEach(async () => {
        vi.clearAllMocks();
        await TestBed.configureTestingModule({
            imports: [
                TeamSSOPage,
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ],
            providers: [{ provide: TeamService, useValue: teamServiceMock }]
        }).compileComponents();

        fixture = TestBed.createComponent(TeamSSOPage);
        component = fixture.componentInstance;
        await fixture.whenStable();
    });

    it('loads a redacted provider configuration without placing the stored secret in the form', () => {
        expect(teamServiceMock.getTeamSSO).toHaveBeenCalledWith('team-1');
        expect(component['model']().issuerURL).toBe('https://identity.example.com');
        expect(component['model']().clientSecret).toBe('');
        expect(component['clientSecretConfigured']()).toBe(true);
        expect(component['callbackURL']()).toBe('https://analytics.example.com/api/auth/sso/callback');
    });

    it('normalizes domains and keeps a blank secret when saving an existing connection', () => {
        component['model'].update((value) => ({ ...value, allowedDomains: 'example.com, example.org\nanalytics.example' }));

        component['saveSettings']();

        const [teamID, payload] = teamServiceMock.updateTeamSSO.mock.calls[0] as [string, UpdateTeamSSORequest];
        expect(teamID).toBe('team-1');
        expect(payload.client_secret).toBe('');
        expect(payload.allowed_domains).toEqual(['example.com', 'example.org', 'analytics.example']);
        expect(payload.enabled).toBe(true);
        expect(component['successKey']()).toBe('admin.team.sso.saveSuccess');
    });

    it('tests only a previously saved connection', () => {
        component['testConnection']();

        expect(teamServiceMock.testTeamSSO).toHaveBeenCalledWith('team-1');
        expect(component['successKey']()).toBe('admin.team.sso.testSuccess');

        component['clientSecretConfigured'].set(false);
        component['testConnection']();
        expect(teamServiceMock.testTeamSSO).toHaveBeenCalledTimes(1);
    });
});
