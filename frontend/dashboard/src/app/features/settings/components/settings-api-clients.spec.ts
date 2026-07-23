import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { of, Subject, throwError } from 'rxjs';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { vi } from 'vitest';
import { ConfirmationService } from '@openng/optimus-ui/api';

import { SettingsAPIClients } from './settings-api-clients';
import { APIClient, APIClientsService } from '@services/api-clients.service';
import { PermissionService } from '@services/permission.service';
import { Site } from '@models/analytics.types';
import { SiteService } from '@features/sites/services/site.service';

describe('SettingsAPIClients', () => {
    let fixture: ComponentFixture<SettingsAPIClients>;
    let component: SettingsAPIClients;

    const defaultClient: APIClient = {
        id: 'client-default',
        owner_type: 'personal',
        name: 'Default client',
        description: '',
        instance_role: 'user',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
        site_roles: []
    };

    const apiClientsServiceMock = {
        listClients: vi.fn(() => of([])),
        createClient: vi.fn(() => of({ client: defaultClient, token: '' })),
        updateClient: vi.fn(() => of(defaultClient)),
        rotateClient: vi.fn(() => of({ client: defaultClient, token: '' })),
        deleteClient: vi.fn(() => of(void 0))
    };

    const siteServiceMock = {
        sites: signal<Site[]>([])
    };

    const permissionServiceMock = {
        permissions: signal({
            instance_role: 'owner' as const,
            permissions: {}
        })
    };

    const site = (id: string, domain: string): Site => ({
        id,
        user_id: 'user-1',
        domain,
        created_at: '2026-01-01T00:00:00Z'
    });

    beforeEach(async () => {
        vi.clearAllMocks();
        siteServiceMock.sites.set([]);

        await TestBed.configureTestingModule({
            imports: [
                SettingsAPIClients,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            settings: {
                                apiClients: {
                                    title: 'API clients',
                                    description: 'Personal',
                                    teamTitle: 'Team API clients',
                                    teamDescription: 'Team',
                                    loading: 'Loading',
                                    empty: 'Empty',
                                    listTitle: 'Existing',
                                    createDialogTitle: 'Add API client',
                                    editDialogTitle: 'Edit API client',
                                    confirmDelete: 'Delete',
                                    confirmRotate: 'Rotate',
                                    noSiteAccess: 'No site access',
                                    noSiteAccessInstanceOnly: 'Instance/admin only; no site access',
                                    siteGrantCount: '{{count}} site grants',
                                    siteGrantPopoverTitle: 'Site grants',
                                    status: { active: 'Active', inactive: 'Inactive', revoked: 'Revoked', expired: 'Expired' },
                                    meta: {
                                        instanceRole: 'Instance role',
                                        created: 'Created',
                                        lastUsed: 'Last used',
                                        expires: 'Expires'
                                    },
                                    actions: {
                                        add: 'Add API client',
                                        create: 'Create',
                                        save: 'Save',
                                        edit: 'Edit',
                                        revoke: 'Revoke',
                                        reactivate: 'Reactivate',
                                        delete: 'Delete',
                                        addScope: 'Add grant',
                                        refresh: 'Refresh',
                                        rollToken: 'Roll token'
                                    },
                                    form: {
                                        nameLabel: 'Client name',
                                        namePlaceholder: 'Name',
                                        descriptionLabel: 'Description',
                                        descriptionPlaceholder: 'Description',
                                        instanceRoleLabel: 'Instance role',
                                        expiresAtLabel: 'Expiration',
                                        expiresAtHint: 'Hint',
                                        siteScopesLabel: 'Site grants',
                                        siteScopesHint: 'Personal grants',
                                        teamSiteScopesHint: 'Team grants',
                                        selectSitePlaceholder: 'Select site',
                                        validation: {
                                            nameRequired: 'Required',
                                            nameTooLong: 'Too long',
                                            instanceRoleRequired: 'Required',
                                            expiresAtPast: 'Past',
                                            expiresAtInvalid: 'Invalid',
                                            scopeSiteRequired: 'Required'
                                        }
                                    },
                                    tokenNotice: { title: 'Token', description: 'Copy it' },
                                    messages: {
                                        created: 'Created',
                                        updated: 'Updated',
                                        deleted: 'Deleted',
                                        revoked: 'Revoked',
                                        reactivated: 'Reactivated',
                                        rotated: 'Rotated'
                                    },
                                    errors: {
                                        loadFailed: 'Load failed',
                                        createFailed: 'Create failed',
                                        updateFailed: 'Update failed',
                                        rotateFailed: 'Rotate failed',
                                        deleteFailed: 'Delete failed',
                                        invalidExpiration: 'Invalid expiration',
                                        notFound: 'Not found'
                                    }
                                }
                            },
                            admin: {
                                roles: {
                                    instanceOwner: 'Owner',
                                    instanceAdmin: 'Admin',
                                    user: 'User'
                                }
                            },
                            roles: {
                                owner: 'Owner',
                                admin: 'Admin',
                                editor: 'Editor',
                                viewer: 'Viewer'
                            },
                            common: {
                                copyLink: 'Copy',
                                copyControl: {
                                    copy: 'Copy',
                                    copied: 'Copied',
                                    failed: 'Copy failed',
                                    ariaLabel: 'Copy to clipboard'
                                },
                                columns: {
                                    name: 'Name',
                                    actions: 'Actions'
                                },
                                searchPlaceholder: 'Search',
                                actions: {
                                    cancel: 'Cancel',
                                    more: 'More actions'
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
                }),
                { provide: APIClientsService, useValue: apiClientsServiceMock },
                { provide: SiteService, useValue: siteServiceMock },
                { provide: PermissionService, useValue: permissionServiceMock }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(SettingsAPIClients);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    afterEach(() => {
        document.querySelectorAll('.table-row-actions-menu, .p-confirm-dialog, .p-dialog-mask, .p-select-overlay').forEach((element) => element.remove());
    });

    it('loads personal API clients by default', () => {
        const calls = (apiClientsServiceMock.listClients as unknown as { mock: { calls: unknown[][] } }).mock.calls;
        expect(calls[0][0]).toBeNull();
    });

    it('uses the shared site service for alphabetized grant options', () => {
        siteServiceMock.sites.set([site('site-alpha', 'alpha.example.com'), site('site-shop', 'shop.example.com'), site('site-zeta', 'zeta.example.com')]);
        fixture.detectChanges();

        expect(component['siteOptions']().map((entry) => entry.domain)).toEqual(['alpha.example.com', 'shop.example.com', 'zeta.example.com']);
        expect(apiClientsServiceMock.listClients).toHaveBeenCalled();
    });

    it('uses team-scoped endpoints and hides instance role selection in team mode', async () => {
        apiClientsServiceMock.listClients.mockClear();

        fixture.componentRef.setInput('scope', 'team');
        fixture.componentRef.setInput('teamId', 'team-123');
        fixture.detectChanges();
        await fixture.whenStable();

        const calls = (apiClientsServiceMock.listClients as unknown as { mock: { calls: unknown[][] } }).mock.calls;
        expect(calls[calls.length - 1][0]).toBe('team-123');
        expect(fixture.nativeElement.textContent).toContain('Team API clients');

        component['openCreateDialog']();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.body.textContent).not.toContain('Instance role');
    });

    it('renders the API clients table first and opens create in a dialog', async () => {
        expect(fixture.nativeElement.querySelector('.api-client-form')).toBeNull();
        expect(fixture.nativeElement.querySelector('.api-client-listing')).not.toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Add API client');

        const addButton = Array.from(fixture.nativeElement.querySelectorAll('button') as NodeListOf<HTMLButtonElement>).find((button) => button.textContent?.includes('Add API client'));
        addButton?.click();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.body.textContent).toContain('Add API client');
        expect(document.body.querySelector('.api-client-form')).not.toBeNull();
    });

    it('forces team clients to use the user instance role when submitting', () => {
        fixture.componentRef.setInput('scope', 'team');
        fixture.componentRef.setInput('teamId', 'team-123');
        fixture.detectChanges();

        component['form'].setValue({
            name: 'Shared automation',
            description: 'Team token',
            instanceRole: 'owner',
            expiresAt: null
        });

        const payload = component['buildPayload']();
        expect(payload?.instance_role).toBe('user');
    });

    it('renders empty site grants as no site access instead of unrestricted access', () => {
        component['clients'].set([
            {
                id: 'client-1',
                owner_type: 'personal',
                name: 'Admin automation',
                description: '',
                instance_role: 'admin',
                created_at: '2026-01-01T00:00:00Z',
                updated_at: '2026-01-01T00:00:00Z',
                site_roles: []
            }
        ]);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Site grants');
        expect(fixture.nativeElement.textContent).toContain('Instance/admin only; no site access');
        expect(fixture.nativeElement.querySelector('app-site-scope-summary .pi-ban')).not.toBeNull();
    });

    it('renders client status as a color-coded icon next to the name', () => {
        component['clients'].set([
            {
                ...defaultClient,
                id: 'active-client',
                name: 'Active client'
            },
            {
                ...defaultClient,
                id: 'inactive-client',
                name: 'Inactive client',
                revoked_at: '2026-01-01T00:00:00Z'
            },
            {
                ...defaultClient,
                id: 'expired-client',
                name: 'Expired client',
                expires_at: '2020-01-01T00:00:00Z'
            }
        ]);
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('.hk-status-icon--ok.pi-check-circle')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('.hk-status-icon--error.pi-ban')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('.hk-status-icon--warn.pi-clock')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('.api-client-status')).toBeNull();
    });

    it('renders site grants as OptimusUI tags', () => {
        siteServiceMock.sites.set([site('site-1', 'shop.example.com')]);
        component['clients'].set([
            {
                ...defaultClient,
                site_roles: [{ site_id: 'site-1', role: 'viewer' }]
            }
        ]);
        fixture.detectChanges();

        const grantTag = fixture.nativeElement.querySelector('app-site-scope-summary .hk-site-scope-tag') as HTMLElement | null;
        expect(grantTag?.textContent).toContain('shop.example.com');
        expect(grantTag?.textContent).toContain('Viewer');
        expect(grantTag?.querySelector('.pi-globe')).not.toBeNull();
    });

    it('summarizes multiple site grants with a OptimusUI popover for the full scope', async () => {
        siteServiceMock.sites.set([site('site-1', 'search-reconnect.example.com'), site('site-2', 'search-pending.example.com'), site('site-3', 'search-quota.example.com')]);
        component['clients'].set([
            {
                ...defaultClient,
                site_roles: [
                    { site_id: 'site-1', role: 'viewer' },
                    { site_id: 'site-2', role: 'admin' },
                    { site_id: 'site-3', role: 'editor' }
                ]
            }
        ]);
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('app-site-scope-summary')?.textContent).toContain('3 site grants');

        const trigger = fixture.nativeElement.querySelector('app-site-scope-summary button') as HTMLButtonElement;
        expect(trigger.textContent).toContain('3 site grants');
        trigger.click();
        fixture.detectChanges();
        await fixture.whenStable();

        const popoverText = document.body.textContent ?? '';
        expect(popoverText).toContain('search-reconnect.example.com');
        expect(popoverText).toContain('search-pending.example.com');
        expect(popoverText).toContain('search-quota.example.com');
        expect(popoverText).toContain('Admin');
        expect(popoverText).toContain('Editor');
    });

    it('renders the API client grant picker with favicon-backed site options', async () => {
        siteServiceMock.sites.set([site('site-alpha', 'alpha.example.com'), site('site-zeta', 'zeta.example.com')]);
        component['openCreateDialog']();
        component['siteRoleForm'].controls.siteID.setValue('site-alpha');
        fixture.detectChanges();
        await fixture.whenStable();

        const dialog = document.body.querySelector('.p-dialog') as HTMLElement | null;

        expect(dialog?.querySelector('app-site-select-option img[alt=""][aria-hidden="true"]')).not.toBeNull();
        expect(dialog?.textContent).toContain('alpha.example.com');
    });

    it('rotates active clients and shows the one-time token', () => {
        const client = {
            id: 'client-1',
            owner_type: 'personal' as const,
            name: 'Reader',
            description: '',
            instance_role: 'user' as const,
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
            site_roles: []
        };
        const rotated = { ...client, updated_at: '2026-01-02T00:00:00Z' };
        apiClientsServiceMock.rotateClient.mockReturnValueOnce(of({ client: rotated, token: 'hk_new_token' }));
        component['clients'].set([client]);

        component['rotateClient'](client);
        fixture.detectChanges();

        expect((apiClientsServiceMock.rotateClient as unknown as { mock: { calls: unknown[][] } }).mock.calls[0]).toEqual(['client-1', null]);
        expect(component['createdToken']()).toBe('hk_new_token');
        expect(component['clients']()[0].updated_at).toBe('2026-01-02T00:00:00Z');
    });

    it('shows row-action feedback near the API client table', () => {
        const client = {
            ...defaultClient,
            id: 'client-1',
            name: 'Reader'
        };
        const rotated = { ...client, updated_at: '2026-01-02T00:00:00Z' };
        apiClientsServiceMock.rotateClient.mockReturnValueOnce(of({ client: rotated, token: 'hk_new_token' }));
        component['clients'].set([client]);

        component['rotateClient'](client);
        fixture.detectChanges();

        const listing = fixture.nativeElement.querySelector('.api-client-listing') as HTMLElement;

        expect(listing.textContent).toContain('Rotated');
        expect(listing.textContent).toContain('hk_new_token');
        expect(document.body.querySelector('.p-dialog')?.textContent ?? '').not.toContain('Rotated');
    });

    it('creates clients in a dialog and shows the one-time token near the table', async () => {
        const created = { ...defaultClient, id: 'client-created', name: 'Created client' };
        apiClientsServiceMock.createClient.mockReturnValueOnce(of({ client: created, token: 'hk_created_token' }));
        component['openCreateDialog']();
        fixture.detectChanges();
        await fixture.whenStable();
        component['form'].setValue({
            name: 'Created client',
            description: '',
            instanceRole: 'user',
            expiresAt: null
        });

        component['submit']();
        fixture.detectChanges();

        const listing = fixture.nativeElement.querySelector('.api-client-listing') as HTMLElement;

        expect(component['isFormDialogVisible']()).toBe(false);
        expect(listing.textContent).toContain('Created');
        expect(listing.textContent).toContain('hk_created_token');
        expect(document.body.querySelector('.p-dialog')?.textContent ?? '').not.toContain('hk_created_token');
    });

    it('ignores duplicate create submits while the request is in flight', () => {
        const pending = new Subject<{ client: APIClient; token: string }>();
        apiClientsServiceMock.createClient.mockReturnValueOnce(pending.asObservable());
        component['openCreateDialog']();
        component['form'].setValue({
            name: 'Created client',
            description: '',
            instanceRole: 'user',
            expiresAt: null
        });

        component['submit']();
        component['submit']();

        expect(apiClientsServiceMock.createClient).toHaveBeenCalledTimes(1);
        pending.complete();
    });

    it('keeps create errors inside the dialog', async () => {
        apiClientsServiceMock.createClient.mockReturnValueOnce(throwError(() => new Error('nope')));
        component['openCreateDialog']();
        component['form'].setValue({
            name: 'Created client',
            description: '',
            instanceRole: 'user',
            expiresAt: null
        });

        component['submit']();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.body.querySelector('.p-dialog')?.textContent).toContain('Create failed');
        expect(fixture.nativeElement.querySelector('.api-client-listing')?.textContent).not.toContain('Create failed');
    });

    it('opens edit in the CRUD dialog from row actions', async () => {
        component['clients'].set([{ ...defaultClient, name: 'Editable client', description: 'Existing description' }]);
        fixture.detectChanges();

        component['startEdit']({ ...defaultClient, name: 'Editable client', description: 'Existing description' });
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.body.textContent).toContain('Edit API client');
        expect((document.body.querySelector('#api-client-name') as HTMLInputElement | null)?.value).toBe('Editable client');
    });

    it('does not rotate revoked or expired clients', () => {
        const revoked = {
            id: 'client-1',
            owner_type: 'personal' as const,
            name: 'Revoked',
            description: '',
            instance_role: 'user' as const,
            revoked_at: '2026-01-01T00:00:00Z',
            created_at: '2026-01-01T00:00:00Z',
            updated_at: '2026-01-01T00:00:00Z',
            site_roles: []
        };
        const expired = { ...revoked, id: 'client-2', revoked_at: null, expires_at: '2020-01-01T00:00:00Z' };

        expect(component['canRotateClient'](revoked)).toBe(false);
        expect(component['canRotateClient'](expired)).toBe(false);
    });

    it('renders row actions through a shared popup menu', async () => {
        component['clients'].set([defaultClient]);
        fixture.detectChanges();

        const triggers = fixture.nativeElement.querySelectorAll('button[aria-label="More actions"]');
        expect(triggers.length).toBe(1);

        triggers[0].click();
        fixture.detectChanges();
        await fixture.whenStable();

        const menuText = document.body.textContent ?? '';
        expect(menuText).toContain('Edit');
        expect(menuText).toContain('Roll token');
        expect(menuText).toContain('Revoke');
        expect(menuText).toContain('Delete');
    });

    it('disables token rotation in the row menu for revoked clients', () => {
        const revoked = {
            ...defaultClient,
            revoked_at: '2026-01-01T00:00:00Z'
        };

        const rollTokenAction = component['apiClientActions'](revoked).find((action) => action['label'] === 'Roll token');

        expect(rollTokenAction?.['disabled']).toBe(true);
    });

    it('opens a confirmation dialog before deleting from the row menu', () => {
        const confirmationService = fixture.debugElement.injector.get(ConfirmationService);
        const confirmSpy = vi.spyOn(confirmationService, 'confirm').mockReturnValue(confirmationService);
        const deleteAction = component['apiClientActions'](defaultClient).find((action) => action['label'] === 'Delete');

        deleteAction?.['command']?.({} as never);

        expect(confirmSpy).toHaveBeenCalledTimes(1);
        expect(apiClientsServiceMock.deleteClient).not.toHaveBeenCalled();

        const confirmation = confirmSpy.mock.calls[0]?.[0];
        confirmation?.accept?.();

        expect((apiClientsServiceMock.deleteClient as unknown as { mock: { calls: unknown[][] } }).mock.calls[0]).toEqual([defaultClient.id, null]);
    });
});
