import { Signal, WritableSignal, signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { ReportDefinition, ReportDefinitionInput, ReportScope, Team } from '@models/analytics.types';
import { SiteService } from '@features/sites/services/site.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { ReportDefinitionsService } from '@services/report-definitions.service';
import { TeamService } from '@services/team.service';
import { UserProfileService } from '@services/user-profile.service';
import { ReportSettings } from './report-settings';

describe('ReportSettings', () => {
    let fixture: ComponentFixture<ReportSettings>;
    let reports: WritableSignal<ReportDefinition[]>;
    let serviceMock: {
        reports: WritableSignal<ReportDefinition[]>;
        isLoading: WritableSignal<boolean>;
        load: ReturnType<typeof vi.fn>;
        create: ReturnType<typeof vi.fn>;
        update: ReturnType<typeof vi.fn>;
        delete: ReturnType<typeof vi.fn>;
        preview: ReturnType<typeof vi.fn>;
        testSend: ReturnType<typeof vi.fn>;
        runs: ReturnType<typeof vi.fn>;
        retry: ReturnType<typeof vi.fn>;
        resubscribe: ReturnType<typeof vi.fn>;
        resendConfirmation: ReturnType<typeof vi.fn>;
    };

    type TestAccess = ReportSettings & {
        draft: WritableSignal<ReportDefinitionInput>;
        externalEmailInput: WritableSignal<string>;
        externalEmailError: WritableSignal<string>;
        externalRecipientsLocked: Signal<boolean>;
        openCreate(): void;
        openEdit(report: ReportDefinition): void;
        setScope(scope: ReportScope): void;
        addExternalRecipient(): void;
        reportActions(report: ReportDefinition): { label?: string; items?: unknown }[];
        save(): void;
    };

    const teamID = '00000000-0000-0000-0000-000000000001';
    const userID = '00000000-0000-0000-0000-000000000002';
    const siteID = '00000000-0000-0000-0000-000000000003';
    const freeTeam: Team = {
        id: teamID,
        name: 'Free team',
        logo_url: '',
        role: 'owner',
        created_at: '2026-07-19T00:00:00Z',
        entitlements: {
            max_sites_per_team: 3,
            max_team_members: 3,
            max_retention_days: 60,
            allow_sso: false,
            allow_custom_branding: false,
            allow_external_report_recipients: false
        },
        plan: { code: 'free', name: 'Free' }
    };

    beforeEach(async () => {
        reports = signal([]);
        serviceMock = {
            reports,
            isLoading: signal(false),
            load: vi.fn(() => of([])),
            create: vi.fn((draft: ReportDefinitionInput) => of(reportFixture({ ...draft, id: 'report-created' }))),
            update: vi.fn((id: string, draft: Partial<ReportDefinitionInput>) => of(reportFixture({ ...draft, id }))),
            delete: vi.fn(() => of(undefined)),
            preview: vi.fn(),
            testSend: vi.fn(),
            runs: vi.fn(() => of([])),
            retry: vi.fn(() => of(undefined)),
            resubscribe: vi.fn(() => of(undefined)),
            resendConfirmation: vi.fn(() => of(undefined))
        };

        await TestBed.configureTestingModule({
            imports: [
                ReportSettings,
                NoopAnimationsModule,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                actions: { cancel: 'Cancel', close: 'Close', delete: 'Delete', edit: 'Edit', more: 'More actions', refresh: 'Refresh', remove: 'Remove', save: 'Save' },
                                columns: { actions: 'Actions', name: 'Name', sites: 'Sites', status: 'Status' },
                                searchPlaceholder: 'Search...'
                            },
                            settings: {
                                reports: {
                                    saved: 'Report saved.',
                                    errors: { save: 'The report could not be saved.' },
                                    recipients: {
                                        proNudge: 'External recipients are available on Pro and Business.',
                                        proRequired: 'External report recipients require the Pro plan or higher.',
                                        upgradeToPro: 'Upgrade to Pro'
                                    }
                                }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }
                })
            ],
            providers: [
                provideRouter([]),
                { provide: ReportDefinitionsService, useValue: serviceMock },
                { provide: DashboardBootstrapService, useValue: { cloudHosted: signal(true), status: signal({ mail_delivery: { available: true } }) } },
                {
                    provide: TeamService,
                    useValue: {
                        activeTeam: signal(freeTeam),
                        teams: signal([freeTeam]),
                        activeTeamId: signal(teamID),
                        listTeamMembers: () => of([])
                    }
                },
                { provide: UserProfileService, useValue: { profile: signal({ id: userID }) } },
                { provide: SiteService, useValue: { sites: signal([{ id: siteID, domain: 'example.test' }]) } }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(ReportSettings);
        await fixture.whenStable();
    });

    afterEach(() => {
        document.querySelectorAll('.p-dialog-mask, .p-dialog, .p-confirmdialog').forEach((element) => element.remove());
    });

    it('shows the Pro nudge and prevents adding an external address on Free', async () => {
        const component = fixture.componentInstance as TestAccess;
        component.openCreate();
        component.setScope('team');
        component.externalEmailInput.set('client@example.test');
        component.addExternalRecipient();
        await fixture.whenStable();

        const nudge = document.body.querySelector('[data-testid="external-recipient-pro-nudge"]') as HTMLElement | null;
        const externalInput = document.body.querySelector('[data-testid="report-external-recipient"]');
        expect(component.externalRecipientsLocked()).toBe(true);
        expect(component.draft().external_recipient_emails).toEqual([]);
        expect(component.externalEmailError()).toBe('External report recipients require the Pro plan or higher.');
        expect(nudge?.textContent).toContain('Upgrade to Pro');
        expect(externalInput).toBeNull();
    });

    it('renders reports in the established searchable table with row actions', async () => {
        reports.set([reportFixture()]);
        await fixture.whenStable();

        expect(fixture.nativeElement.querySelector('[data-testid="report-table"]')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('[data-testid="report-search"]')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('app-table-row-actions')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('article.report-card')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Morning growth report');
    });

    it('uses the searchable shared site option in the report dialog', async () => {
        const component = fixture.componentInstance as TestAccess;
        component.openCreate();
        await fixture.whenStable();

        const siteSelect = document.body.querySelector('[data-testid="report-sites"]') as HTMLElement | null;
        siteSelect?.querySelector<HTMLButtonElement>('.p-select-dropdown')?.click();
        await fixture.whenStable();

        expect(siteSelect).not.toBeNull();
        expect(document.body.querySelector('.p-select-filter')).not.toBeNull();
        expect(document.body.querySelector('app-site-select-option')).not.toBeNull();
    });

    it('keeps external recipient controls as flat, interactive row actions', () => {
        const component = fixture.componentInstance as TestAccess;
        const report = {
            ...reportFixture({ scope: 'team' }),
            tenant_id: teamID,
            recipients: [
                { id: 'recipient-1', kind: 'member' as const, user_id: userID, email: 'owner@example.test', status: 'confirmed' as const },
                { id: 'recipient-2', kind: 'external' as const, email: 'client@example.test', status: 'pending_confirmation' as const }
            ]
        };

        const actions = component.reportActions(report);

        expect(actions.every((action) => action.items === undefined)).toBe(true);
        expect(actions.some((action) => action['label']?.includes('client@example.test'))).toBe(true);
        expect(actions.some((action) => action['label'] === 'Edit')).toBe(true);
    });

    it('keeps save errors inline inside the report dialog', async () => {
        serviceMock.update.mockReturnValueOnce(throwError(() => new Error('save failed')));
        const component = fixture.componentInstance as TestAccess;
        component.openEdit(reportFixture());
        component.save();
        await fixture.whenStable();

        const feedback = document.body.querySelector('[data-testid="report-dialog-feedback"]') as HTMLElement | null;
        expect(feedback?.textContent).toContain('The report could not be saved.');
        expect(document.body.querySelector('[role="dialog"]')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('[data-testid="report-list-feedback"]')).toBeNull();
    });

    it('shows save success inline with the report list', async () => {
        reports.set([reportFixture()]);
        const component = fixture.componentInstance as TestAccess;
        component.openEdit(reportFixture());
        component.save();
        await fixture.whenStable();

        const feedback = fixture.nativeElement.querySelector('[data-testid="report-list-feedback"]') as HTMLElement | null;
        expect(feedback?.textContent).toContain('Report saved.');
        expect(document.body.querySelector('[data-testid="report-dialog-feedback"]')).toBeNull();
    });

    function reportFixture(overrides: Partial<ReportDefinitionInput> & { id?: string } = {}): ReportDefinition {
        return {
            id: overrides.id ?? 'report-1',
            owner_user_id: userID,
            name: overrides.name ?? 'Morning growth report',
            scope: overrides.scope ?? 'personal',
            preset: overrides.preset ?? 'site_summary',
            site_mode: overrides.site_mode ?? 'selected',
            sites: [{ id: siteID, domain: 'example.test' }],
            recipients: [{ id: 'recipient-1', kind: 'member', user_id: userID, email: 'owner@example.test', status: 'confirmed' }],
            schedule: overrides.schedule ?? { frequency: 'daily', timezone: 'Europe/Berlin', local_time: '08:00' },
            status: overrides.status ?? 'draft',
            source: 'v2',
            consent_version: 1,
            created_at: '2026-07-19T00:00:00Z',
            updated_at: '2026-07-19T00:00:00Z'
        };
    }
});
