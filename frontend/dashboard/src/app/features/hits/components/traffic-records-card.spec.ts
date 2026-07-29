import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { TrafficRecordsCard } from './traffic-records-card';

describe('TrafficRecordsCard', () => {
    let http: HttpTestingController;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                TrafficRecordsCard,
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }
                })
            ],
            providers: [
                provideHttpClient(),
                provideHttpClientTesting(),
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: { en: 'en-US' }
                })
            ]
        }).compileComponents();
        http = TestBed.inject(HttpTestingController);
    });

    afterEach(() => http.verify());

    it('loads a union cohort with dimension filters in isolated table state', () => {
        const fixture = TestBed.createComponent(TrafficRecordsCard);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('siteDomain', 'example.com');
        fixture.componentRef.setInput('from', '2026-07-01T00:00:00Z');
        fixture.componentRef.setInput('to', '2026-07-29T23:59:59Z');
        fixture.componentRef.setInput('filters', [{ type: 'country', value: 'DE' }]);
        fixture.componentRef.setInput('goalIds', ['goal-1', 'goal-2']);
        fixture.componentRef.setInput('enabled', false);
        fixture.detectChanges();
        fixture.componentRef.setInput('enabled', true);

        const card = fixture.componentInstance as unknown as {
            loadHits: (event: { first: number; rows: number; sortField: string; sortOrder: number }) => void;
            exportUrl: () => string;
        };
        card.loadHits({ first: 25, rows: 25, sortField: 'timestamp', sortOrder: -1 });

        const request = http.expectOne((candidate) => candidate.url === '/api/sites/site-1/hits');
        expect(request.request.params.get('offset')).toBe('25');
        expect(request.request.params.getAll('filter')).toEqual(['country:DE']);
        expect(request.request.params.getAll('goal_id')).toEqual(['goal-1', 'goal-2']);
        expect(request.request.params.getAll('funnel_id')).toBeNull();
        expect(card.exportUrl()).toContain('goal_id=goal-1&goal_id=goal-2');
        request.flush({ data: [], total: 0 });
    });

    it('does not fall through to unfiltered traffic when no definitions exist', () => {
        const fixture = TestBed.createComponent(TrafficRecordsCard);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('from', '2026-07-01T00:00:00Z');
        fixture.componentRef.setInput('to', '2026-07-29T23:59:59Z');
        fixture.componentRef.setInput('enabled', false);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('trafficRecords.unavailableTitle');
        http.expectNone('/api/sites/site-1/hits');
    });

    it('gives the traffic search control an accessible name', () => {
        const fixture = TestBed.createComponent(TrafficRecordsCard);
        fixture.componentRef.setInput('enabled', true);
        fixture.detectChanges();

        expect((fixture.nativeElement as HTMLElement).querySelector('input')?.getAttribute('aria-label')).toBe('common.searchPlaceholder');
    });

    it('keeps public-share exports on the share endpoint with the funnel union', () => {
        const fixture = TestBed.createComponent(TrafficRecordsCard);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('from', '2026-07-01T00:00:00Z');
        fixture.componentRef.setInput('to', '2026-07-29T23:59:59Z');
        fixture.componentRef.setInput('funnelIds', ['funnel-1', 'funnel-2']);
        fixture.componentRef.setInput('shareToken', 'public token');

        const card = fixture.componentInstance as unknown as { exportUrl: () => string };
        expect(card.exportUrl()).toContain('/api/share/public%20token/sites/site-1/hits/export?');
        expect(card.exportUrl()).toContain('funnel_id=funnel-1&funnel_id=funnel-2');
    });
});
