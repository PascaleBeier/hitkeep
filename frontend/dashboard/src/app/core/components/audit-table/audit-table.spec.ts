import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { AuditTableComponent } from './audit-table';
import { AuditTableQuery, AuditTableRow } from './audit-table.types';

const rows: AuditTableRow[] = [
    {
        id: 'audit-1',
        created_at: '2026-05-01T12:00:00Z',
        actor_id: 'actor-1',
        actor_email_snapshot: 'admin@example.com',
        actor_role_snapshot: 'owner',
        team_id: 'team-1',
        action: 'permission.site_member_granted',
        target_type: 'permission',
        target_id: 'site-1',
        target_label: 'example.com',
        target_user_id: 'user-2',
        outcome: 'success',
        ip_address: '203.0.113.10',
        ip_country_code: 'US',
        request_id: 'req-1',
        user_agent: 'Mozilla/5.0',
        details: 'Granted site access.'
    },
    {
        id: 'audit-2',
        created_at: '2026-05-01T11:55:00Z',
        actor_id: 'actor-1',
        actor_email_snapshot: 'admin@example.com',
        actor_role_snapshot: 'owner',
        team_id: 'team-1',
        action: 'google_search_console.connected',
        target_type: 'google_search_console_connection',
        target_id: 'team-1',
        target_label: 'Search Console',
        outcome: 'success',
        details: 'Search Console connected.'
    }
];

describe('AuditTableComponent', () => {
    let fixture: ComponentFixture<AuditTableComponent>;
    let component: AuditTableComponent;
    let emittedQueries: AuditTableQuery[];

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                AuditTableComponent,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: { unknown: 'Unknown' },
                            auditTable: {
                                empty: 'No audit entries found',
                                filters: {
                                    aria: 'Audit filters',
                                    action: 'Action',
                                    outcome: 'Outcome',
                                    targetType: 'Target type',
                                    dateRange: 'Date range',
                                    dateRangePlaceholder: 'Date range',
                                    search: 'Search',
                                    searchPlaceholder: 'Search',
                                    allActions: 'All actions',
                                    allTargets: 'All targets',
                                    allOutcomes: 'All outcomes'
                                },
                                actions: {
                                    refresh: 'Refresh',
                                    export: 'Export',
                                    clearFilters: 'Clear filters',
                                    expandRow: 'Show evidence',
                                    collapseRow: 'Hide evidence',
                                    permissionSiteMemberGranted: 'Site access granted',
                                    googleSearchConsoleConnected: 'Search Console connected'
                                },
                                columns: {
                                    evidence: 'Evidence',
                                    time: 'Time',
                                    actor: 'Actor',
                                    action: 'Action',
                                    targetType: 'Area',
                                    target: 'Target',
                                    outcome: 'Outcome',
                                    ipAddress: 'IP address',
                                    country: 'Country',
                                    details: 'Details'
                                },
                                outcomes: { success: 'Success' },
                                targetTypes: { permission: 'Permission', googleSearchConsoleConnection: 'Search Console connection' },
                                roles: { owner: 'Owner' },
                                evidence: {
                                    details: 'Full details',
                                    actorId: 'Actor ID',
                                    actorRole: 'Actor role',
                                    teamId: 'Team ID',
                                    targetType: 'Target type',
                                    targetId: 'Target ID',
                                    targetUserId: 'Target user ID',
                                    requestId: 'Request ID',
                                    userAgent: 'User agent'
                                },
                                pagination: {
                                    summary: 'Showing {{start}}-{{end}} of {{total}} entries',
                                    empty: 'No entries to show'
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
                provideTranslocoLocale({
                    langToLocaleMapping: {
                        en: 'en-US'
                    }
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(AuditTableComponent);
        component = fixture.componentInstance;
        emittedQueries = [];
        component.queryChange.subscribe((query) => emittedQueries.push(query));
        fixture.componentRef.setInput('rows', rows);
        fixture.componentRef.setInput('total', 2);
        fixture.componentRef.setInput('query', { limit: 25, offset: 0 });
        fixture.componentRef.setInput('actionOptions', [
            { label: 'All actions', value: '' },
            { label: 'Site access granted', value: 'permission.site_member_granted' }
        ]);
        fixture.componentRef.setInput('outcomeOptions', [
            { label: 'All outcomes', value: '' },
            { label: 'Success', value: 'success' }
        ]);
        fixture.componentRef.setInput('targetTypeOptions', [
            { label: 'All targets', value: '' },
            { label: 'Permission', value: 'permission' }
        ]);
        fixture.detectChanges();
    });

    it('renders compact audit evidence columns', () => {
        const text = fixture.nativeElement.textContent as string;

        expect(text).toContain('admin@example.com');
        expect(text).toContain('Site access granted');
        expect(text).toContain('example.com');
        expect(text).toContain('203.0.113.10');
        expect(text).toContain('US');
    });

    it('renders Search Console audit actions and target types from translations', () => {
        const text = fixture.nativeElement.textContent as string;

        expect(text).toContain('Search Console connected');
        expect(text).toContain('Search Console connection');
        expect(text).not.toContain('Google Search Console Connected');
    });

    it('expands evidence without requiring an API call', () => {
        component['toggleRow'](rows[0]);
        fixture.detectChanges();

        const text = fixture.nativeElement.textContent as string;
        expect(text).toContain('Full details');
        expect(text).toContain('req-1');
        expect(text).toContain('Mozilla/5.0');
    });

    it('emits filter and paginator query changes', () => {
        component['updateFilter']('action', 'permission.site_member_granted');
        component['onPageChange']({ first: 50, rows: 50 });

        expect(emittedQueries[0]).toEqual({ action: 'permission.site_member_granted', limit: 25, offset: 0 });
        expect(emittedQueries[1]).toEqual({ limit: 50, offset: 50 });
    });

    it('debounces free-text search before emitting query changes', () => {
        vi.useFakeTimers();
        component['onSearchInput']('req-1');
        vi.advanceTimersByTime(299);
        expect(emittedQueries.length).toBe(0);

        vi.advanceTimersByTime(1);

        expect(emittedQueries).toEqual([{ query: 'req-1', limit: 25, offset: 0 }]);
        vi.useRealTimers();
    });

    it('clears active filters while preserving the page size', () => {
        fixture.componentRef.setInput('query', { action: 'auth.login_succeeded', limit: 50, offset: 100 });
        fixture.detectChanges();

        component['clearFilters']();

        expect(emittedQueries.at(-1)).toEqual({ limit: 50, offset: 0 });
    });
});
