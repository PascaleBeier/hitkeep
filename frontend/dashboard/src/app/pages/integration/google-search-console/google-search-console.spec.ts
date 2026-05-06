import { ComponentFixture, TestBed } from '@angular/core/testing';
import { HttpErrorResponse } from '@angular/common/http';
import { signal } from '@angular/core';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { ConfirmationService } from 'primeng/api';
import { of, Subject, throwError } from 'rxjs';
import { vi } from 'vitest';

import { SiteService } from '@features/sites/services/site.service';
import { TeamService } from '@services/team.service';
import { GoogleSearchConsoleService, GoogleSearchConsoleStatus } from '@services/google-search-console.service';
import { GoogleSearchConsolePage } from './google-search-console';

describe('GoogleSearchConsolePage', () => {
    let fixture: ComponentFixture<GoogleSearchConsolePage>;
    let status: GoogleSearchConsoleStatus;

    const teamService = {
        activeTeamId: signal('team-1')
    };
    const siteService = {
        activeSite: signal({
            id: 'site-1',
            domain: 'example.com',
            created_at: '2026-05-05T00:00:00Z'
        })
    };

    let integrationService: {
        getStatus: ReturnType<typeof vi.fn>;
        connect: ReturnType<typeof vi.fn>;
        disconnect: ReturnType<typeof vi.fn>;
        listProperties: ReturnType<typeof vi.fn>;
        getSiteMapping: ReturnType<typeof vi.fn>;
        mapSiteProperty: ReturnType<typeof vi.fn>;
        unmapSiteProperty: ReturnType<typeof vi.fn>;
        requestSync: ReturnType<typeof vi.fn>;
    };

    beforeEach(async () => {
        status = {
            status: 'credentials_missing',
            configured: false,
            connected: false,
            credential_status: 'missing',
            needs_admin_action: true,
            can_manage: true,
            managed_credentials_mode: 'self_hosted'
        };

        teamService.activeTeamId.set('team-1');
        siteService.activeSite.set({
            id: 'site-1',
            domain: 'example.com',
            created_at: '2026-05-05T00:00:00Z'
        });
        integrationService = {
            getStatus: vi.fn(() => of(status)),
            connect: vi.fn(() => of({ auth_url: 'https://accounts.example.test/oauth' })),
            disconnect: vi.fn(() => of({ status: 'ok' })),
            listProperties: vi.fn(() =>
                of({
                    properties: [{ uri: 'sc-domain:example.com', permission_level: 'siteOwner' }]
                })
            ),
            getSiteMapping: vi.fn(() =>
                of({
                    site_id: 'site-1',
                    team_id: 'team-1',
                    mapped: false,
                    can_manage: true
                })
            ),
            mapSiteProperty: vi.fn(() =>
                of({
                    site_id: 'site-1',
                    team_id: 'team-1',
                    mapped: true,
                    property_uri: 'sc-domain:example.com',
                    property_permission_level: 'siteOwner',
                    can_manage: true
                })
            ),
            unmapSiteProperty: vi.fn(() =>
                of({
                    site_id: 'site-1',
                    team_id: 'team-1',
                    mapped: false,
                    can_manage: true
                })
            ),
            requestSync: vi.fn(() =>
                of({
                    site_id: 'site-1',
                    team_id: 'team-1',
                    mapped: true,
                    property_uri: 'sc-domain:example.com',
                    property_permission_level: 'siteOwner',
                    can_manage: true,
                    sync_status: {
                        state: 'pending',
                        last_attempt_at: '2026-05-05T10:00:00Z',
                        manual: true
                    }
                })
            )
        };

        await TestBed.configureTestingModule({
            imports: [
                GoogleSearchConsolePage,
                NoopAnimationsModule,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { integration: 'Integration', googleSearchConsole: 'Search Console' },
                            integration: {
                                googleSearchConsole: {
                                    title: 'Google Search Console',
                                    subtitle: 'Connect Search Console properties.',
                                    status: {
                                        credentials_missing: 'Credentials missing',
                                        connected: 'Connected',
                                        disconnected: 'Disconnected',
                                        loading: 'Loading',
                                        mapped: 'Mapped',
                                        unmapped: 'Unmapped'
                                    },
                                    labels: {
                                        account: 'Account',
                                        site: 'Site',
                                        property: 'Property',
                                        searchProperties: 'Search properties',
                                        propertyMapping: 'Property mapping',
                                        permission: 'Permission',
                                        syncStatus: 'Sync status',
                                        lastSuccess: 'Last success',
                                        lastAttempt: 'Last attempt',
                                        importedRange: 'Imported range',
                                        nextRetry: 'Next retry',
                                        lastError: 'Last error'
                                    },
                                    actions: {
                                        refresh: 'Refresh',
                                        docs: 'Setup docs',
                                        connect: 'Connect',
                                        reconnect: 'Reconnect',
                                        disconnect: 'Disconnect',
                                        confirmDisconnect: 'Disconnect Search Console?',
                                        mapProperty: 'Map property',
                                        removeMapping: 'Remove mapping',
                                        confirmRemoveMapping: 'Remove Search Console mapping?',
                                        syncNow: 'Sync now',
                                        syncRequesting: 'Requesting sync...',
                                        syncQueued: 'Sync queued',
                                        syncRunning: 'Sync running'
                                    },
                                    sync: {
                                        pending: 'Pending',
                                        running: 'Running',
                                        succeeded: 'Succeeded',
                                        failed: 'Failed',
                                        needs_attention: 'Needs attention',
                                        idle: 'No sync yet',
                                        manualPending: 'Manual sync queued.',
                                        queuedFeedback: 'Sync queued.',
                                        runningFeedback: 'Sync running.',
                                        succeededWithRange: '{{start}}-{{end}} imported.',
                                        noHistory: 'No imports yet.',
                                        quota_limited: 'Quota limited',
                                        authorization_revoked: 'Authorization revoked',
                                        token_refresh_failed: 'Token refresh failed',
                                        property_access_lost: 'Property access lost',
                                        credentials_invalid: 'Invalid credentials',
                                        credentials_missing: 'Credentials missing',
                                        api_disabled: 'Search Console API disabled',
                                        google_unavailable: 'Google unavailable',
                                        unknown: 'Unknown sync error'
                                    },
                                    states: {
                                        missingCredentials: 'OAuth credentials required.',
                                        connected: 'Connected.',
                                        disconnected: 'Not connected.',
                                        propertyMapped: 'Mapped to {{property}}.',
                                        propertyUnmapped: 'No property mapped.',
                                        loadingMapping: 'Loading mapping.',
                                        noProperties: 'No properties returned.',
                                        noMatchingProperties: 'No matching Search Console properties for this site.',
                                        noSite: 'Select a site first.',
                                        readonly: 'Read-only for your role.',
                                        noMappingSync: 'Map a Search Console property before syncing.',
                                        syncRequested: 'Sync requested.',
                                        disconnectSuccess: 'Disconnected.',
                                        reconnectHint: 'Reconnect to refresh access.'
                                    },
                                    confirm: {
                                        disconnectMessage: 'Future imports stop. Imported data stays.',
                                        removeMappingMessage: 'Future imports stop for this site.',
                                        disconnectAccept: 'Disconnect',
                                        removeMappingAccept: 'Remove mapping',
                                        cancel: 'Cancel'
                                    },
                                    errors: {
                                        load: 'Status unavailable.',
                                        connect: 'Connection failed.',
                                        disconnect: 'Disconnect failed.',
                                        properties: 'Properties unavailable.',
                                        apiDisabled: 'Enable the Search Console API, then refresh.',
                                        reconnect: 'Reconnect to refresh access.',
                                        propertyAccess: 'Property access lost.',
                                        mapping: 'Mapping unavailable.',
                                        mapProperty: 'Mapping failed.',
                                        unmapProperty: 'Unmap failed.',
                                        sync: 'Sync request failed.'
                                    }
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    }
                })
            ],
            providers: [
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US',
                        'en-US': 'en-US'
                    }
                }),
                { provide: TeamService, useValue: teamService },
                { provide: SiteService, useValue: siteService },
                { provide: GoogleSearchConsoleService, useValue: integrationService }
            ]
        }).compileComponents();
    });

    it('renders missing credential status', () => {
        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Credentials missing');
        expect(fixture.nativeElement.textContent).toContain('OAuth credentials required.');
        expect(fixture.nativeElement.textContent).not.toContain('Mode');
        expect(fixture.nativeElement.textContent).not.toContain('Self-hosted');
    });

    it('renders connected account status', () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Connected');
        expect((fixture.nativeElement.textContent.match(/Connected/g) ?? []).length).toBe(1);
        expect(fixture.nativeElement.textContent).toContain('owner@example.com');
        expect(fixture.nativeElement.textContent).not.toContain('Mode');
        expect(fixture.nativeElement.textContent).not.toContain('Self-hosted');
        const docsLink = fixture.nativeElement.querySelector('[data-testid="gsc-docs-link"]') as HTMLAnchorElement;
        expect(docsLink?.href).toBe('https://hitkeep.com/guides/integrations/google-search-console/');
        expect(docsLink.closest('.google-search-console-panel__header-actions')).toBeTruthy();
        expect(docsLink.closest('.google-search-console-actions')).toBeFalsy();
    });

    it('reloads status when the active team changes', () => {
        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();

        teamService.activeTeamId.set('team-2');
        fixture.detectChanges();

        expect(integrationService.getStatus.mock.calls).toEqual([['team-1'], ['team-2']]);
    });

    it('shows a mapping loading state without readonly or empty-property copy while mapping is unresolved', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        const mappingResponse = new Subject<{
            site_id: string;
            team_id: string;
            mapped: boolean;
            can_manage: boolean;
        }>();
        integrationService.getSiteMapping = vi.fn(() => mappingResponse.asObservable());

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Loading mapping.');
        expect(fixture.nativeElement.textContent).not.toContain('No properties returned.');
        expect(fixture.nativeElement.textContent).not.toContain('Read-only for your role.');
        expect(integrationService.listProperties).not.toHaveBeenCalled();
    });

    it('maps a listed property for the active site when connected', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const select = fixture.nativeElement.querySelector('[data-testid="gsc-property-select"]') as HTMLElement;
        expect(fixture.nativeElement.textContent).toContain('example.com');
        expect(select.textContent).toContain('sc-domain:example.com');

        const button = fixture.nativeElement.querySelector('[data-testid="gsc-map-property"]') as HTMLButtonElement;
        button.click();
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(integrationService.mapSiteProperty).toHaveBeenCalledWith('site-1', 'sc-domain:example.com');
        expect(fixture.nativeElement.textContent).toContain('Mapped to sc-domain:example.com.');
    });

    it('only offers Search Console properties that match the active site', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        integrationService.listProperties = vi.fn(() =>
            of({
                properties: [
                    { uri: 'https://www.example.com/', permission_level: 'siteOwner' },
                    { uri: 'sc-domain:other.example', permission_level: 'siteOwner' },
                    { uri: 'sc-domain:example.com', permission_level: 'siteOwner' },
                    { uri: 'https://vest-hv.de/', permission_level: 'siteOwner' }
                ]
            })
        );

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const options = (fixture.componentInstance as unknown as { propertyOptions: () => { label: string; value: string }[] }).propertyOptions();
        expect(options).toEqual([
            { label: 'sc-domain:example.com', value: 'sc-domain:example.com' },
            { label: 'https://www.example.com/', value: 'https://www.example.com/' }
        ]);

        const button = fixture.nativeElement.querySelector('[data-testid="gsc-map-property"]') as HTMLButtonElement;
        button.click();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(integrationService.mapSiteProperty).toHaveBeenCalledWith('site-1', 'sc-domain:example.com');
    });

    it('shows a no-matching-properties state when Google only returns unrelated properties', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        integrationService.listProperties = vi.fn(() =>
            of({
                properties: [{ uri: 'sc-domain:other.example', permission_level: 'siteOwner' }]
            })
        );

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('No matching Search Console properties for this site.');
        expect(fixture.nativeElement.textContent).not.toContain('No properties returned.');
        const button = fixture.nativeElement.querySelector('[data-testid="gsc-map-property"]') as HTMLButtonElement;
        expect(button.disabled).toBe(true);
        button.click();
        expect(integrationService.mapSiteProperty).not.toHaveBeenCalled();
    });

    it('does not apply a stale mapping response after the active site changes', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        const mappingResponse = new Subject<{
            site_id: string;
            team_id: string;
            mapped: boolean;
            property_uri: string;
            property_permission_level: string;
            can_manage: boolean;
        }>();
        integrationService.mapSiteProperty = vi.fn(() => mappingResponse.asObservable());

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const button = fixture.nativeElement.querySelector('[data-testid="gsc-map-property"]') as HTMLButtonElement;
        button.click();
        fixture.detectChanges();

        siteService.activeSite.set({
            id: 'site-2',
            domain: 'other.example.com',
            created_at: '2026-05-05T00:00:00Z'
        });
        fixture.detectChanges();
        await fixture.whenStable();

        mappingResponse.next({
            site_id: 'site-1',
            team_id: 'team-1',
            mapped: true,
            property_uri: 'sc-domain:example.com',
            property_permission_level: 'siteOwner',
            can_manage: true
        });
        mappingResponse.complete();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('other.example.com');
        expect(fixture.nativeElement.textContent).not.toContain('Mapped to sc-domain:example.com.');
    });

    it('clears mapped controls while loading a newly selected site', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        const siteTwoMapping = new Subject<{
            site_id: string;
            team_id: string;
            mapped: boolean;
            can_manage: boolean;
        }>();
        integrationService.getSiteMapping = vi
            .fn()
            .mockReturnValueOnce(
                of({
                    site_id: 'site-1',
                    team_id: 'team-1',
                    mapped: true,
                    property_uri: 'sc-domain:example.com',
                    property_permission_level: 'siteOwner',
                    can_manage: true
                })
            )
            .mockReturnValueOnce(siteTwoMapping.asObservable());

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('[data-testid="gsc-remove-mapping"]')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('[data-testid="gsc-sync-now"]')).not.toBeNull();

        siteService.activeSite.set({
            id: 'site-2',
            domain: 'other.example.com',
            created_at: '2026-05-05T00:00:00Z'
        });
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Loading');
        expect(fixture.nativeElement.textContent).not.toContain('Mapped to sc-domain:example.com.');
        expect(fixture.nativeElement.querySelector('[data-testid="gsc-remove-mapping"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('[data-testid="gsc-sync-now"]')).toBeNull();

        siteTwoMapping.next({
            site_id: 'site-2',
            team_id: 'team-1',
            mapped: false,
            can_manage: true
        });
        siteTwoMapping.complete();
    });

    it('renders sync status metadata and quota guidance for a mapped site', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        integrationService.getSiteMapping = vi.fn(() =>
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                mapped: true,
                property_uri: 'sc-domain:example.com',
                property_permission_level: 'siteOwner',
                can_manage: true,
                sync_status: {
                    state: 'failed',
                    imported_start_date: '2026-04-01',
                    imported_end_date: '2026-04-30',
                    last_success_at: '2026-05-04T12:00:00Z',
                    last_attempt_at: '2026-05-05T10:00:00Z',
                    last_error_category: 'quota_limited',
                    next_retry_at: '2026-05-05T11:00:00Z',
                    manual: false
                }
            })
        );

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Sync status');
        expect(fixture.nativeElement.textContent).toContain('Failed');
        expect(fixture.nativeElement.textContent).toContain('Quota limited');
        expect(fixture.nativeElement.textContent).not.toContain('2026-04-01');
        expect(fixture.nativeElement.textContent).not.toContain('2026-04-30');
        expect(fixture.nativeElement.textContent).toContain('Next retry');
        expect(fixture.nativeElement.textContent).not.toContain('2026-05-04T12:00:00Z');
        expect(fixture.nativeElement.textContent).not.toContain('2026-05-05T11:00:00Z');
    });

    it('shows an existing mapping as an association until the admin disassociates it', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        integrationService.getSiteMapping = vi.fn(() =>
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                mapped: true,
                property_uri: 'sc-domain:example.com',
                property_permission_level: 'siteOwner',
                can_manage: true
            })
        );

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Mapped to sc-domain:example.com.');
        expect(fixture.nativeElement.querySelector('[data-testid="gsc-property-select"]')).toBeNull();
        expect(integrationService.listProperties).not.toHaveBeenCalled();
        expect(fixture.nativeElement.querySelector('[data-testid="gsc-change-mapping"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('[data-testid="gsc-remove-mapping"]')).not.toBeNull();
    });

    it('renders credentials-missing sync guidance without exposing a raw translation key', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        integrationService.getSiteMapping = vi.fn(() =>
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                mapped: true,
                property_uri: 'sc-domain:example.com',
                property_permission_level: 'siteOwner',
                can_manage: true,
                sync_status: {
                    state: 'needs_attention',
                    last_error_category: 'credentials_missing',
                    manual: false
                }
            })
        );

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Needs attention');
        expect(fixture.nativeElement.textContent).toContain('Credentials missing');
        expect(fixture.nativeElement.textContent).not.toContain('integration.googleSearchConsole.sync.credentials_missing');
    });

    it('requests a manual sync only after a property is mapped', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('[data-testid="gsc-sync-now"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('[data-testid="gsc-sync-mapping-required"]')?.textContent).toContain('Map a Search Console property before syncing.');

        integrationService.getSiteMapping = vi.fn(() =>
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                mapped: true,
                property_uri: 'sc-domain:example.com',
                property_permission_level: 'siteOwner',
                can_manage: true
            })
        );
        teamService.activeTeamId.set('team-2');
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const syncButton = fixture.nativeElement.querySelector('[data-testid="gsc-sync-now"]') as HTMLButtonElement;
        syncButton.click();
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(integrationService.requestSync).toHaveBeenCalledWith('site-1');
        expect(fixture.nativeElement.textContent).toContain('Manual sync queued.');
        expect(fixture.nativeElement.textContent).toContain('Sync requested.');
        expect(fixture.nativeElement.querySelector('[data-testid="gsc-sync-feedback"]')?.textContent).toContain('Sync queued.');
        expect(syncButton.disabled).toBe(true);
        expect(syncButton.textContent).toContain('Sync queued');
    });

    it('shows manual sync request failures inside the sync panel', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        integrationService.getSiteMapping = vi.fn(() =>
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                mapped: true,
                property_uri: 'sc-domain:example.com',
                property_permission_level: 'siteOwner',
                can_manage: true
            })
        );
        integrationService.requestSync = vi.fn(() => throwError(() => new Error('sync failed')));

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const syncButton = fixture.nativeElement.querySelector('[data-testid="gsc-sync-now"]') as HTMLButtonElement;
        syncButton.click();
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('[data-testid="gsc-sync-error"]')?.textContent).toContain('Sync request failed.');
    });

    it('confirms before disconnecting a connected account', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        const confirmation = fixture.debugElement.injector.get(ConfirmationService);
        const confirmSpy = vi.spyOn(confirmation, 'confirm');
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const button = fixture.nativeElement.querySelector('[data-testid="gsc-disconnect"]') as HTMLButtonElement;
        button.click();
        fixture.detectChanges();

        expect(confirmSpy).toHaveBeenCalled();
        expect(integrationService.disconnect).not.toHaveBeenCalled();

        teamService.activeTeamId.set('team-2');
        fixture.detectChanges();

        confirmSpy.mock.calls[0][0].accept?.();
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(integrationService.disconnect).toHaveBeenCalledWith('team-1');
        expect(fixture.nativeElement.textContent).toContain('Disconnected.');
    });

    it('confirms before removing an existing property mapping for the original site', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        integrationService.getSiteMapping = vi.fn(() =>
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                mapped: true,
                property_uri: 'sc-domain:example.com',
                property_permission_level: 'siteOwner',
                can_manage: true
            })
        );

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        const confirmation = fixture.debugElement.injector.get(ConfirmationService);
        const confirmSpy = vi.spyOn(confirmation, 'confirm');
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const button = fixture.nativeElement.querySelector('[data-testid="gsc-remove-mapping"]') as HTMLButtonElement;
        button.click();
        fixture.detectChanges();

        expect(confirmSpy).toHaveBeenCalled();
        expect(integrationService.unmapSiteProperty).not.toHaveBeenCalled();

        siteService.activeSite.set({
            id: 'site-2',
            domain: 'other.example.com',
            created_at: '2026-05-05T00:00:00Z'
        });
        fixture.detectChanges();

        confirmSpy.mock.calls[0][0].accept?.();
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(integrationService.unmapSiteProperty).toHaveBeenCalledWith('site-1');
    });

    it('loads the property picker after disassociation and does not keep a stale selected property on refresh failure', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        integrationService.getSiteMapping = vi.fn(() =>
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                mapped: true,
                property_uri: 'sc-domain:example.com',
                property_permission_level: 'siteOwner',
                can_manage: true
            })
        );
        integrationService.listProperties = vi.fn(() =>
            throwError(
                () =>
                    new HttpErrorResponse({
                        status: 502,
                        error: { status: 'error', code: 'api_disabled', message: 'Could not list Google Search Console properties' }
                    })
            )
        );

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        const confirmation = fixture.debugElement.injector.get(ConfirmationService);
        const confirmSpy = vi.spyOn(confirmation, 'confirm');
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const button = fixture.nativeElement.querySelector('[data-testid="gsc-remove-mapping"]') as HTMLButtonElement;
        button.click();
        fixture.detectChanges();
        confirmSpy.mock.calls[0][0].accept?.();
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const mapButton = fixture.nativeElement.querySelector('[data-testid="gsc-map-property"]') as HTMLButtonElement;
        expect(integrationService.listProperties).toHaveBeenCalledWith('team-1');
        expect(mapButton.disabled).toBe(true);
        expect(integrationService.mapSiteProperty).not.toHaveBeenCalled();
    });

    it('renders readonly unmapped state without pretending the property list was loaded', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'viewer@example.com',
            needs_admin_action: false,
            can_manage: false,
            managed_credentials_mode: 'managed'
        };
        integrationService.getSiteMapping = vi.fn(() =>
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                mapped: false,
                can_manage: false
            })
        );

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(integrationService.listProperties).not.toHaveBeenCalled();
        expect(fixture.nativeElement.textContent).toContain('No property mapped.');
        expect(fixture.nativeElement.textContent).not.toContain('No properties returned.');
    });

    it('shows API-disabled guidance when Google rejects property listing for project setup', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'owner@example.com',
            needs_admin_action: false,
            can_manage: true,
            managed_credentials_mode: 'managed'
        };
        integrationService.listProperties = vi.fn(() =>
            throwError(
                () =>
                    new HttpErrorResponse({
                        status: 502,
                        error: { status: 'error', code: 'api_disabled', message: 'Could not list Google Search Console properties' }
                    })
            )
        );

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Enable the Search Console API, then refresh.');
        expect(fixture.nativeElement.textContent).not.toContain('No properties returned.');
    });

    it('renders a readonly mapped state without mutation controls or admin-only properties', async () => {
        status = {
            status: 'connected',
            configured: true,
            connected: true,
            credential_status: 'configured',
            connected_account_label: 'viewer@example.com',
            needs_admin_action: false,
            can_manage: false,
            managed_credentials_mode: 'managed'
        };
        integrationService.getSiteMapping = vi.fn(() =>
            of({
                site_id: 'site-1',
                team_id: 'team-1',
                mapped: true,
                property_uri: 'sc-domain:example.com',
                property_permission_level: 'siteOwner',
                can_manage: false
            })
        );

        fixture = TestBed.createComponent(GoogleSearchConsolePage);
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        expect(integrationService.listProperties).not.toHaveBeenCalled();
        expect(fixture.nativeElement.textContent).toContain('Mapped to sc-domain:example.com.');
        expect(fixture.nativeElement.textContent).toContain('Read-only for your role.');
        expect(fixture.nativeElement.querySelector('[data-testid="gsc-map-property"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('[data-testid="gsc-remove-mapping"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('[data-testid="gsc-sync-now"]')).toBeNull();
    });
});
