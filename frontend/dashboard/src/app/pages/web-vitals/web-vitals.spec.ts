import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { By } from '@angular/platform-browser';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { TRANSLOCO_LOCALE_CONFIG, TRANSLOCO_LOCALE_LANG_MAPPING, TranslocoLocaleService } from '@jsverse/transloco-locale';
import { WebVitalsPage } from './web-vitals';
import { SiteService } from '@features/sites/services/site.service';
import { AnalyticsService } from '@core/services/analytics.service';

describe('WebVitalsPage', () => {
    let fixture: ComponentFixture<WebVitalsPage>;
    let analyticsServiceMock: {
        getWebVitalsSummary: ReturnType<typeof vi.fn>;
        getWebVitalsTimeseries: ReturnType<typeof vi.fn>;
        getWebVitalsPages: ReturnType<typeof vi.fn>;
        getWebVitalsBreakdown: ReturnType<typeof vi.fn>;
    };

    beforeEach(async () => {
        analyticsServiceMock = {
            getWebVitalsSummary: vi.fn(() =>
                of([
                    { metric: 'LCP', p75: 2400, samples: 120, good: 90, needs_improvement: 20, poor: 10, rating: 'good' },
                    { metric: 'INP', p75: 280, samples: 90, good: 50, needs_improvement: 30, poor: 10, rating: 'needs_improvement' },
                    { metric: 'CLS', p75: 0.09, samples: 88, good: 70, needs_improvement: 12, poor: 6, rating: 'good' }
                ])
            ),
            getWebVitalsTimeseries: vi.fn(() => of([{ time: '2026-05-06T00:00:00Z', p75: 2400, samples: 24, good: 18, needs_improvement: 4, poor: 2 }])),
            getWebVitalsPages: vi.fn(() =>
                of([
                    {
                        path: '/pricing',
                        p75: 3100,
                        samples: 24,
                        good: 12,
                        needs_improvement: 8,
                        poor: 4,
                        rating: 'needs_improvement',
                        metrics: {
                            LCP: { p75: 3100, samples: 24, good: 12, needs_improvement: 8, poor: 4, rating: 'needs_improvement' },
                            CLS: { p75: 0.08, samples: 24, good: 21, needs_improvement: 2, poor: 1, rating: 'good' }
                        }
                    }
                ])
            ),
            getWebVitalsBreakdown: vi.fn(() => of([{ name: 'Chrome', p75: 2800, samples: 40, good: 30, needs_improvement: 7, poor: 3, rating: 'needs_improvement' }]))
        };

        await TestBed.configureTestingModule({
            imports: [
                WebVitalsPage,
                NoopAnimationsModule,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { webVitals: 'Web Vitals' },
                            common: {
                                loadingSiteData: 'Loading site data...',
                                noSiteSelected: 'No site selected',
                                selectDateRange: 'Select date range',
                                searchPlaceholder: 'Search...',
                                columns: { actions: 'Actions' },
                                actions: { apply: 'Apply', cancel: 'Cancel', clearAll: 'Clear all' },
                                removeFilterAria: 'Remove filter',
                                timeRanges: {
                                    last24Hours: 'Last 24 hours',
                                    last7Days: 'Last 7 days',
                                    last30Days: 'Last 30 days',
                                    lastYear: 'Last year',
                                    customRange: 'Custom range'
                                },
                                empty: { noDataTitle: 'No data yet', noDataDescription: 'No data.' },
                                seriesChartAria: 'Series chart with {{count}} points'
                            },
                            webVitals: {
                                docsAction: 'Setup guide',
                                error: 'Web Vitals data could not be loaded.',
                                noSiteDescription: 'Select a site.',
                                metrics: { LCP: 'LCP', INP: 'INP', CLS: 'CLS', FCP: 'FCP', TTFB: 'TTFB' },
                                ratings: { good: 'Good', needs_improvement: 'Needs work', poor: 'Poor', none: 'No samples' },
                                cards: { samples: '{{count}} samples', selectMetricAria: 'Inspect {{metric}} over time', goodUntil: 'Good to {{value}}' },
                                filters: {
                                    metric: 'Metric',
                                    rating: 'Rating',
                                    path: 'Page path',
                                    allRatings: 'All ratings',
                                    ratingChip: 'Rating: {{rating}}',
                                    pathChip: 'Path: {{path}}'
                                },
                                chart: { title: '{{metric}} p75 trend', description: 'p75 over time', p75: '{{metric}} p75' },
                                distribution: { title: 'Rating mix', description: 'Site-wide {{metric}} samples.' },
                                pages: {
                                    title: 'Page breakdown',
                                    description: 'All paths with {{metric}} samples.',
                                    filteredTitle: '{{rating}} {{metric}} pages',
                                    filteredDescription: 'Showing paths that produced {{rating}} {{metric}} samples.',
                                    empty: 'No Web Vitals samples match the current filters.'
                                },
                                breakdown: {
                                    title: '{{dimension}} breakdown',
                                    description: '{{metric}} by visitor context',
                                    empty: 'No breakdown rows match the current filters.',
                                    unknown: 'Unknown',
                                    tabs: { pages: 'Pages', countries: 'Countries', languages: 'Languages', browsers: 'Browsers', devices: 'Devices' }
                                },
                                columns: { path: 'Path', p75: 'p75', samples: 'Samples', ratingSamples: '{{rating}} samples', rating: 'Rating' },
                                empty: { title: 'No Web Vitals yet', description: 'Enable Web Vitals.' }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    }
                })
            ],
            providers: [
                provideRouter([]),
                {
                    provide: SiteService,
                    useValue: {
                        activeSite: signal({
                            id: 'site-1',
                            user_id: 'user-1',
                            domain: 'example.test',
                            created_at: '2026-05-01T00:00:00Z'
                        }),
                        isLoading: signal(false)
                    }
                },
                { provide: AnalyticsService, useValue: analyticsServiceMock },
                {
                    provide: TranslocoLocaleService,
                    useValue: {
                        langChanges$: of('en'),
                        localeChanges$: of('en'),
                        getLocale: () => 'en-US',
                        localizeNumber: (value: number) => value.toString(),
                        localizeDate: (value: Date) => value.toISOString()
                    }
                },
                { provide: TRANSLOCO_LOCALE_CONFIG, useValue: {} },
                { provide: TRANSLOCO_LOCALE_LANG_MAPPING, useValue: { en: 'en-US' } }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(WebVitalsPage);
        fixture.detectChanges();
    });

    it('renders populated Web Vitals report sections', () => {
        const text = fixture.nativeElement.textContent;
        expect(text).toContain('LCP p75 trend');
        expect(text).toContain('Rating mix');
        expect(text).toContain('Page breakdown');
        expect(text).toContain('/pricing');
        const timeseriesCall = analyticsServiceMock.getWebVitalsTimeseries.mock.calls[0];
        expect(timeseriesCall[0]).toBe('site-1');
        expect(timeseriesCall[3]).toBe('LCP');
        expect(timeseriesCall[4]).toBe('/');
        expect(timeseriesCall[5]).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Path: /');
    });

    it('keeps breakdown tables searchable, sortable, and paginated', () => {
        const searches = Array.from<HTMLInputElement>(fixture.nativeElement.querySelectorAll('input[placeholder="Search..."]'));
        const tables = fixture.debugElement.queryAll(By.css('p-table'));
        const sortIcons = Array.from<HTMLElement>(fixture.nativeElement.querySelectorAll('p-sorticon'));

        expect(searches.length).toBeGreaterThanOrEqual(2);
        expect(tables.length).toBeGreaterThanOrEqual(2);
        expect(tables.every((table) => table.componentInstance.paginator === true)).toBe(true);
        expect(sortIcons.length).toBeGreaterThanOrEqual(11);
    });

    it('keeps metric cards site-wide while passing rating and path filters to drilldowns', () => {
        const component = fixture.componentInstance as unknown as {
            selectedRating: { set: (value: string) => void };
            selectPathFilter: (value: string) => void;
        };
        component.selectedRating.set('poor');
        component.selectPathFilter('/checkout');
        fixture.detectChanges();

        const summaryCall = analyticsServiceMock.getWebVitalsSummary.mock.calls.at(-1);
        const pagesCall = analyticsServiceMock.getWebVitalsPages.mock.calls.at(-1);
        expect(summaryCall?.[0]).toBe('site-1');
        expect(summaryCall?.[3]).toBeNull();
        expect(summaryCall?.[4]).toBeNull();
        expect(summaryCall?.[5]).toBeNull();
        expect(pagesCall?.[0]).toBe('site-1');
        expect(pagesCall?.[3]).toBe('LCP');
        expect(pagesCall?.[4]).toBeNull();
        expect(pagesCall?.[5]).toBe('poor');
        expect(pagesCall?.[6]).toBe(100);
        expect(fixture.nativeElement.textContent).toContain('Poor LCP pages');
        expect(fixture.nativeElement.textContent).toContain('Poor samples');
        expect(fixture.nativeElement.textContent).toContain('4');
    });

    it('resets optional filters back to the homepage Web Vitals scope', () => {
        const component = fixture.componentInstance as unknown as {
            selectedRating: { set: (value: string) => void };
            selectPathFilter: (value: string) => void;
            clearAllFilters: () => void;
        };
        component.selectedRating.set('poor');
        component.selectPathFilter('/pricing?campaign=spring#plans');
        component.clearAllFilters();
        fixture.detectChanges();

        const summaryCall = analyticsServiceMock.getWebVitalsSummary.mock.calls.at(-1);
        const pagesCall = analyticsServiceMock.getWebVitalsPages.mock.calls.at(-1);
        expect(summaryCall?.[4]).toBeNull();
        expect(summaryCall?.[5]).toBeNull();
        expect(pagesCall?.[4]).toBeNull();
        expect(pagesCall?.[5]).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Path: /');
    });

    it('renders an error state when the report fails', () => {
        analyticsServiceMock.getWebVitalsSummary.mockReturnValue(throwError(() => new Error('nope')));
        fixture = TestBed.createComponent(WebVitalsPage);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Web Vitals data could not be loaded.');
    });
});
