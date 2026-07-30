import { Signal, WritableSignal, signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { Subject, of, throwError } from 'rxjs';
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
    let listTeamMembers: ReturnType<typeof vi.fn>;

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
        refresh(): void;
        setSearch(value: string): void;
        setMobileSortField(field: 'name' | 'schedule' | 'status' | 'nextRun' | 'lastOutcome'): void;
        toggleMobileSortOrder(): void;
        onMobilePage(event: { first: number; rows: number }): void;
        previewReport(): void;
        openHistory(report: ReportDefinition): void;
        filteredReportRows: Signal<{ report: ReportDefinition }[]>;
        mobilePageRows: Signal<{ report: ReportDefinition }[]>;
        previewLoading: Signal<boolean>;
        membersError: Signal<boolean>;
        historyLoading: Signal<boolean>;
        initialLoadError: Signal<boolean>;
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
        listTeamMembers = vi.fn(() => of([]));
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
                                    emptyFiltered: 'No reports match your search.',
                                    notAvailable: 'Not available',
                                    siteCount: '{{count}} sites',
                                    sites: { label: 'Sites' },
                                    status: { draft: 'Draft', active: 'Active', paused: 'Paused' },
                                    runStatus: { completed: 'Accepted by mail server' },
                                    recipientStatus: { confirmed: 'Confirmed' },
                                    filteredEmpty: { title: 'No matching reports' },
                                    errors: {
                                        loadTitle: 'Reports are unavailable',
                                        load: 'Reports could not be loaded.',
                                        save: 'The report could not be saved.',
                                        preview: 'The preview could not be generated.',
                                        members: 'Team members could not be loaded.',
                                        history: 'Delivery history could not be loaded.'
                                    },
                                    recipients: {
                                        proNudge: 'External recipients are available on Pro and Business.',
                                        proRequired: 'External report recipients require the Pro plan or higher.',
                                        removeEmail: 'Remove {{ email }}',
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
                        listTeamMembers
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
        expect(fixture.nativeElement.querySelector('[data-testid="report-mobile-list"]')).not.toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Morning growth report');
    });

    it('reuses the shared site-scope summary for report sites', async () => {
        reports.set([
            {
                ...reportFixture(),
                sites: [
                    { id: siteID, domain: 'example.test' },
                    { id: 'site-2', domain: 'docs.example.test' }
                ]
            }
        ]);
        await fixture.whenStable();

        const summaries = fixture.nativeElement.querySelectorAll('app-site-scope-summary');
        expect(summaries.length).toBeGreaterThan(0);
        expect(summaries[0].textContent).toContain('2 sites');
        expect(summaries[0].querySelector('.pi-sitemap')).not.toBeNull();
    });

    it('uses the established compact status and outcome indicators in the desktop table', async () => {
        reports.set([
            {
                ...reportFixture({ status: 'active' }),
                last_outcome: {
                    run_id: 'run-1',
                    status: 'completed',
                    scheduled_at: '2026-07-20T08:00:00Z',
                    completed_at: '2026-07-20T08:01:00Z'
                }
            }
        ]);
        await fixture.whenStable();

        const desktopTable = fixture.nativeElement.querySelector('.report-desktop-table') as HTMLElement;
        const status = desktopTable.querySelector('[data-testid="report-status-indicator"]') as HTMLElement;
        const outcome = desktopTable.querySelector('[data-testid="report-outcome-indicator"]') as HTMLElement;
        const recipient = desktopTable.querySelector('[data-testid="report-recipient-status-indicator"]') as HTMLElement;

        expect(status.classList).toContain('hk-status-icon--ok');
        expect(status.getAttribute('aria-label')).toBe('Active');
        expect(status.getAttribute('title')).toBe('Active');
        expect(outcome.classList).toContain('hk-status-icon--ok');
        expect(outcome.getAttribute('aria-label')).toBe('Accepted by mail server');
        expect(outcome.getAttribute('title')).toBe('Accepted by mail server');
        expect(recipient.classList).toContain('hk-status-icon--ok');
        expect(recipient.getAttribute('aria-label')).toBe('Confirmed');
        expect(recipient.getAttribute('title')).toBe('Confirmed');
        expect(desktopTable.querySelectorAll('thead th').length).toBe(7);
    });

    it('shares search state across desktop and mobile rows', async () => {
        reports.set([reportFixture(), reportFixture({ id: 'report-2', name: 'Weekly portfolio review' })]);
        const component = fixture.componentInstance as TestAccess;
        component.setSearch('portfolio');
        await fixture.whenStable();

        expect(component.filteredReportRows().map((row) => row.report.name)).toEqual(['Weekly portfolio review']);
        expect(fixture.nativeElement.querySelectorAll('[data-testid="report-mobile-row"]').length).toBe(1);
        expect(fixture.nativeElement.textContent).toContain('Weekly portfolio review');
        expect(fixture.nativeElement.textContent).not.toContain('Morning growth report');
    });

    it('sorts and paginates the mobile summary list', () => {
        reports.set(Array.from({ length: 12 }, (_, index) => reportFixture({ id: `report-${index}`, name: `Report ${String(index).padStart(2, '0')}` })));
        const component = fixture.componentInstance as TestAccess;
        component.setMobileSortField('name');
        component.toggleMobileSortOrder();
        component.onMobilePage({ first: 10, rows: 10 });

        expect(component.mobilePageRows().map((row) => row.report.name)).toEqual(['Report 01', 'Report 00']);
    });

    it('wraps long report identity, domains, and recipient addresses into the mobile summary', async () => {
        const longDomain = 'analytics-for-a-very-long-client-domain-that-must-wrap.example.test';
        const longEmail = 'a-very-long-recipient-address-that-must-wrap@example.test';
        reports.set([
            {
                ...reportFixture({ name: 'A report name that remains reachable on a very narrow screen' }),
                sites: [{ id: siteID, domain: longDomain }],
                recipients: [{ id: 'recipient-long', kind: 'external', email: longEmail, status: 'pending_confirmation' }]
            }
        ]);
        await fixture.whenStable();

        const mobile = fixture.nativeElement.querySelector('[data-testid="report-mobile-row"]') as HTMLElement;
        expect(mobile.textContent).toContain(longDomain);
        expect(mobile.textContent).toContain(longEmail);
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

    it('uses the shared removable chip with a recipient-specific accessible label', async () => {
        const component = fixture.componentInstance as TestAccess;
        component.openCreate();
        component.draft.update((draft) => ({
            ...draft,
            scope: 'team',
            tenant_id: teamID,
            external_recipient_emails: ['client@example.test']
        }));
        await fixture.whenStable();

        const remove = document.body.querySelector('.external-recipient-chip .p-chip-remove-icon') as HTMLElement | null;
        expect(remove?.getAttribute('aria-label')).toBe('Remove client@example.test');
        remove?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
        expect(component.draft().external_recipient_emails).toEqual([]);
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

    it('omits immutable scope fields from report updates', () => {
        const report = reportFixture({ scope: 'team', tenant_id: teamID });
        const component = fixture.componentInstance as TestAccess;
        component.openEdit(report);

        component.save();

        expect(serviceMock.update).toHaveBeenCalledTimes(1);
        const update = serviceMock.update.mock.calls[0]?.[1] as Record<string, unknown>;
        expect(update['scope']).toBeUndefined();
        expect(update['tenant_id']).toBeUndefined();
        expect(update['name']).toBe(report.name);
        expect(update['preset']).toBe(report.preset);
        expect(update['status']).toBe(report.status);
    });

    it('shows an actionable initial error separately from an empty collection', async () => {
        serviceMock.load.mockReturnValueOnce(throwError(() => new Error('load failed')));
        const component = fixture.componentInstance as TestAccess;
        component.refresh();
        await fixture.whenStable();

        expect(component.initialLoadError()).toBe(true);
        expect(fixture.nativeElement.textContent).toContain('Reports are unavailable');
        expect(fixture.nativeElement.querySelector('app-page-state button')).not.toBeNull();
    });

    it('keeps stale reports visible when refresh fails', async () => {
        reports.set([reportFixture()]);
        serviceMock.load.mockReturnValueOnce(throwError(() => new Error('refresh failed')));
        const component = fixture.componentInstance as TestAccess;
        component.refresh();
        await fixture.whenStable();

        expect(fixture.nativeElement.textContent).toContain('Morning growth report');
        expect(fixture.nativeElement.querySelector('[data-testid="report-list-feedback"]')?.textContent).toContain('Reports could not be loaded.');
    });

    it('prevents duplicate previews and keeps preview errors in the editor', async () => {
        const previewRequest = new Subject<never>();
        serviceMock.preview.mockReturnValue(previewRequest);
        const component = fixture.componentInstance as TestAccess;
        component.openCreate();
        component.draft.update((draft) => ({ ...draft, name: 'Preview test' }));
        component.previewReport();
        component.previewReport();
        expect(serviceMock.preview).toHaveBeenCalledTimes(1);
        expect(component.previewLoading()).toBe(true);

        previewRequest.error(new Error('preview failed'));
        await fixture.whenStable();
        expect(component.previewLoading()).toBe(false);
        expect(document.body.querySelector('[data-testid="report-dialog-feedback"]')?.textContent).toContain('The preview could not be generated.');
    });

    it('surfaces team-member and history loading failures locally', async () => {
        listTeamMembers.mockReturnValueOnce(throwError(() => new Error('members failed'))).mockReturnValueOnce(throwError(() => new Error('members failed')));
        serviceMock.runs.mockReturnValueOnce(throwError(() => new Error('history failed')));
        const component = fixture.componentInstance as TestAccess;
        component.openCreate();
        component.setScope('team');
        component.openHistory(reportFixture());
        await fixture.whenStable();

        expect(component.membersError()).toBe(true);
        expect(document.body.querySelector('[data-testid="report-members-error"]')?.textContent).toContain('Team members could not be loaded.');
        expect(component.historyLoading()).toBe(false);
        expect(document.body.querySelector('[data-testid="report-history-feedback"]')?.textContent).toContain('Delivery history could not be loaded.');
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
            consent_version: 1,
            created_at: '2026-07-19T00:00:00Z',
            updated_at: '2026-07-19T00:00:00Z'
        };
    }
});
