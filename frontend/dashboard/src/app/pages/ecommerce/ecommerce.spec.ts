import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { signal } from '@angular/core';
import { of } from 'rxjs';
import { vi } from 'vitest';
import { By } from '@angular/platform-browser';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { TRANSLOCO_LOCALE_CONFIG, TRANSLOCO_LOCALE_LANG_MAPPING, TranslocoLocaleService } from '@jsverse/transloco-locale';
import { EcommercePage } from './ecommerce';
import { SiteService } from '@features/sites/services/site.service';
import { AnalyticsService } from '@core/services/analytics.service';
import { RealtimeRefreshCoordinator } from '@services/realtime-refresh-coordinator.service';
import type { SiteSetupState } from '@services/setup-state.service';
import { flushSetupState } from '@testing/setup-state';

type EcommercePageTestAccess = EcommercePage & {
    activeFilters(): { type: string; value: string }[];
    selectedProduct(): { itemId: string; itemName: string } | null;
};

describe('EcommercePage', () => {
    let fixture: ComponentFixture<EcommercePage>;
    let httpMock: HttpTestingController;
    let analyticsServiceMock: {
        getEcommerceSummary: ReturnType<typeof vi.fn>;
        getEcommerceTimeseries: ReturnType<typeof vi.fn>;
        getEcommerceProducts: ReturnType<typeof vi.fn>;
        getEcommerceSources: ReturnType<typeof vi.fn>;
        getSiteStats: ReturnType<typeof vi.fn>;
    };
    let realtimeRefresh: (() => void) | undefined;

    const clickTab = (label: string): void => {
        const tab = Array.from<HTMLElement>(fixture.nativeElement.querySelectorAll('p-tab')).find((element) => element.textContent?.includes(label));
        expect(tab).toBeTruthy();
        tab?.click();
        fixture.detectChanges();
    };

    beforeEach(async () => {
        realtimeRefresh = undefined;
        analyticsServiceMock = {
            getEcommerceSummary: vi.fn(() =>
                of({
                    revenue: 180,
                    orders: 2,
                    average_order_value: 90,
                    checkout_starts: 3,
                    checkout_conversion_rate: 66.7,
                    currency: 'USD',
                    top_cities: [{ name: 'Berlin', value: 2 }],
                    top_providers: [{ name: 'Hetzner Online GmbH', value: 2 }],
                    top_asns: [{ name: 'AS24940 Hetzner Online GmbH', value: 2 }]
                })
            ),
            getEcommerceTimeseries: vi.fn(() =>
                of([
                    { time: '2026-03-07T00:00:00Z', revenue: 120, orders: 1 },
                    { time: '2026-03-08T00:00:00Z', revenue: 60, orders: 1 }
                ])
            ),
            getEcommerceProducts: vi.fn(() =>
                of([
                    {
                        item_id: 'pro',
                        item_name: 'Pro',
                        revenue: 120,
                        orders: 1,
                        quantity: 1
                    },
                    {
                        item_id: 'starter',
                        item_name: 'Starter',
                        revenue: 60,
                        orders: 1,
                        quantity: 2
                    }
                ])
            ),
            getEcommerceSources: vi.fn(() =>
                of([
                    {
                        utm_source: 'google.com',
                        utm_medium: 'cpc',
                        utm_campaign: 'launch',
                        referrer: 'https://google.com/search',
                        revenue: 120,
                        orders: 1
                    }
                ])
            ),
            getSiteStats: vi.fn(() =>
                of({
                    live_visitors: 0,
                    total_pageviews: 0,
                    unique_sessions: 0,
                    bounce_rate: 0,
                    avg_session_duration: 0,
                    pages_per_session: 0,
                    chart_data: [],
                    top_pages: [],
                    top_landing_pages: [],
                    top_exit_pages: [],
                    top_referrers: [{ name: 'https://google.com/search', value: 1 }],
                    top_devices: [{ name: 'Desktop', value: 1 }],
                    top_countries: [{ name: 'United States', value: 1 }],
                    top_cities: [{ name: 'Mountain View', value: 99 }],
                    top_providers: [{ name: 'Google LLC', value: 99 }],
                    top_asns: [{ name: 'AS15169 Google LLC', value: 99 }],
                    top_languages: [],
                    top_utm_campaigns: [],
                    top_utm_contents: [],
                    top_utm_mediums: [],
                    top_utm_sources: [{ name: 'google.com', value: 1 }],
                    top_utm_terms: [],
                    utm_campaign_hits: 0,
                    utm_content_hits: 0,
                    utm_medium_hits: 0,
                    utm_source_hits: 0,
                    utm_term_hits: 0,
                    goals: [],
                    funnels: []
                })
            )
        };

        await TestBed.configureTestingModule({
            imports: [
                EcommercePage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { ecommerce: 'Ecommerce' },
                            common: {
                                loadingSiteData: 'Loading site data...',
                                noSiteSelected: 'No site selected',
                                noActiveFilter: 'No active filter',
                                removeFilterAria: 'Remove filter',
                                selectDateRange: 'Select date range',
                                searchPlaceholder: 'Search...',
                                actions: {
                                    apply: 'Apply',
                                    cancel: 'Cancel',
                                    clearAll: 'Clear all',
                                    more: 'More actions'
                                },
                                columns: { actions: 'Actions' },
                                timeRanges: {
                                    today: 'Today',
                                    todayShort: 'Today',
                                    yesterday: 'Yesterday',
                                    yesterdayShort: 'Yesterday',
                                    lastMinute: 'Last minute',
                                    lastMinutes: 'Last {{count}} minutes',
                                    lastHour: 'Last hour',
                                    lastHours: 'Last {{count}} hours',
                                    lastDay: 'Last day',
                                    lastDays: 'Last {{count}} days',
                                    lastYear: 'Last year',
                                    lastYearShort: '1y',
                                    moreRanges: 'More ranges',
                                    customRange: 'Custom range',
                                    customShort: 'Custom',
                                    customRangeSummary: '{{start}} - {{end}}'
                                }
                            },
                            ecommerce: {
                                kpis: {
                                    revenue: 'Revenue',
                                    orders: 'Orders',
                                    averageOrderValue: 'Avg. order value',
                                    checkoutConversion: 'Checkout conversion'
                                },
                                chart: {
                                    title: 'Revenue over time',
                                    description: 'Track revenue and completed purchases for the active date range.',
                                    revenue: 'Revenue',
                                    orders: 'Orders'
                                },
                                filtersPanels: {
                                    sources: 'Top sources',
                                    referrers: 'Top referrers',
                                    devices: 'Top devices',
                                    countries: 'Top countries'
                                },
                                filters: {
                                    product: 'Product: {{value}}',
                                    utmSource: 'UTM source: {{value}}'
                                },
                                breakdowns: {
                                    title: 'Revenue breakdown',
                                    description: 'Compare product performance and revenue attribution in one place.'
                                },
                                products: {
                                    title: 'Top products',
                                    description: 'See which products drive the most revenue and completed orders.',
                                    empty: 'No purchase items matched the current filters.'
                                },
                                sources: {
                                    title: 'Revenue sources',
                                    description: 'Attribute revenue to source, medium, campaign, and referrer.',
                                    empty: 'No revenue sources matched the current filters.'
                                },
                                columns: {
                                    product: 'Product',
                                    quantity: 'Quantity',
                                    orders: 'Orders',
                                    revenue: 'Revenue',
                                    source: 'Source',
                                    campaign: 'Campaign',
                                    referrer: 'Referrer'
                                },
                                actions: {
                                    filterProduct: 'Filter',
                                    clearProductFilter: 'Clear',
                                    filterSource: 'Filter',
                                    clearSourceFilter: 'Clear'
                                },
                                empty: {
                                    title: 'No ecommerce data yet',
                                    description: 'Track purchase events.'
                                },
                                noSiteDescription: 'Select a site to view ecommerce analytics.',
                                docsAction: 'Set up ecommerce',
                                setup: {
                                    title: 'Send purchase events',
                                    description: 'Track ecommerce events to fill this page.',
                                    docsAction: 'Read the ecommerce guide'
                                }
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
                provideHttpClient(),
                provideHttpClientTesting(),
                provideRouter([]),
                {
                    provide: SiteService,
                    useValue: {
                        activeSite: signal({
                            id: 'site-1',
                            user_id: 'user-1',
                            domain: 'shop.test',
                            created_at: '2026-03-08T00:00:00Z'
                        }),
                        isLoading: signal(false)
                    }
                },
                {
                    provide: AnalyticsService,
                    useValue: analyticsServiceMock
                },
                {
                    provide: RealtimeRefreshCoordinator,
                    useValue: {
                        registerUntilDestroyed: vi.fn((_destroyRef: unknown, options: { refresh: () => void }) => {
                            realtimeRefresh = options.refresh;
                        })
                    }
                },
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
                {
                    provide: TRANSLOCO_LOCALE_CONFIG,
                    useValue: {}
                },
                {
                    provide: TRANSLOCO_LOCALE_LANG_MAPPING,
                    useValue: { en: 'en-US' }
                }
            ]
        }).compileComponents();

        httpMock = TestBed.inject(HttpTestingController);
        fixture = TestBed.createComponent(EcommercePage);
        fixture.detectChanges();
    });

    afterEach(() => {
        // Drain the shared setup-state lookup for the tests that do not assert on it.
        flushSetupState(httpMock, 'site-1');
        httpMock.verify();
    });

    /** Recreates the page on a range without a single ecommerce event. */
    const createWithoutRevenue = (): void => {
        analyticsServiceMock.getEcommerceSummary.mockReturnValue(of({ revenue: 0, orders: 0, average_order_value: 0, checkout_starts: 0, checkout_conversion_rate: 0, currency: 'USD' }));
        analyticsServiceMock.getEcommerceProducts.mockReturnValue(of([]));
        analyticsServiceMock.getEcommerceSources.mockReturnValue(of([]));
        analyticsServiceMock.getEcommerceTimeseries.mockReturnValue(of([]));
        fixture = TestBed.createComponent(EcommercePage);
        fixture.detectChanges();
    };

    const answerSetupState = (overrides: Partial<SiteSetupState> = {}) => flushSetupState(httpMock, 'site-1', overrides, fixture);

    it('renders a setup callout when the site never sent an ecommerce event', () => {
        createWithoutRevenue();
        answerSetupState({ has_ecommerce_events: false });

        const callout = fixture.nativeElement.querySelector('[data-testid="ecommerce-setup-callout"]');
        expect(callout).not.toBeNull();
        expect(callout.textContent).toContain('Send purchase events');
        expect(callout.textContent).toContain('Track ecommerce events to fill this page.');
        expect(callout.querySelector('a[href="https://hitkeep.com/guides/analytics/ecommerce/"]')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('app-kpi-card')).toBeNull();
        expect(fixture.nativeElement.querySelector('app-report-range-toolbar')).not.toBeNull();
    });

    it('keeps the regular empty states when ecommerce events exist outside the selected range', () => {
        createWithoutRevenue();
        answerSetupState({ has_ecommerce_events: true });

        expect(fixture.nativeElement.querySelector('[data-testid="ecommerce-setup-callout"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('app-kpi-card')).not.toBeNull();
    });

    it('never shows the callout while the setup state is still unknown', () => {
        createWithoutRevenue();

        httpMock.expectOne('/api/sites/site-1/setup-state');
        expect(fixture.nativeElement.querySelector('[data-testid="ecommerce-setup-callout"]')).toBeNull();
    });

    it('skips the setup-state lookup while the selected range has revenue', () => {
        httpMock.expectNone('/api/sites/site-1/setup-state');
    });

    it('renders revenue-focused ecommerce analytics', () => {
        const text = fixture.nativeElement.textContent;
        expect(text).toContain('Revenue over time');
        expect(text).toContain('Revenue breakdown');
        expect(text).toContain('Top products');
        expect(text).toContain('Revenue sources');
        expect(text).toContain('Pro');
    });

    it('keeps currency and conversion KPIs numeric with locale-aware formatting metadata', () => {
        const cards = (
            fixture.componentInstance as unknown as {
                kpiCards: () => {
                    value: string | number;
                    format?: Intl.NumberFormatOptions;
                    suffix?: string;
                }[];
            }
        ).kpiCards();

        expect(cards.map((card) => card.value)).toEqual([180, 2, 90, 66.7]);
        expect(cards[0]?.format).toEqual({
            style: 'currency',
            currency: 'USD',
            maximumFractionDigits: 2
        });
        expect(cards[2]?.format).toEqual({
            style: 'currency',
            currency: 'USD',
            maximumFractionDigits: 2
        });
        expect(cards[3]?.format).toEqual({
            minimumFractionDigits: 1,
            maximumFractionDigits: 1
        });
        expect(cards[3]?.suffix).toBe('%');
    });

    it('commits custom-loader realtime results in the background and advances the KPI update key', () => {
        analyticsServiceMock.getEcommerceSummary.mockReturnValue(
            of({
                revenue: 450,
                orders: 5,
                average_order_value: 90,
                checkout_starts: 6,
                checkout_conversion_rate: 83.3,
                currency: 'USD',
                top_cities: [],
                top_providers: [],
                top_asns: []
            })
        );

        expect(typeof realtimeRefresh).toBe('function');
        realtimeRefresh?.();
        fixture.detectChanges();

        const component = fixture.componentInstance as unknown as {
            isLoading: () => boolean;
            kpiCards: () => { value: string | number; updateKey?: number }[];
        };
        const cards = component.kpiCards();

        expect(component.isLoading()).toBe(false);
        expect(cards.map((card) => card.value)).toEqual([450, 5, 90, 83.3]);
        expect(cards.every((card) => card.updateKey === 1)).toBe(true);
        expect(fixture.nativeElement.querySelectorAll('app-kpi-card p-skeleton').length).toBe(0);
    });

    it('keeps revenue tables searchable, sortable, and paginated', () => {
        const searches = Array.from<HTMLInputElement>(fixture.nativeElement.querySelectorAll('input[placeholder="Search..."]'));
        const tables = fixture.debugElement.queryAll(By.css('p-table'));
        const sortIcons = Array.from<HTMLElement>(fixture.nativeElement.querySelectorAll('p-sorticon'));

        expect(searches.length).toBeGreaterThanOrEqual(2);
        expect(tables.length).toBeGreaterThanOrEqual(2);
        expect(tables.every((table) => table.componentInstance.paginator === true)).toBe(true);
        expect(sortIcons.length).toBeGreaterThanOrEqual(9);
    });

    it('renders source and referrer URLs with favicons and hover links', () => {
        const component = fixture.componentInstance as unknown as {
            metricCardTabs: () => {
                id: string;
                cards: { id: string; linkMode?: string }[];
            }[];
        };
        const acquisitionCards = component.metricCardTabs().find((tab) => tab.id === 'acquisition')?.cards ?? [];
        expect(acquisitionCards.find((card) => card.id === 'utm-sources')?.linkMode).toBeUndefined();
        expect(acquisitionCards.find((card) => card.id === 'referrers')?.linkMode).toBe('url');

        clickTab('Revenue sources');

        const links = Array.from<HTMLAnchorElement>(fixture.nativeElement.querySelectorAll('.ecommerce-source-cell__link'));
        const favicons = Array.from<HTMLImageElement>(fixture.nativeElement.querySelectorAll('.ecommerce-source-cell__favicon'));

        expect(links.map((link) => link.href)).toContain('https://google.com/search');
        expect(links.map((link) => link.href)).not.toContain('https://google.com/');
        expect(favicons.length).toBeGreaterThanOrEqual(1);
        expect(favicons.some((img) => (img.getAttribute('ngsrc') ?? img.getAttribute('src') ?? '').includes('/api/favicon/google.com'))).toBe(true);
    });

    it('toggles product filters from table rows like metric cards', () => {
        const component = fixture.componentInstance as EcommercePageTestAccess;
        const productRow = Array.from<HTMLElement>(fixture.nativeElement.querySelectorAll('tr.ecommerce-filter-row')).find((row) => row.textContent?.includes('Pro'));
        expect(productRow).toBeTruthy();

        productRow?.click();
        fixture.detectChanges();

        expect(component.selectedProduct()).toEqual({
            itemId: 'pro',
            itemName: 'Pro'
        });
        expect(fixture.nativeElement.textContent).toContain('Product: Pro');
        expect(fixture.nativeElement.querySelector('tr.ecommerce-filter-row--active')?.textContent).toContain('Pro');

        fixture.nativeElement.querySelector('tr.ecommerce-filter-row--active')?.click();
        fixture.detectChanges();

        expect(component.selectedProduct()).toBeNull();
    });

    it('toggles revenue source filters from table rows like metric cards', () => {
        const component = fixture.componentInstance as EcommercePageTestAccess;

        clickTab('Revenue sources');
        const sourceRow = Array.from<HTMLElement>(fixture.nativeElement.querySelectorAll('tr.ecommerce-filter-row')).find((row) => row.textContent?.includes('google'));
        expect(sourceRow).toBeTruthy();

        sourceRow?.click();
        fixture.detectChanges();

        expect(component.activeFilters()).toEqual([{ type: 'utm_source', value: 'google.com' }]);
        expect(fixture.nativeElement.textContent).toContain('UTM source: google.com');
        expect(fixture.nativeElement.querySelector('tr.ecommerce-filter-row--active')?.textContent).toContain('google.com');
    });

    it('renders ecommerce-specific geo and network aggregates', () => {
        expect(fixture.nativeElement.textContent).toContain('Hetzner Online GmbH');
        expect(fixture.nativeElement.textContent).not.toContain('Google LLC');

        clickTab('common.metrics.cities');
        expect(fixture.nativeElement.textContent).toContain('Berlin');
        expect(fixture.nativeElement.textContent).not.toContain('Mountain View');

        clickTab('common.metrics.asns');
        expect(fixture.nativeElement.textContent).toContain('AS24940 Hetzner Online GmbH');
        expect(fixture.nativeElement.textContent).not.toContain('AS15169 Google LLC');
    });

    it('does not crash when ecommerce currency is unspecified', () => {
        analyticsServiceMock.getEcommerceSummary.mockReturnValue(
            of({
                revenue: 0,
                orders: 0,
                average_order_value: 0,
                checkout_starts: 0,
                checkout_conversion_rate: 0,
                currency: '(Unspecified)'
            })
        );
        analyticsServiceMock.getEcommerceTimeseries.mockReturnValue(of([]));
        analyticsServiceMock.getEcommerceProducts.mockReturnValue(of([]));
        analyticsServiceMock.getEcommerceSources.mockReturnValue(of([]));

        fixture = TestBed.createComponent(EcommercePage);
        expect(() => fixture.detectChanges()).not.toThrow();

        const text = fixture.nativeElement.textContent;
        expect(text).toContain('Revenue');
        expect(text).toContain('Avg. order value');
        expect(text).toContain('No ecommerce data yet');

        const cards = (
            fixture.componentInstance as unknown as {
                kpiCards: () => {
                    value: string | number;
                    format?: Intl.NumberFormatOptions;
                }[];
            }
        ).kpiCards();
        expect(cards[0]?.value).toBe(0);
        expect(cards[0]?.format).toEqual({
            minimumFractionDigits: 2,
            maximumFractionDigits: 2
        });
    });
});
