import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';

import { ExclusionsService } from './exclusions.service';

describe('ExclusionsService', () => {
    let service: ExclusionsService;
    let http: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
        service = TestBed.inject(ExclusionsService);
        http = TestBed.inject(HttpTestingController);
    });

    afterEach(() => http.verify());

    it('requests effective site and team exclusions only when selected', () => {
        service.listSiteExclusions('site-1', true).subscribe();
        const site = http.expectOne((request) => request.url === '/api/sites/site-1/exclusions');
        expect(site.request.params.get('effective')).toBe('true');
        site.flush([]);

        service.listTeamExclusions('team-1', true).subscribe();
        const team = http.expectOne((request) => request.url === '/api/user/teams/team-1/exclusions');
        expect(team.request.params.get('effective')).toBe('true');
        team.flush([]);

        service.listTeamExclusions('team-1').subscribe();
        const owned = http.expectOne((request) => request.url === '/api/user/teams/team-1/exclusions');
        expect(owned.request.params.has('effective')).toBe(false);
        owned.flush([]);
    });

    it('uses the team CRUD routes', () => {
        service.createTeamExclusion('team-1', { type: 'path', path: '/admin' }).subscribe();
        const create = http.expectOne('/api/user/teams/team-1/exclusions');
        expect(create.request.method).toBe('POST');
        expect(create.request.body).toEqual({ type: 'path', path: '/admin' });
        create.flush({ id: 'rule-1', type: 'path', path: '/admin', created_at: '2026-01-01T00:00:00Z' });

        service.deleteTeamExclusion('team-1', 'rule-1').subscribe();
        const remove = http.expectOne('/api/user/teams/team-1/exclusions/rule-1');
        expect(remove.request.method).toBe('DELETE');
        remove.flush(null);
    });
});
