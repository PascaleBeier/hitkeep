import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { TeamService } from '@services/team.service';
import { TeamBrandingPage } from './team-branding';

describe('TeamBrandingPage', () => {
    let fixture: ComponentFixture<TeamBrandingPage>;

    const teamServiceMock = {
        activeTeam: signal({
            id: 'team-1',
            name: 'Acme',
            logo_url: 'https://cdn.example.com/acme.png',
            role: 'owner' as const,
            created_at: '2026-01-01T00:00:00Z'
        }),
        updateTeam: vi.fn((teamID: string, payload: { name: string; logo_url: string }) => {
            void teamID;
            void payload;
            return of({ status: 'ok' });
        })
    };

    beforeEach(async () => {
        vi.clearAllMocks();
        await TestBed.configureTestingModule({
            imports: [
                TeamBrandingPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            admin: {
                                team: {
                                    settings: {
                                        nameSectionTitle: 'Team name',
                                        nameSectionDescription: 'The display name for this team.',
                                        nameRequired: 'Team name is required.',
                                        logoSectionTitle: 'Team logo',
                                        logoSectionDescription: 'A URL pointing to this team logo.',
                                        logoPlaceholder: 'https://example.com/logo.png',
                                        logoPreviewAlt: 'Team logo preview',
                                        saveAction: 'Save branding',
                                        saveSuccess: 'Saved.',
                                        saveError: 'Could not save.'
                                    }
                                }
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
            providers: [{ provide: TeamService, useValue: teamServiceMock }]
        }).compileComponents();

        fixture = TestBed.createComponent(TeamBrandingPage);
        fixture.detectChanges();
    });

    it('owns team name and logo editing without danger-zone actions', () => {
        expect(fixture.nativeElement.textContent).toContain('Team name');
        expect(fixture.nativeElement.textContent).toContain('Team logo');
        expect(fixture.nativeElement.textContent).not.toContain('Leave team');
        expect(fixture.nativeElement.textContent).not.toContain('Archive team');
    });

    it('saves branding changes for the active team', () => {
        const component = fixture.componentInstance;
        component['form'].controls.name.setValue('Acme Cloud');
        component['form'].controls.logo_url.setValue('https://cdn.example.com/cloud.png');

        component['saveSettings']();

        expect(teamServiceMock.updateTeam).toHaveBeenCalledWith('team-1', {
            name: 'Acme Cloud',
            logo_url: 'https://cdn.example.com/cloud.png'
        });
        expect(fixture.nativeElement.textContent).not.toContain('Could not save.');
    });
});
