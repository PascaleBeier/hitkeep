import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { of } from 'rxjs';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { vi } from 'vitest';
import { TeamAuditPage } from './team-audit';
import { TeamService } from '@services/team.service';
import { AuditPresentationService } from '@services/audit-presentation.service';

interface MockWithCalls {
    mock: {
        calls: unknown[][];
    };
}

describe('TeamAuditPage', () => {
    let fixture: ComponentFixture<TeamAuditPage>;
    const activeTeam = signal({
        id: 'team-1',
        name: 'Acme',
        logo_url: '',
        role: 'owner' as const,
        created_at: '2026-01-01T00:00:00Z'
    });

    const auditResponse = {
        entries: [
            {
                id: 'audit-1',
                team_id: 'team-1',
                action: 'member.added',
                details: 'Added user',
                actor_email: 'owner@example.com',
                ip_address: '203.0.113.10',
                ip_country_code: 'US',
                created_at: '2026-01-03T00:00:00Z'
            }
        ],
        total: 1,
        limit: 25,
        offset: 0,
        has_more: false
    };

    const teamServiceMock = {
        activeTeam,
        listTeamAudit: vi.fn(() => of(auditResponse))
    };

    beforeEach(async () => {
        teamServiceMock.listTeamAudit.mockClear();
        teamServiceMock.listTeamAudit.mockReturnValue(of(auditResponse));
        activeTeam.set({
            id: 'team-1',
            name: 'Acme',
            logo_url: '',
            role: 'owner' as const,
            created_at: '2026-01-01T00:00:00Z'
        });

        await TestBed.configureTestingModule({
            imports: [
                TeamAuditPage,
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                { provide: TeamService, useValue: teamServiceMock },
                {
                    provide: AuditPresentationService,
                    useValue: {
                        actionOptions: vi.fn(() => []),
                        outcomeOptions: vi.fn(() => []),
                        targetTypeOptions: vi.fn(() => []),
                        actionLabel: vi.fn((action: string) => action),
                        targetTypeLabel: vi.fn((targetType: string) => targetType || '-'),
                        targetLabel: vi.fn((row: { target_label?: string; target_email?: string; target_id?: string }) => row.target_label || row.target_email || row.target_id || '-'),
                        actorLabel: vi.fn((row: { actor_email_snapshot?: string; actor_email?: string }) => row.actor_email_snapshot || row.actor_email || 'Unknown'),
                        roleLabel: vi.fn((role: string) => role),
                        outcomeLabel: vi.fn((outcome: string) => outcome || '-'),
                        actionSeverity: vi.fn(() => 'secondary'),
                        outcomeSeverity: vi.fn(() => 'secondary')
                    }
                },
                provideTranslocoLocale({
                    langToLocaleMapping: {
                        en: 'en-US'
                    }
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(TeamAuditPage);
        fixture.detectChanges();
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('should load team audit rows for owner/admin', () => {
        expect(teamServiceMock.listTeamAudit).toHaveBeenCalled();
        expect((teamServiceMock.listTeamAudit as unknown as MockWithCalls).mock.calls[0][0]).toBe('team-1');
        expect((teamServiceMock.listTeamAudit as unknown as MockWithCalls).mock.calls[0][1]).toEqual({ limit: 25, offset: 0 });
    });

    it('should refetch when the shared table query changes', () => {
        const component = fixture.componentInstance;

        component['onQueryChange']({ action: 'member.added', outcome: 'success', target_type: 'user', query: 'owner@example.com', limit: 50, offset: 0 });
        fixture.detectChanges();

        expect((teamServiceMock.listTeamAudit as unknown as MockWithCalls).mock.calls.at(-1)?.[1]).toEqual({ action: 'member.added', outcome: 'success', target_type: 'user', query: 'owner@example.com', limit: 50, offset: 0 });
    });
});
