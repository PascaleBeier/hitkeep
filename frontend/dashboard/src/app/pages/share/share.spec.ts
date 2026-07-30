import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { Subject } from 'rxjs';
import { vi } from 'vitest';

import type { Site } from '@models/analytics.types';
import { SiteService } from '@features/sites/services/site.service';
import { ShareService } from '@services/share.service';
import { ShareDashboard } from './share';

describe('ShareDashboard route inputs', () => {
    let fixture: ComponentFixture<ShareDashboard>;
    let responses: Subject<Site>[];
    const sites = { set: vi.fn() };
    const activeSite = { set: vi.fn() };

    beforeEach(async () => {
        responses = [];
        sites.set.mockClear();
        activeSite.set.mockClear();

        await TestBed.configureTestingModule({
            imports: [
                ShareDashboard,
                TranslocoTestingModule.forRoot({
                    langs: { en: { share: { page: { loading: 'Loading', missingToken: 'Missing', invalidOrExpired: 'Invalid' } } } },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }
                })
            ],
            providers: [
                provideRouter([]),
                {
                    provide: ShareService,
                    useValue: {
                        loadShareSite: vi.fn(() => {
                            const response = new Subject<Site>();
                            responses.push(response);
                            return response.asObservable();
                        })
                    }
                },
                { provide: SiteService, useValue: { sites, activeSite } }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(ShareDashboard);
    });

    it('cancels a stale share load when the path token changes', () => {
        fixture.componentRef.setInput('token', 'old-token');
        fixture.detectChanges();
        expect(responses[0].observed).toBe(true);

        fixture.componentRef.setInput('token', 'new-token');
        fixture.detectChanges();
        expect(responses[0].observed).toBe(false);
        expect(responses[1].observed).toBe(true);

        const stale = { id: 'site-old' } as Site;
        const fresh = { id: 'site-new' } as Site;
        responses[0].next(stale);
        responses[1].next(fresh);

        expect(sites.set).toHaveBeenCalledWith([fresh]);
        expect(activeSite.set).toHaveBeenCalledWith(fresh);
        expect(activeSite.set).not.toHaveBeenCalledWith(stale);
    });

    it('tears down the active share load with the component', () => {
        fixture.componentRef.setInput('token', 'active-token');
        fixture.detectChanges();
        expect(responses[0].observed).toBe(true);

        fixture.destroy();
        expect(responses[0].observed).toBe(false);
    });
});
