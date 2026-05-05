import { provideHttpClient } from '@angular/common/http';
import { TestBed } from '@angular/core/testing';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';

import { GoogleSearchConsoleService } from './google-search-console.service';

describe('GoogleSearchConsoleService', () => {
    let service: GoogleSearchConsoleService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [GoogleSearchConsoleService, provideHttpClient(), provideHttpClientTesting()]
        });

        service = TestBed.inject(GoogleSearchConsoleService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('loads team connection status', () => {
        let status = '';

        service.getStatus('team-1').subscribe((response) => {
            status = response.status;
        });

        const req = httpMock.expectOne('/api/user/teams/team-1/integrations/google-search-console/status');
        expect(req.request.method).toBe('GET');
        req.flush({
            status: 'credentials_missing',
            configured: false,
            connected: false,
            credential_status: 'missing',
            needs_admin_action: true,
            can_manage: true,
            managed_credentials_mode: 'self_hosted'
        });

        expect(status).toBe('credentials_missing');
    });

    it('starts OAuth with a local return path', () => {
        let authURL = '';

        service.connect('team-1', '/integration/google-search-console').subscribe((response) => {
            authURL = response.auth_url;
        });

        const req = httpMock.expectOne('/api/user/teams/team-1/integrations/google-search-console/connect');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toEqual({ return_path: '/integration/google-search-console' });
        req.flush({ auth_url: 'https://accounts.example.test/oauth' });

        expect(authURL).toBe('https://accounts.example.test/oauth');
    });

    it('disconnects the team connection', () => {
        let ok = false;

        service.disconnect('team-1').subscribe((response) => {
            ok = response.status === 'ok';
        });

        const req = httpMock.expectOne('/api/user/teams/team-1/integrations/google-search-console');
        expect(req.request.method).toBe('DELETE');
        req.flush({ status: 'ok' });

        expect(ok).toBe(true);
    });

    it('loads connected Search Console properties for a team', () => {
        let propertyURI = '';

        service.listProperties('team-1').subscribe((response) => {
            propertyURI = response.properties[0]?.uri ?? '';
        });

        const req = httpMock.expectOne('/api/user/teams/team-1/integrations/google-search-console/properties');
        expect(req.request.method).toBe('GET');
        req.flush({
            properties: [{ uri: 'sc-domain:example.com', permission_level: 'siteOwner' }]
        });

        expect(propertyURI).toBe('sc-domain:example.com');
    });

    it('maps and unmaps a Search Console property for a site', () => {
        const results: string[] = [];

        service.mapSiteProperty('site-1', 'sc-domain:example.com').subscribe((response) => {
            results.push(response.property_uri ?? '');
        });

        const mapReq = httpMock.expectOne('/api/sites/site-1/integrations/google-search-console/property');
        expect(mapReq.request.method).toBe('PUT');
        expect(mapReq.request.body).toEqual({ property_uri: 'sc-domain:example.com' });
        mapReq.flush({
            site_id: 'site-1',
            team_id: 'team-1',
            mapped: true,
            property_uri: 'sc-domain:example.com',
            property_permission_level: 'siteOwner',
            can_manage: true
        });

        service.unmapSiteProperty('site-1').subscribe((response) => {
            results.push(response.mapped ? 'mapped' : 'unmapped');
        });

        const unmapReq = httpMock.expectOne('/api/sites/site-1/integrations/google-search-console/property');
        expect(unmapReq.request.method).toBe('DELETE');
        unmapReq.flush({
            site_id: 'site-1',
            team_id: 'team-1',
            mapped: false,
            can_manage: true
        });

        expect(results).toEqual(['sc-domain:example.com', 'unmapped']);
    });

    it('requests a manual Search Console sync for a site', () => {
        let syncState = '';

        service.requestSync('site-1').subscribe((response) => {
            syncState = response.sync_status?.state ?? '';
        });

        const req = httpMock.expectOne('/api/sites/site-1/integrations/google-search-console/sync');
        expect(req.request.method).toBe('POST');
        expect(req.request.body).toBeNull();
        req.flush({
            site_id: 'site-1',
            team_id: 'team-1',
            mapped: true,
            property_uri: 'sc-domain:example.com',
            can_manage: true,
            sync_status: {
                state: 'pending',
                manual: true
            }
        });

        expect(syncState).toBe('pending');
    });

    it('loads Search Console drilldown reports with compatible filters', () => {
        let clicks = 0;

        service
            .getOverview('site-1', {
                from: '2026-05-01T00:00:00Z',
                to: '2026-05-05T00:00:00Z',
                path: '/docs',
                country: 'US',
                device: 'desktop'
            })
            .subscribe((response) => {
                clicks = response.clicks;
            });

        const req = httpMock.expectOne('/api/sites/site-1/search-console/overview?from=2026-05-01T00:00:00Z&to=2026-05-05T00:00:00Z&path=/docs&country=US&device=desktop');
        expect(req.request.method).toBe('GET');
        req.flush({
            data_source: 'google_search_console',
            clicks: 12,
            impressions: 120,
            ctr: 0.1,
            average_position: 3.4
        });

        expect(clicks).toBe(12);
    });
});
