import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { Router, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { TEAM_CAPABILITIES } from '@core/access/capabilities';
import { SiteService } from '@features/sites/services/site.service';
import { AccessService } from '@services/access.service';
import { PermissionService } from '@services/permission.service';
import { TeamService } from '@services/team.service';
import { TeamDangerZonePage } from './team-danger-zone';

describe('TeamDangerZonePage', () => {
    let fixture: ComponentFixture<TeamDangerZonePage>;

    const teamServiceMock = {
        activeTeamId: signal('team-1'),
        activeTeam: signal({
            id: 'team-1',
            name: 'Acme',
            logo_url: '',
            role: 'owner' as const,
            created_at: '2026-01-01T00:00:00Z'
        }),
        leaveTeam: vi.fn((teamID: string) => {
            void teamID;
            return of({ status: 'ok', active_team_id: 'team-2' });
        }),
        archiveTeam: vi.fn((teamID: string) => {
            void teamID;
            return of({ status: 'ok', active_team_id: 'team-2' });
        }),
        loadTeams: vi.fn(() => of({ active_team_id: 'team-2', teams: [] }))
    };

    const accessServiceMock = {
        canActiveTeam: vi.fn((capability: string) => capability === TEAM_CAPABILITIES.archive)
    };

    const siteServiceMock = {
        sites: signal([]),
        activeSite: signal(null),
        loadSites: vi.fn()
    };

    const permissionServiceMock = {
        loadPermissions: vi.fn(() => of({}))
    };

    beforeEach(async () => {
        vi.clearAllMocks();
        accessServiceMock.canActiveTeam.mockImplementation((capability: string) => capability === TEAM_CAPABILITIES.archive);
        await TestBed.configureTestingModule({
            imports: [
                TeamDangerZonePage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            admin: {
                                team: {
                                    tabs: {
                                        dangerZone: 'Danger Zone'
                                    },
                                    danger: {
                                        leaveConfirmTitle: 'Leave team',
                                        archiveConfirmTitle: 'Archive team',
                                        confirmNameLabel: 'Type {{name}} to confirm'
                                    },
                                    settings: {
                                        leaveSectionTitle: 'Leave team',
                                        leaveSectionDescription: 'Remove yourself from this team and switch to another team you belong to.',
                                        leaveAction: 'Leave team',
                                        leaveSuccess: 'You left the team.',
                                        archiveSectionTitle: 'Archive team',
                                        archiveSectionDescription: 'Archive this team after all of its sites have been transferred or removed.',
                                        archiveSectionHint: 'Transfer or delete every site in this team first.',
                                        archiveAction: 'Archive team',
                                        archiveSuccess: 'The team was archived.'
                                    }
                                }
                            },
                            common: {
                                actions: {
                                    cancel: 'Cancel'
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
            providers: [
                provideRouter([]),
                { provide: TeamService, useValue: teamServiceMock },
                { provide: AccessService, useValue: accessServiceMock },
                { provide: SiteService, useValue: siteServiceMock },
                { provide: PermissionService, useValue: permissionServiceMock }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(TeamDangerZonePage);
        fixture.detectChanges();
    });

    afterEach(() => {
        document.querySelectorAll('.p-dialog-mask, .p-dialog').forEach((element) => element.remove());
    });

    it('renders leave and archive actions as danger cards without inline confirmation inputs', () => {
        expect(fixture.nativeElement.textContent).toContain('Leave team');
        expect(fixture.nativeElement.textContent).toContain('Archive team');
        expect(fixture.nativeElement.querySelectorAll('.settings-card--danger').length).toBe(2);
        expect(fixture.nativeElement.querySelector('#team-danger-confirm')).toBeNull();
    });

    it('requires the team name in the confirmation dialog before leaving the team', async () => {
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigateByUrl').mockResolvedValue(true);

        clickButton(fixture.nativeElement, 'Leave team');
        fixture.detectChanges();
        await fixture.whenStable();

        const input = document.body.querySelector('#team-danger-confirm') as HTMLInputElement | null;
        expect(input).toBeTruthy();
        expect(document.body.textContent).not.toContain('leave this team.');
        expect(document.body.textContent).toContain('Type Acme to confirm');
        expect(dialogPrimaryButton('Leave team').disabled).toBe(true);

        input!.value = 'wrong';
        input!.dispatchEvent(new Event('input'));
        fixture.detectChanges();
        expect(dialogPrimaryButton('Leave team').disabled).toBe(true);

        input!.value = 'Acme';
        input!.dispatchEvent(new Event('input'));
        fixture.detectChanges();
        expect(dialogPrimaryButton('Leave team').disabled).toBe(false);

        dialogPrimaryButton('Leave team').click();

        expect(teamServiceMock.leaveTeam).toHaveBeenCalledWith('team-1');
        expect(permissionServiceMock.loadPermissions).toHaveBeenCalled();
        expect(navigateSpy).toHaveBeenCalledWith('/dashboard');
    });

    it('hides archive when the active team lacks archive capability', () => {
        accessServiceMock.canActiveTeam.mockReturnValue(false);
        fixture = TestBed.createComponent(TeamDangerZonePage);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Leave team');
        expect(fixture.nativeElement.textContent).not.toContain('Archive team');
    });

    it('keeps the archive confirmation body to a single name prompt', async () => {
        clickButton(fixture.nativeElement, 'Archive team');
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.body.textContent).toContain('Archive team');
        expect(document.body.textContent).not.toContain('archive this team.');
        expect(document.body.textContent).toContain('Type Acme to confirm');
    });

    function clickButton(root: HTMLElement, label: string): void {
        const button = Array.from(root.querySelectorAll('button')).find((candidate) => candidate.textContent?.includes(label)) as HTMLButtonElement | undefined;
        if (!button) {
            throw new Error(`Button ${label} not found`);
        }
        button.click();
    }

    function dialogPrimaryButton(label: string): HTMLButtonElement {
        const buttons = Array.from(document.body.querySelectorAll('.dialog-shell-footer button')) as HTMLButtonElement[];
        const button = buttons.find((candidate) => candidate.textContent?.includes(label));
        if (!button) {
            throw new Error(`Dialog button ${label} not found`);
        }
        return button;
    }
});
