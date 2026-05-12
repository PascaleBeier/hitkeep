import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';

import { PermissionService } from './permission.service';

describe('PermissionService', () => {
    let service: PermissionService;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [provideHttpClient(), provideHttpClientTesting()]
        });
        service = TestBed.inject(PermissionService);
    });

    it('does not treat instance admin as site data manager without the permission', () => {
        service.applyPermissions({
            instance_role: 'admin',
            instance_permissions: ['instance.view_all_sites', 'site.view'],
            permissions: {}
        });

        expect(service.canViewSite('site-1')).toBe(true);
        expect(service.canManageSite('site-1')).toBe(false);
    });

    it('allows site management when the user has site.manage_data through instance permissions', () => {
        service.applyPermissions({
            instance_role: 'owner',
            instance_permissions: ['site.view', 'site.manage_data'],
            permissions: {}
        });

        expect(service.canManageSite('site-1')).toBe(true);
    });

    it('allows site management for site owner and admin roles', () => {
        service.applyPermissions({
            instance_role: 'user',
            instance_permissions: [],
            permissions: {
                'site-owner': 'owner',
                'site-admin': 'admin',
                'site-editor': 'editor',
                'site-viewer': 'viewer'
            }
        });

        expect(service.canManageSite('site-owner')).toBe(true);
        expect(service.canManageSite('site-admin')).toBe(true);
        expect(service.canManageSite('site-editor')).toBe(false);
        expect(service.canManageSite('site-viewer')).toBe(false);
    });
});
