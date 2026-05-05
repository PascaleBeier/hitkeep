import { signal } from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { HttpErrorResponse } from '@angular/common/http';
import { TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { ConfirmationService } from 'primeng/api';
import { provideRouter } from '@angular/router';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { vi } from 'vitest';

import { UserProfileService } from '@services/user-profile.service';
import { PermissionService } from '@services/permission.service';
import { AdminSettings } from './admin-settings';

interface AdminSettingsTestAccess {
    handleDeleteUserError(err: unknown, user: { email: string }): boolean;
    resolveDeleteErrorKey(err: unknown, fallbackKey: string): string;
    confirmDeleteSite(
        event: Event,
        site: {
            id: string;
            domain: string;
            user_id: string;
            created_at: string;
        }
    ): void;
    deleteUserBlock(): {
        email: string;
        teams: string[];
    } | null;
    deleteUserBlockMessage(): string;
    userActionStatus: {
        set(
            value: {
                severity: 'success' | 'error';
                key: string;
                params?: Record<string, string | number>;
            } | null
        ): void;
    };
    userActionStatusMessage(): string;
    siteActionStatus(): {
        severity: 'success' | 'error';
        key: string;
        params?: Record<string, string | number>;
    } | null;
    siteActionStatusMessage(): string;
    deletingSiteId(): string;
    isRefreshingSpam(): boolean;
    refreshSpamFilter(): void;
    spamActionStatus(): {
        severity: 'success' | 'error';
        key: string;
        params?: Record<string, string | number>;
    } | null;
    spamActionStatusMessage(): string;
    isRunningImportCleanup(): boolean;
    runImportStageCleanup(): void;
    importCleanupActionStatus(): {
        severity: 'success' | 'error';
        key: string;
        params?: Record<string, string | number>;
    } | null;
    importCleanupActionStatusMessage(): string;
    isLoadingSearchConsole(): boolean;
    loadSearchConsoleStatus(): void;
    systemSearchConsole(): {
        status: string;
        credentials_status: string;
        worker_status: string;
        sync_status: string;
        connected_teams: number;
        mapped_sites: number;
        pending_syncs: number;
        running_syncs: number;
        failed_syncs: number;
        needs_attention_syncs: number;
    } | null;
    searchConsoleSyncIssueCount(): number;
    canDisableUserMfa(): boolean;
    currentUserId: { set(value: string): void };
    users: {
        set(
            value: {
                id: string;
                email: string;
                instance_role: 'owner' | 'admin' | 'user';
                created_at: string;
            }[]
        ): void;
    };
}

describe('AdminSettings', () => {
    let component: AdminSettingsTestAccess;
    let httpMock: HttpTestingController;
    let confirmationServiceMock: {
        confirm: ReturnType<typeof vi.fn>;
    };
    const permissionServiceMock = {
        isInstanceOwner: signal(false),
        permissions: signal(null)
    };

    beforeEach(() => {
        confirmationServiceMock = {
            confirm: vi.fn()
        };

        TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            admin: {
                                errors: {
                                    deleteUserBlockedOwnership: 'Cannot delete {{email}} until ownership is transferred for: {{teams}}.',
                                    deleteSiteFailed: 'Could not delete site {{domain}}.',
                                    deleteForbidden: 'You do not have permission to delete this resource.',
                                    deleteNotFound: 'This resource no longer exists.',
                                    deleteUnavailable: 'Deletion is only available on the active instance node.',
                                    deleteDefaultTeam: 'The default team cannot be deleted.',
                                    deleteTeamNotArchived: 'Archive the team before deleting it.',
                                    deleteTeamHasSites: 'Transfer or delete all sites before deleting this team.'
                                },
                                status: {
                                    deleteUserSuccess: 'Deleted user {{email}}.',
                                    deleteSiteSuccess: 'Deleted site {{domain}}.'
                                },
                                system: {
                                    spam: {
                                        refreshTriggered: 'Spam filter refreshed.',
                                        refreshFailed: 'Could not refresh the spam filter.'
                                    },
                                    importCleanup: {
                                        runSuccess: 'Cleaned {{files}} staged file(s), freeing {{bytes}}.',
                                        runFailed: 'Could not clean staged import files.'
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
            providers: [
                provideRouter([]),
                provideHttpClient(),
                provideHttpClientTesting(),
                {
                    provide: ConfirmationService,
                    useValue: confirmationServiceMock
                },
                {
                    provide: UserProfileService,
                    useValue: {
                        profile: signal({ id: 'admin-user', email: 'admin@example.com' }),
                        loadProfile: vi.fn()
                    }
                },
                {
                    provide: PermissionService,
                    useValue: permissionServiceMock
                }
            ]
        });

        permissionServiceMock.isInstanceOwner.set(false);
        permissionServiceMock.permissions.set(null);
        httpMock = TestBed.inject(HttpTestingController);
        component = TestBed.runInInjectionContext(() => new AdminSettings()) as unknown as AdminSettingsTestAccess;
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('stores blocking team details for sole-owner delete errors', () => {
        const handled = component.handleDeleteUserError(
            new HttpErrorResponse({
                status: 409,
                error: {
                    status: 'error',
                    code: 'user_owns_teams',
                    message: 'Transfer ownership before deleting this user.',
                    teams: [
                        { id: 'team-1', name: 'Acme' },
                        { id: 'team-2', name: 'Northwind Studio' }
                    ]
                }
            }),
            { email: 'owner@example.com' }
        );

        expect(handled).toBe(true);
        expect(component.deleteUserBlock()).toEqual({
            email: 'owner@example.com',
            teams: ['Acme', 'Northwind Studio']
        });
        expect(component.deleteUserBlockMessage()).toContain('owner@example.com');
        expect(component.deleteUserBlockMessage()).toContain('Acme, Northwind Studio');
    });

    it('ignores unrelated delete errors', () => {
        const handled = component.handleDeleteUserError(
            new HttpErrorResponse({
                status: 500,
                error: {
                    status: 'error',
                    code: 'unexpected',
                    message: 'Unexpected failure'
                }
            }),
            { email: 'owner@example.com' }
        );

        expect(handled).toBe(false);
        expect(component.deleteUserBlock()).toBeNull();
    });

    it('renders inline admin action feedback messages', () => {
        component.userActionStatus.set({
            severity: 'success',
            key: 'admin.status.deleteUserSuccess',
            params: { email: 'owner@example.com' }
        });

        expect(component.userActionStatusMessage()).toBe('Deleted user owner@example.com.');
    });

    it('maps delete errors to specific in-place messages when the API gives useful context', () => {
        expect(component.resolveDeleteErrorKey(new HttpErrorResponse({ status: 403 }), 'fallback')).toBe('admin.errors.deleteForbidden');
        expect(component.resolveDeleteErrorKey(new HttpErrorResponse({ status: 404 }), 'fallback')).toBe('admin.errors.deleteNotFound');
        expect(component.resolveDeleteErrorKey(new HttpErrorResponse({ status: 503 }), 'fallback')).toBe('admin.errors.deleteUnavailable');
        expect(component.resolveDeleteErrorKey(new HttpErrorResponse({ status: 400, error: 'Default team cannot be deleted.' }), 'fallback')).toBe('admin.errors.deleteDefaultTeam');
        expect(component.resolveDeleteErrorKey(new HttpErrorResponse({ status: 400, error: 'Archive the team before deleting it.' }), 'fallback')).toBe('admin.errors.deleteTeamNotArchived');
        expect(component.resolveDeleteErrorKey(new HttpErrorResponse({ status: 400, error: 'Transfer or delete all sites before deleting this team.' }), 'fallback')).toBe('admin.errors.deleteTeamHasSites');
        expect(component.resolveDeleteErrorKey(new HttpErrorResponse({ status: 500, error: 'Unexpected failure' }), 'fallback')).toBe('fallback');
    });

    it('shows in-place success feedback after deleting a site', () => {
        confirmationServiceMock.confirm.mockImplementation((options: { accept?: () => void }) => options.accept?.());

        component.confirmDeleteSite({ currentTarget: document.createElement('button') } as unknown as Event, {
            id: 'site-1',
            domain: 'example.com',
            user_id: 'user-1',
            created_at: '2026-03-10T00:00:00Z'
        });

        expect(component.deletingSiteId()).toBe('site-1');
        const deleteRequest = httpMock.expectOne('/api/admin/sites/site-1');
        expect(deleteRequest.request.method).toBe('DELETE');
        deleteRequest.flush({ status: 'ok' });

        const reloadRequest = httpMock.expectOne('/api/admin/sites');
        reloadRequest.flush([]);

        expect(component.deletingSiteId()).toBe('');
        expect(component.siteActionStatus()).toEqual({
            severity: 'success',
            key: 'admin.status.deleteSiteSuccess',
            params: { domain: 'example.com' }
        });
        expect(component.siteActionStatusMessage()).toBe('Deleted site example.com.');
    });

    it('shows in-place error feedback when deleting a site fails', () => {
        const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
        confirmationServiceMock.confirm.mockImplementation((options: { accept?: () => void }) => options.accept?.());

        component.confirmDeleteSite({ currentTarget: document.createElement('button') } as unknown as Event, {
            id: 'site-1',
            domain: 'example.com',
            user_id: 'user-1',
            created_at: '2026-03-10T00:00:00Z'
        });

        const deleteRequest = httpMock.expectOne('/api/admin/sites/site-1');
        deleteRequest.flush('Failed to delete site', { status: 500, statusText: 'Server Error' });

        expect(component.deletingSiteId()).toBe('');
        expect(component.siteActionStatus()).toEqual({
            severity: 'error',
            key: 'admin.errors.deleteSiteFailed',
            params: { domain: 'example.com' }
        });
        expect(component.siteActionStatusMessage()).toBe('Could not delete site example.com.');

        consoleSpy.mockRestore();
    });

    it('shows in-place success feedback after refreshing the spam filter', () => {
        component.refreshSpamFilter();

        expect(component.isRefreshingSpam()).toBe(true);
        const refreshRequest = httpMock.expectOne('/api/admin/system/spam-filter/refresh');
        expect(refreshRequest.request.method).toBe('POST');
        refreshRequest.flush({ status: 'ok', message: 'refreshed' });

        const reloadRequest = httpMock.expectOne('/api/admin/system/spam-filter');
        reloadRequest.flush({
            db_path: '/tmp/spam-filter.json',
            rule_count: 10,
            auto_update: true
        });

        expect(component.isRefreshingSpam()).toBe(false);
        expect(component.spamActionStatus()).toEqual({
            severity: 'success',
            key: 'admin.system.spam.refreshTriggered'
        });
        expect(component.spamActionStatusMessage()).toBe('Spam filter refreshed.');
    });

    it('shows in-place error feedback when refreshing the spam filter fails', () => {
        component.refreshSpamFilter();

        const refreshRequest = httpMock.expectOne('/api/admin/system/spam-filter/refresh');
        refreshRequest.flush('refresh failed', { status: 500, statusText: 'Server Error' });

        expect(component.isRefreshingSpam()).toBe(false);
        expect(component.spamActionStatus()).toEqual({
            severity: 'error',
            key: 'admin.system.spam.refreshFailed'
        });
        expect(component.spamActionStatusMessage()).toBe('Could not refresh the spam filter.');
    });

    it('shows in-place feedback after running import stage cleanup', () => {
        component.runImportStageCleanup();

        expect(component.isRunningImportCleanup()).toBe(true);
        const runRequest = httpMock.expectOne('/api/admin/system/import-stage-cleanup/run');
        expect(runRequest.request.method).toBe('POST');
        runRequest.flush({
            status: 'ok',
            message: 'completed',
            result: {
                imports_cleaned: 1,
                files_cleaned: 2,
                bytes_cleaned: 2048,
                imports_marked_failed: 1
            }
        });

        const reloadRequest = httpMock.expectOne('/api/admin/system/import-stage-cleanup');
        reloadRequest.flush({
            enabled: true,
            retention_days: 7,
            stale_imports: 0,
            stale_files: 0,
            stale_bytes: 0,
            recent_failures: 0,
            last_cleaned_imports: 1,
            last_cleaned_files: 2,
            last_cleaned_bytes: 2048,
            last_marked_failed: 1
        });

        expect(component.isRunningImportCleanup()).toBe(false);
        expect(component.importCleanupActionStatus()).toEqual({
            severity: 'success',
            key: 'admin.system.importCleanup.runSuccess',
            params: { files: 2, bytes: '2 KB' }
        });
        expect(component.importCleanupActionStatusMessage()).toBe('Cleaned 2 staged file(s), freeing 2 KB.');
    });

    it('shows in-place error feedback when import stage cleanup fails', () => {
        component.runImportStageCleanup();

        const runRequest = httpMock.expectOne('/api/admin/system/import-stage-cleanup/run');
        runRequest.flush('cleanup failed', { status: 500, statusText: 'Server Error' });

        expect(component.isRunningImportCleanup()).toBe(false);
        expect(component.importCleanupActionStatus()).toEqual({
            severity: 'error',
            key: 'admin.system.importCleanup.runFailed'
        });
        expect(component.importCleanupActionStatusMessage()).toBe('Could not clean staged import files.');
    });

    it('loads Search Console system status for the runtime console', () => {
        component.loadSearchConsoleStatus();

        expect(component.isLoadingSearchConsole()).toBe(true);
        const request = httpMock.expectOne('/api/admin/system/search-console');
        expect(request.request.method).toBe('GET');
        request.flush({
            status: 'needs_attention',
            credentials_status: 'configured',
            worker_status: 'enabled',
            sync_status: 'needs_attention',
            connected_teams: 1,
            mapped_sites: 2,
            pending_syncs: 1,
            running_syncs: 0,
            failed_syncs: 1,
            needs_attention_syncs: 2
        });

        expect(component.isLoadingSearchConsole()).toBe(false);
        expect(component.systemSearchConsole()?.status).toBe('needs_attention');
        expect(component.searchConsoleSyncIssueCount()).toBe(3);
    });

    it('allows MFA recovery actions when the current user is an instance owner', () => {
        permissionServiceMock.isInstanceOwner.set(true);

        expect(component.canDisableUserMfa()).toBe(true);
    });

    it('falls back to the loaded current user role for MFA recovery actions', () => {
        component.currentUserId.set('owner-user');
        component.users.set([
            {
                id: 'owner-user',
                email: 'owner@example.com',
                instance_role: 'owner',
                created_at: '2026-03-10T00:00:00Z'
            }
        ]);

        expect(component.canDisableUserMfa()).toBe(true);
    });
});
