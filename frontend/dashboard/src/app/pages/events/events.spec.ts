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
                                    countries: 'Countries'
                                },
                                filters: {
                                    page: 'Page: {{value}}',
                                    source: 'Source: {{value}}',
                                    device: 'Device: {{value}}',
                                    country: 'Country: {{value}}'
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
});
