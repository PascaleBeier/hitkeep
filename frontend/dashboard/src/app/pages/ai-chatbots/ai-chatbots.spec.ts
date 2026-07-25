import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { signal, WritableSignal } from '@angular/core';
import { By } from '@angular/platform-browser';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { Observable, of, Subject } from 'rxjs';
import { vi } from 'vitest';

import { AnalyticsService } from '@core/services/analytics.service';
import { ReportRangeToolbar } from '@components/report-range-toolbar/report-range-toolbar';
import { TakeoutDownloadService } from '@services/takeout-download.service';
import { SiteService } from '@features/sites/services/site.service';
import type { EventSeriesPoint } from '@models/analytics.types';
import { AIChatbots } from '@pages/ai-chatbots/ai-chatbots';
import { ReportRangePreferencesService } from '@services/report-range-preferences.service';
import type { SiteSetupState } from '@services/setup-state.service';
import { flushSetupState } from '@testing/setup-state';

const CHAT_STARTED_EVENT = 'assistant.chat_started';
const SETUP_STATE_URL = '/api/sites/site-1/setup-state';

interface ChatbotSite {
    id: string;
    user_id: string;
    domain: string;
    created_at: string;
}

interface ChatbotInternals {
    topIntents: { set: (value: { name: string; value: number }[]) => void };
    topProviders: { set: (value: { name: string; value: number }[]) => void };
    topSurfaces: { set: (value: { name: string; value: number }[]) => void };
    audience: { set: (value: unknown) => void };
    metricCardTabs: () => { id: string; cards: { id: string; title: string; data: { name: string; value: number }[] }[] }[];
    refreshData: (mode?: 'blocking' | 'background') => void;
    isLoadingSeries: () => boolean;
    needsSetup: () => boolean;
}

describe('AIChatbots', () => {
    let component: AIChatbots;
    let fixture: ComponentFixture<AIChatbots>;
    let activeSite: WritableSignal<ChatbotSite | null>;
    let httpMock: HttpTestingController;
    let rangeFrom: string;
    let plan: {
        /** In-range `assistant.chat_started` series. */
        started: EventSeriesPoint[];
        /** While set, every in-range request stays pending so loading flags stay observable. */
        pending: Subject<EventSeriesPoint[]> | null;
    };

    const inRange = <T>(value: T): Observable<T> => (plan.pending ? (plan.pending.asObservable() as unknown as Observable<T>) : of(value));

    const analyticsServiceStub = {
        getEventTimeseries: vi.fn((_siteId: string, from: string, _to: string, eventName: string) => inRange(from === rangeFrom && eventName === CHAT_STARTED_EVENT ? plan.started : [])),
        getEventPropertyBreakdown: vi.fn(() => inRange<{ name: string; value: number }[]>([])),
        getEventAudience: vi.fn(() => inRange(null))
    };

    const instance = () => component as AIChatbots & ChatbotInternals;
    const inRangeCalls = () => analyticsServiceStub.getEventTimeseries.mock.calls as unknown as string[][];
    const comparisonCalls = () => inRangeCalls().filter((call) => call[1] !== rangeFrom);
    const answerSetupState = (overrides: Partial<SiteSetupState> = {}) => flushSetupState(httpMock, 'site-1', overrides, fixture);

    const create = (siteId: string | null = 'site-1'): void => {
        activeSite.set(siteId ? { id: siteId, user_id: 'user-1', domain: 'assistant.test', created_at: '2026-05-01T00:00:00Z' } : null);
        fixture = TestBed.createComponent(AIChatbots);
        component = fixture.componentInstance;
        fixture.detectChanges();
    };

    beforeEach(async () => {
        localStorage.clear();
        analyticsServiceStub.getEventTimeseries.mockClear();
        analyticsServiceStub.getEventPropertyBreakdown.mockClear();
        analyticsServiceStub.getEventAudience.mockClear();
        plan = {
            started: [],
            pending: null
        };

        await TestBed.configureTestingModule({
            imports: [
                AIChatbots,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            aiChatbots: {
                                title: 'AI Chatbots',
                                docsAction: 'Instrumentation guide',
                                setup: {
                                    title: 'Instrument your chatbot',
                                    description: 'Send assistant.* custom events from your chatbot to fill this page.',
                                    docsAction: 'Read the instrumentation guide'
                                },
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
                provideHttpClient(),
                provideHttpClientTesting(),
                provideRouter([]),
                {
                    provide: SiteService,
                    useValue: {
                        activeSite: signal<ChatbotSite | null>(null),
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

        activeSite = TestBed.inject(SiteService).activeSite as unknown as WritableSignal<ChatbotSite | null>;
        httpMock = TestBed.inject(HttpTestingController);
        rangeFrom = TestBed.inject(ReportRangePreferencesService).currentDateRange()!.from;
    });

    afterEach(() => {
        // Drain the shared setup-state lookup for the tests that do not assert on it.
        flushSetupState(httpMock, 'site-1');
        httpMock.verify();
    });

    it('should create', () => {
        create();

        expect(component).toBeTruthy();
    });

    it('keeps chatbot provider cards separate from network provider cards', () => {
        create();

        const chatbots = instance();

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

    it('should reload the report in blocking mode when the toolbar requests a refresh', () => {
        create();

        const callsBeforeRefresh = inRangeCalls().length;
        const breakdownsBeforeRefresh = analyticsServiceStub.getEventPropertyBreakdown.mock.calls.length;
        const audienceBeforeRefresh = analyticsServiceStub.getEventAudience.mock.calls.length;
        const pending = new Subject<EventSeriesPoint[]>();
        plan.pending = pending;

        fixture.debugElement.query(By.directive(ReportRangeToolbar)).componentInstance.refresh.emit();

        expect(inRangeCalls().length).toBeGreaterThan(callsBeforeRefresh);
        expect(analyticsServiceStub.getEventPropertyBreakdown.mock.calls.length).toBeGreaterThan(breakdownsBeforeRefresh);
        expect(analyticsServiceStub.getEventAudience.mock.calls.length).toBeGreaterThan(audienceBeforeRefresh);
        expect(instance().isLoadingSeries()).toBe(true);

        plan.pending = null;
        pending.next([]);
        pending.complete();

        expect(instance().isLoadingSeries()).toBe(false);
    });

    it('should reload the comparison series when the toolbar requests a refresh', () => {
        create();

        const comparisonCallsBefore = comparisonCalls().length;

        instance().refreshData();

        expect(comparisonCalls().length).toBeGreaterThan(comparisonCallsBefore);
    });

    it('should not reload the comparison series on a background realtime refresh', () => {
        create();

        const comparisonCallsBefore = comparisonCalls().length;

        instance().refreshData('background');

        expect(comparisonCalls().length).toBe(comparisonCallsBefore);
    });

    it('should ask for the shared setup state only when the selected range has no conversations', () => {
        create();

        expect(httpMock.expectOne(SETUP_STATE_URL).request.method).toBe('GET');
    });

    it('should skip the setup-state lookup when the selected range already has conversations', () => {
        plan.started = [{ time: '2026-07-25T00:00:00Z', count: 4 }];

        create();

        httpMock.expectNone(SETUP_STATE_URL);
    });

    it('should render a setup callout when the site never sent chatbot events', () => {
        create();
        answerSetupState({ has_chatbot_events: false });

        const callout = fixture.nativeElement.querySelector('[data-testid="ai-chatbots-setup-callout"]');
        expect(callout).not.toBeNull();
        expect(callout.textContent).toContain('Instrument your chatbot');
        expect(callout.querySelector('a[href="https://hitkeep.com/guides/analytics/ai-chatbot-analytics/"]')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('app-kpi-card')).toBeNull();
        expect(fixture.nativeElement.querySelector('app-metric-card-group')).toBeNull();
        expect(fixture.nativeElement.querySelector('app-report-range-toolbar')).not.toBeNull();
    });

    it('should keep the regular empty states when chatbot events exist outside the selected range', () => {
        create();
        answerSetupState({ has_chatbot_events: true });

        expect(fixture.nativeElement.querySelector('[data-testid="ai-chatbots-setup-callout"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('app-kpi-card')).not.toBeNull();
        expect(fixture.nativeElement.querySelector('app-metric-card-group')).not.toBeNull();
    });

    it('should treat a failed setup-state lookup as an instrumented site', () => {
        create();
        httpMock.expectOne(SETUP_STATE_URL).flush(null, { status: 503, statusText: 'Service Unavailable' });
        fixture.detectChanges();

        expect(instance().needsSetup()).toBe(false);
        expect(fixture.nativeElement.querySelector('[data-testid="ai-chatbots-setup-callout"]')).toBeNull();
    });

    it('should reuse the cached setup state so refreshes do not ask again', () => {
        create();
        answerSetupState({ has_chatbot_events: false });
        instance().refreshData();

        httpMock.expectNone(SETUP_STATE_URL);
    });
});
