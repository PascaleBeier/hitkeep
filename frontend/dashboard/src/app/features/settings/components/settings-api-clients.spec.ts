import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { of } from 'rxjs';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { SettingsAPIClients } from './settings-api-clients';
import { APIClientsService } from '@services/api-clients.service';
import { PermissionService } from '@services/permission.service';

describe('SettingsAPIClients', () => {
    let fixture: ComponentFixture<SettingsAPIClients>;
    let component: SettingsAPIClients;

    const apiClientsServiceMock = {
        listClients: vi.fn(() => of([])),
        listSites: vi.fn(() => of([])),
        createClient: vi.fn(() => of({ client: null, token: '' })),
        updateClient: vi.fn(() => of(null)),
        deleteClient: vi.fn(() => of(void 0))
    };

    const permissionServiceMock = {
        permissions: signal({
            instance_role: 'owner' as const,
            permissions: {}
        })
    };

    beforeEach(async () => {
        vi.clearAllMocks();

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
                                    confirmDelete: 'Delete',
                                    status: { active: 'Active', revoked: 'Revoked' },
                                    meta: {
                                        instanceRole: 'Instance role',
                                        created: 'Created',
                                        lastUsed: 'Last used',
                                        expires: 'Expires'
                                    },
                                    actions: {
                                        create: 'Create',
                                        save: 'Save',
                                        edit: 'Edit',
                                        revoke: 'Revoke',
                                        reactivate: 'Reactivate',
                                        delete: 'Delete',
                                        addScope: 'Add scope',
                                        refresh: 'Refresh'
                                    },
                                    form: {
                                        nameLabel: 'Client name',
                                        namePlaceholder: 'Name',
                                        descriptionLabel: 'Description',
                                        descriptionPlaceholder: 'Description',
                                        instanceRoleLabel: 'Instance role',
                                        expiresAtLabel: 'Expiration',
                                        expiresAtHint: 'Hint',
                                        siteScopesLabel: 'Site scopes',
                                        siteScopesHint: 'Personal scopes',
                                        teamSiteScopesHint: 'Team scopes',
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
                                        reactivated: 'Reactivated'
                                    },
                                    errors: {
                                        loadFailed: 'Load failed',
                                        createFailed: 'Create failed',
                                        updateFailed: 'Update failed',
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
                { provide: APIClientsService, useValue: apiClientsServiceMock },
                { provide: PermissionService, useValue: permissionServiceMock }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(SettingsAPIClients);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('loads personal API clients by default', () => {
        const calls = (apiClientsServiceMock.listClients as unknown as { mock: { calls: unknown[][] } }).mock.calls;
        expect(calls[0][0]).toBeNull();
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
        expect(fixture.nativeElement.textContent).not.toContain('Instance role');
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
});
