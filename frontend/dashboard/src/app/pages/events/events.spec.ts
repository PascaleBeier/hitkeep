import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideRouter } from '@angular/router';
import { signal } from '@angular/core';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of } from 'rxjs';

import { Events } from '@pages/events/events';
import { AnalyticsService } from '@core/services/analytics.service';
import { SiteService } from '@features/sites/services/site.service';

describe('Events', () => {
    let component: Events;
    let fixture: ComponentFixture<Events>;
    const siteServiceStub = {
        activeSite: signal({
            id: 'site-1',
            user_id: 'user-1',
            domain: 'acme-analytics.io',
            created_at: new Date().toISOString()
        })
    };
    const analyticsServiceStub = {
        getEventNames: () => of(['outbound_click', 'newsletter_signup']),
        getEventPropertyKeys: () => of(['target_host']),
        getEventPropertyBreakdown: () => of([{ name: 'external.example.com', value: 12 }]),
        getEventTimeseries: () => of([{ time: new Date().toISOString(), count: 12 }]),
        getEventAudience: () =>
            of({
                top_pages: [{ name: '/pricing', value: 8 }],
                top_referrers: [{ name: 'https://google.com', value: 5 }],
                top_devices: [{ name: 'Desktop', value: 7 }],
                top_countries: [{ name: 'US', value: 4 }]
            })
    };

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                Events,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            events: {
                                title: 'Events',
                                docsAction: 'Auto-tracking guide',
                                noSiteDescription: 'Select or create a site to view event analytics.',
                                eventNameLabel: 'Event',
                                eventNamePlaceholder: 'Select an event',
                                propertyKeyLabel: 'Break down by',
                                propertyKeyPlaceholder: 'Select a property',
                                automatic: {
                                    badge: 'Auto'
                                },
                                series: {
                                    title: 'Event activity',
                                    description: 'Event occurrences over the selected period.',
                                    emptyTitle: 'No event data',
                                    emptyDescription: 'Select an event to view activity over time.'
                                },
                                breakdown: {
                                    title: 'Property breakdown',
                                    selectEventFirst: 'Select an event to view a property breakdown.',
                                    selectPropertyFirst: 'Select a property to view the breakdown.'
                                },
                                kpis: {
                                    totalEvents: 'Total events'
                                }
                            },
                            dashboard: {
                                filteredBadge: 'Filtered'
                            },
                            common: {
                                noSiteSelected: 'No site selected',
                                noActiveFilter: 'No active filter',
                                removeFilterAria: 'Remove filter',
                                actions: {
                                    clearAll: 'Clear all'
                                },
                                metrics: {
                                    topPages: 'Top Pages',
                                    topSources: 'Top Sources',
                                    devices: 'Devices',
                                    countries: 'Countries',
                                    cities: 'Cities',
                                    providers: 'Providers',
                                    asns: 'ASNs'
                                },
                                metricGroups: {
                                    content: 'Content',
                                    acquisition: 'Acquisition',
                                    audience: 'Audience',
                                    location: 'Location',
                                    network: 'Network'
                                },
                                filters: {
                                    page: 'Page: {{value}}',
                                    source: 'Source: {{value}}',
                                    device: 'Device: {{value}}',
                                    country: 'Country: {{value}}',
                                    city: 'City: {{value}}',
                                    provider: 'Provider: {{value}}',
                                    asn: 'ASN: {{value}}'
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
                provideHttpClient(),
                provideRouter([]),
                { provide: SiteService, useValue: siteServiceStub },
                { provide: AnalyticsService, useValue: analyticsServiceStub },
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US',
                        'en-US': 'en-US'
                    }
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(Events);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('marks automatic events in the event dropdown', async () => {
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const options = (component as unknown as { eventOptions: () => { value: string; isAutomatic: boolean; icon: string }[] }).eventOptions();
        const outboundOption = options.find((option) => option.value === 'outbound_click');

        expect(outboundOption?.isAutomatic).toBeTruthy();
        expect(outboundOption?.icon).toBe('pi pi-external-link');
    });

    it('keeps automatic events available even without data in the selected range', async () => {
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();

        const optionValues = (component as unknown as { eventOptions: () => { value: string }[] }).eventOptions().map((option) => option.value);

        expect(optionValues).toContain('outbound_click');
        expect(optionValues).toContain('file_download');
        expect(optionValues).toContain('form_submit');
        expect(optionValues).toContain('newsletter_signup');
    });

    it('keeps multiple audience dimension filters active together', () => {
        const events = component as unknown as {
            toggleAudienceDimFilter: (type: 'path' | 'referrer' | 'device' | 'country' | 'city' | 'provider' | 'asn', item: { name: string; value: number }) => void;
            audienceDimFilters: () => { type: string; value: string }[];
            activeDimensionFilterValue: (type: 'path' | 'referrer' | 'device' | 'country' | 'city' | 'provider' | 'asn') => string | null;
        };

        events.toggleAudienceDimFilter('path', { name: '/pricing', value: 8 });
        events.toggleAudienceDimFilter('device', { name: 'Desktop', value: 7 });

        expect(events.audienceDimFilters()).toEqual([
            { type: 'path', value: '/pricing' },
            { type: 'device', value: 'Desktop' }
        ]);
        expect(events.activeDimensionFilterValue('path')).toBe('/pricing');
        expect(events.activeDimensionFilterValue('device')).toBe('Desktop');
    });

    it('replaces a filter value for the same audience dimension', () => {
        const events = component as unknown as {
            toggleAudienceDimFilter: (type: 'path' | 'referrer' | 'device' | 'country' | 'city' | 'provider' | 'asn', item: { name: string; value: number }) => void;
            audienceDimFilters: () => { type: string; value: string }[];
        };

        events.toggleAudienceDimFilter('path', { name: '/pricing', value: 8 });
        events.toggleAudienceDimFilter('path', { name: '/docs', value: 4 });

        expect(events.audienceDimFilters()).toEqual([{ type: 'path', value: '/docs' }]);
    });

    it('groups event property and audience cards into the shared metric card surface', () => {
        const events = component as unknown as {
            audience: { set: (value: unknown) => void };
            selectedEvent: { set: (value: string) => void };
            selectedPropertyKey: { set: (value: string) => void };
            breakdown: { set: (value: { name: string; value: number }[]) => void };
            metricCardTabs: () => { id: string; cards: { id: string; activeValue?: string | null; filterType?: string; data: { name: string; value: number }[] }[] }[];
            onMetricCardClick: (event: { tabId: string; cardId: string; filterType: string; metric: { name: string; value: number } }) => void;
            audienceDimFilters: () => { type: string; value: string }[];
            selectedPropertyValue: () => string | null;
            activeDimensionFilterValue: (type: 'provider') => string | null;
        };

        events.selectedEvent.set('outbound_click');
        events.selectedPropertyKey.set('target_host');
        events.breakdown.set([{ name: 'external.example.com', value: 12 }]);
        events.audience.set({
            top_pages: [{ name: '/pricing', value: 8 }],
            top_referrers: [{ name: 'https://google.com', value: 5 }],
            top_devices: [{ name: 'Desktop', value: 7 }],
            top_countries: [{ name: 'US', value: 4 }],
            top_cities: [{ name: 'Berlin', value: 3 }],
            top_providers: [{ name: 'Hetzner Online GmbH', value: 2 }],
            top_asns: [{ name: 'AS24940 Hetzner Online GmbH', value: 2 }]
        });

        const tabs = events.metricCardTabs();

        expect(tabs.map((tab) => tab.id)).toEqual(['content', 'acquisition', 'audience', 'location', 'network']);
        expect(tabs.find((tab) => tab.id === 'content')?.cards.map((card) => card.id)).toEqual(['property-breakdown', 'top-pages']);
        expect(tabs.find((tab) => tab.id === 'content')?.cards[0]?.filterType).toBe('propertyValue');
        expect(tabs.find((tab) => tab.id === 'location')?.cards.map((card) => card.id)).toEqual(['countries', 'cities']);
        expect(tabs.find((tab) => tab.id === 'network')?.cards.map((card) => card.id)).toEqual(['providers', 'asns']);

        events.onMetricCardClick({
            tabId: 'content',
            cardId: 'property-breakdown',
            filterType: 'propertyValue',
            metric: { name: 'external.example.com', value: 12 }
        });

        expect(events.selectedPropertyValue()).toBe('external.example.com');

        events.onMetricCardClick({
            tabId: 'network',
            cardId: 'providers',
            filterType: 'provider',
            metric: { name: 'Hetzner Online GmbH', value: 2 }
        });

        expect(events.audienceDimFilters()).toEqual([{ type: 'provider', value: 'Hetzner Online GmbH' }]);
        expect(events.activeDimensionFilterValue('provider')).toBe('Hetzner Online GmbH');
    });
});
