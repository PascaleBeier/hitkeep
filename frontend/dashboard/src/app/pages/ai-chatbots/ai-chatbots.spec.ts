import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { AnalyticsService } from '@core/services/analytics.service';
import { TakeoutDownloadService } from '@services/takeout-download.service';
import { SiteService } from '@features/sites/services/site.service';
import { AIChatbots } from '@pages/ai-chatbots/ai-chatbots';

describe('AIChatbots', () => {
    let component: AIChatbots;
    let fixture: ComponentFixture<AIChatbots>;
    const analyticsServiceStub = {
        getEventTimeseries: vi.fn(() => of([])),
        getEventPropertyBreakdown: vi.fn(() => of([])),
        getEventAudience: vi.fn(() => of(null))
    };

    beforeEach(async () => {
        analyticsServiceStub.getEventTimeseries.mockClear();
        analyticsServiceStub.getEventPropertyBreakdown.mockClear();
        analyticsServiceStub.getEventAudience.mockClear();

        await TestBed.configureTestingModule({
            imports: [
                AIChatbots,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            aiChatbots: {
                                title: 'AI Chatbots',
                                filters: {
                                    provider: 'Provider',
                                    botId: 'Bot ID',
                                    surface: 'Surface',
                                    model: 'Model'
                                },
                                kpis: {
                                    conversations: 'Conversations',
                                    prompts: 'Prompts',
                                    responses: 'Responses',
                                    assistedConversions: 'Assisted conversions',
                                    handoffRate: 'Handoff rate',
                                    citationCtr: 'Citation CTR'
                                },
                                breakdowns: {
                                    intents: 'Intents',
                                    providers: 'Chatbot providers',
                                    surfaces: 'Surfaces'
                                }
                            },
                            common: {
                                metricGroups: {
                                    content: 'Content',
                                    acquisition: 'Acquisition',
                                    audience: 'Audience',
                                    location: 'Location',
                                    network: 'Network'
                                },
                                metrics: {
                                    topPages: 'Top pages',
                                    topSources: 'Top sources',
                                    devices: 'Devices',
                                    countries: 'Countries',
                                    cities: 'Cities',
                                    providers: 'Network providers',
                                    asns: 'ASNs'
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
                provideRouter([]),
                {
                    provide: SiteService,
                    useValue: {
                        activeSite: signal({
                            id: 'site-1',
                            user_id: 'user-1',
                            domain: 'assistant.test',
                            created_at: '2026-05-01T00:00:00Z'
                        }),
                        isLoading: signal(false)
                    }
                },
                { provide: AnalyticsService, useValue: analyticsServiceStub },
                {
                    provide: TakeoutDownloadService,
                    useValue: {
                        downloadFromUrl: vi.fn(() => of(undefined))
                    }
                },
                provideTranslocoLocale({
                    defaultLocale: 'en-US',
                    langToLocaleMapping: {
                        en: 'en-US',
                        'en-US': 'en-US'
                    }
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(AIChatbots);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('keeps chatbot provider cards separate from network provider cards', () => {
        const chatbots = component as unknown as {
            topIntents: { set: (value: { name: string; value: number }[]) => void };
            topProviders: { set: (value: { name: string; value: number }[]) => void };
            topSurfaces: { set: (value: { name: string; value: number }[]) => void };
            audience: { set: (value: unknown) => void };
            metricCardTabs: () => { id: string; cards: { id: string; title: string; data: { name: string; value: number }[] }[] }[];
        };

        chatbots.topIntents.set([{ name: 'pricing', value: 6 }]);
        chatbots.topProviders.set([{ name: 'OpenAI', value: 5 }]);
        chatbots.topSurfaces.set([{ name: 'docs-assistant', value: 4 }]);
        chatbots.audience.set({
            top_pages: [{ name: '/docs', value: 4 }],
            top_referrers: [{ name: 'https://example.com', value: 3 }],
            top_devices: [{ name: 'Desktop', value: 3 }],
            top_countries: [{ name: 'US', value: 3 }],
            top_cities: [{ name: 'Mountain View', value: 2 }],
            top_providers: [{ name: 'Google LLC', value: 2 }],
            top_asns: [{ name: 'AS15169 Google LLC', value: 2 }]
        });

        const tabs = chatbots.metricCardTabs();
        const contentCards = tabs.find((tab) => tab.id === 'content')?.cards ?? [];
        const networkCards = tabs.find((tab) => tab.id === 'network')?.cards ?? [];
        const chatbotProviderCard = contentCards.find((card) => card.id === 'chatbot-providers');
        const networkProviderCard = networkCards.find((card) => card.id === 'network-providers');

        expect(chatbotProviderCard?.title).toBe('Chatbot providers');
        expect(chatbotProviderCard?.data).toEqual([{ name: 'OpenAI', value: 5 }]);
        expect(networkProviderCard?.title).toBe('Network providers');
        expect(networkProviderCard?.data).toEqual([{ name: 'Google LLC', value: 2 }]);
        expect(networkCards.map((card) => card.id)).toEqual(['network-providers', 'asns']);
    });
});
