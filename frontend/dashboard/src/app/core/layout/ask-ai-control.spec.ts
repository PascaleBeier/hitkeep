import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { EMPTY } from 'rxjs';
import { AskAIChart, AskAIResponse, Site, SystemStatus } from '@models/analytics.types';
import { AskAIService } from '@services/ask-ai.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { SiteService } from '@features/sites/services/site.service';
import { AskAIControl } from './ask-ai-control';

class FakeAskAIService {
    askStream() {
        return EMPTY;
    }
}

interface AskAIControlTestAccess {
    drawerVisible: { set(value: boolean): void };
    response: { set(value: AskAIResponse | null): void };
    chartOptions(chart: AskAIChart): unknown;
}

interface InspectableChartOption {
    aria: { label: { description: string } };
    series: { type: string; name: string }[];
}

describe('AskAIControl charts', () => {
    let fixture: ComponentFixture<AskAIControl>;
    let component: AskAIControlTestAccess;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                AskAIControl,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            askAi: {
                                title: 'Ask AI',
                                charts: 'Charts',
                                answer: 'Answer',
                                citations: 'Sources',
                                actions: 'Actions',
                                conversation: 'Conversation',
                                triggerAria: 'Ask AI about this site',
                                triggerPlaceholder: 'Ask AI',
                                promptAria: 'Ask AI prompt',
                                promptPlaceholder: 'Ask about this site',
                                askAction: 'Ask',
                                loading: 'Working on it...',
                                status: { ready: 'Ready' },
                                disabled: {
                                    notConfigured: 'Ask AI not configured',
                                    budget: 'AI budget exhausted',
                                    unavailable: 'Ask AI unavailable'
                                },
                                suggestionsLabel: 'Ask AI suggested prompts',
                                suggestions: {
                                    traffic: 'What changed in traffic?',
                                    events: 'Which events drove conversions?',
                                    export: 'Prepare an export for the current view'
                                },
                                dictation: {
                                    startAria: 'Start voice dictation',
                                    stopAria: 'Stop voice dictation'
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
            providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting(), { provide: AskAIService, useClass: FakeAskAIService }]
        }).compileComponents();

        TestBed.inject(SiteService).applySites([site()]);
        TestBed.inject(DashboardBootstrapService).status.set(systemStatus());
        fixture = TestBed.createComponent(AskAIControl);
        component = fixture.componentInstance as unknown as AskAIControlTestAccess;
        fixture.detectChanges();
        component.drawerVisible.set(true);
        component.response.set(responseWithCharts());
        fixture.detectChanges();
    });

    afterEach(() => {
        fixture.destroy();
    });

    it('renders ECharts for Ask AI line and bar charts instead of the PrimeNG chart element', async () => {
        await fixture.whenStable();
        fixture.detectChanges();

        const chartHosts = new Set([...Array.from(document.body.querySelectorAll('.ai-canvas .ai-echarts')), ...Array.from(fixture.nativeElement.querySelectorAll('.ai-canvas .ai-echarts'))]);
        expect(chartHosts.size).toBe(2);
        expect(document.body.querySelector('p-' + 'chart')).toBeNull();

        const barOptions = component.chartOptions(responseWithCharts().charts[1]!) as InspectableChartOption;
        expect(barOptions.aria.label.description).toBe('Top sources');
        expect(barOptions.series[0]?.type).toBe('bar');
        expect(barOptions.series[0]?.name).toBe('Visits');
    });
});

function site(): Site {
    return {
        id: 'site-1',
        user_id: 'user-1',
        domain: 'example.com',
        created_at: '2026-07-01T00:00:00Z'
    };
}

function systemStatus(): SystemStatus {
    return {
        needs_setup: false,
        version: 'test',
        ask_ai: {
            enabled: true,
            available: true,
            status: 'available',
            budget_exhausted: false
        }
    };
}

function responseWithCharts(): AskAIResponse {
    return {
        run_id: 'run-1',
        answer_markdown: 'Here are the charts.',
        citations: [],
        actions: [],
        charts: [
            {
                type: 'line',
                title: 'Traffic',
                x_key: 'day',
                series: [{ key: 'visits', label: 'Visits' }],
                rows: [
                    { day: '2026-07-01', visits: 12 },
                    { day: '2026-07-02', visits: 18 }
                ]
            },
            {
                type: 'bar',
                title: 'Top sources',
                x_key: 'source',
                series: [{ key: 'visits', label: 'Visits' }],
                rows: [
                    { source: 'Direct', visits: 8 },
                    { source: 'Search', visits: 14 }
                ]
            }
        ]
    };
}
