import { TestBed } from '@angular/core/testing';
import { Router, UrlTree, provideRouter } from '@angular/router';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { PermissionService } from '@services/permission.service';
import { SiteService } from '@features/sites/services/site.service';
import { importExportDefaultGuard } from './import-export-default.guard';

describe('importExportDefaultGuard', () => {
    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()]
        });
    });

    it('should send users who can manage the active site to the Import tab', () => {
        const siteService = TestBed.inject(SiteService);
        const permissions = TestBed.inject(PermissionService);
        siteService.applySites([
            {
                id: '00000000-0000-0000-0000-0000000000aa',
                user_id: '00000000-0000-0000-0000-000000000001',
                domain: 'managed.example.com',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);
        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {
                '00000000-0000-0000-0000-0000000000aa': 'admin'
            }
        });

        const target = TestBed.runInInjectionContext(() => importExportDefaultGuard({} as never, {} as never));

        expect(serialize(target)).toBe('/import-export/import');
    });

    it('should send viewers and users without an active importable site to the Export tab', () => {
        const siteService = TestBed.inject(SiteService);
        const permissions = TestBed.inject(PermissionService);
        siteService.applySites([
            {
                id: '00000000-0000-0000-0000-0000000000bb',
                user_id: '00000000-0000-0000-0000-000000000001',
                domain: 'viewer.example.com',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);
        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {
                '00000000-0000-0000-0000-0000000000bb': 'viewer'
            }
        });

        const target = TestBed.runInInjectionContext(() => importExportDefaultGuard({} as never, {} as never));

        expect(serialize(target)).toBe('/import-export/export');
    });
});

function serialize(target: unknown): string {
    expect(target).toBeInstanceOf(UrlTree);
    return TestBed.inject(Router).serializeUrl(target as UrlTree);
}
