import { TestBed } from '@angular/core/testing';
import { Router, UrlTree, provideRouter } from '@angular/router';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { SiteService } from '@features/sites/services/site.service';
import { overviewDefaultGuard } from './overview-default.guard';

describe('overviewDefaultGuard', () => {
    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()]
        });
        localStorage.clear();
    });

    it('sends users with multiple sites to the overview page', () => {
        TestBed.inject(SiteService).applySites([
            {
                id: '00000000-0000-0000-0000-0000000000aa',
                user_id: '00000000-0000-0000-0000-000000000001',
                domain: 'alpha.example.com',
                created_at: '2026-01-01T00:00:00Z'
            },
            {
                id: '00000000-0000-0000-0000-0000000000bb',
                user_id: '00000000-0000-0000-0000-000000000001',
                domain: 'beta.example.com',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);

        const target = TestBed.runInInjectionContext(() => overviewDefaultGuard({} as never, {} as never));

        expect(serialize(target)).toBe('/overview');
    });

    it('keeps users with zero or one site on the dashboard page', () => {
        const siteService = TestBed.inject(SiteService);

        let target = TestBed.runInInjectionContext(() => overviewDefaultGuard({} as never, {} as never));
        expect(serialize(target)).toBe('/dashboard');

        siteService.applySites([
            {
                id: '00000000-0000-0000-0000-0000000000aa',
                user_id: '00000000-0000-0000-0000-000000000001',
                domain: 'alpha.example.com',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);

        target = TestBed.runInInjectionContext(() => overviewDefaultGuard({} as never, {} as never));
        expect(serialize(target)).toBe('/dashboard');
    });
});

function serialize(target: unknown): string {
    expect(target).toBeInstanceOf(UrlTree);
    return TestBed.inject(Router).serializeUrl(target as UrlTree);
}
