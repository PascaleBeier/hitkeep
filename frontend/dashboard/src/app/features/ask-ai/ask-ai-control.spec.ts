import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter, Router } from '@angular/router';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { EMPTY, Subject, of } from 'rxjs';
import { AskAIAction, AskAIChart, AskAIResponse, AskAIStatus, Site, SystemStatus } from '@models/analytics.types';
import { AskAIService } from '@services/ask-ai.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { PermissionService } from '@services/permission.service';
import { TeamService } from '@services/team.service';
import { SiteService } from '@features/sites/services/site.service';
import { TakeoutDownloadService } from '@services/takeout-download.service';
import { AskAIControl } from './ask-ai-control';
import { AskAIAnswer } from './ask-ai-answer';

class FakeAskAIService {
    askStream() {
        return EMPTY;
    }
    getStatus() {
        return of(systemStatus().ask_ai!);
    }
}

class FakeTakeoutDownloadService {
    readonly download = new Subject<void>();
    downloadFromUrl() {
        return this.download.asObservable();
    }
}

interface AskAIControlTestAccess {
    drawerVisible: { set(value: boolean): void };
    siteStatus: { set(value: AskAIStatus | null): void };
    query: { set(value: string): void };
    runningActionKey: () => string;
    runAction(action: AskAIAction): void;
    submit(): void;
    openPlanComparison(): void;
    openSystemStatus(): void;
    response: { set(value: AskAIResponse | null): void };
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
                                tools: { siteOverview: 'Site overview' },
                                actions: 'Actions',
                                conversation: 'Conversation',
                                triggerAria: 'Ask AI about this site',
                                triggerPlaceholder: 'Ask AI',
                                promptAria: 'Ask AI prompt',
                                promptPlaceholder: 'Ask about this site',
                                askAction: 'Ask',
                                loading: 'Working on it...',
                                mcp: { title: 'Use your own AI agent', description: 'Connect with MCP.', action: 'MCP guide' },
                                budget: {
                                    trigger: 'Ask AI paused',
                                    title: 'AI usage limit reached',
                                    description: 'This instance has reached its configured AI request or token allowance.',
                                    operator: 'Review AI usage in System Status.',
                                    askAdmin: 'Ask your instance operator to review AI usage.',
                                    statusAction: 'Open System Status',
                                    guide: 'AI setup and limits'
                                },
                                quota: {
                                    refresh: 'Check availability',
                                    remaining: '{{remaining}} of {{limit}} team questions left today',
                                    exhausted: 'Team daily limit reached',
                                    resets: 'Resets {{reset}}',
                                    proUpgrade: 'Pro gives your team 100 answers a day',
                                    upgradeAction: 'Explore Pro',
                                    askAdmin: 'Ask a team admin about upgrading'
                                },
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
            providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting(), { provide: AskAIService, useClass: FakeAskAIService }, { provide: TakeoutDownloadService, useClass: FakeTakeoutDownloadService }]
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

    it('shows Cloud Free quota and the existing plan comparison to a team manager', async () => {
        await fixture.whenStable();
        setCloudTeam('free', 'owner');
        component.siteStatus.set({
            enabled: true,
            available: false,
            status: 'daily_limit_exhausted',
            budget_exhausted: false,
            daily_limit: 1,
            daily_used: 1,
            daily_remaining: 0,
            daily_reset_at: '2026-09-27T00:00:00Z'
        });
        fixture.detectChanges();

        expect(document.body.textContent).toContain('0 of 1 team questions left today');
        expect(document.body.textContent).toContain('Team daily limit reached');
        expect(document.body.textContent).toContain('Pro gives your team 100 answers a day');
        expect(document.body.textContent).toContain('Explore Pro');
        expect(document.body.textContent).toContain('Use your own AI agent');
        expect(document.body.querySelector<HTMLAnchorElement>('.ai-mcp a')?.href).toContain('/guides/integrations/mcp/');
        expect(document.body.querySelector<HTMLTextAreaElement>('textarea[name="ask-ai-panel-query"]')?.disabled).toBe(true);

        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        component.openPlanComparison();
        expect(navigate).toHaveBeenCalledWith('/admin/team/overview');
    });

    it('keeps Cloud Free upgrade information visible to team members without offering a guarded action', async () => {
        await fixture.whenStable();
        setCloudTeam('free', 'member');
        component.siteStatus.set({
            enabled: true,
            available: false,
            status: 'daily_limit_exhausted',
            budget_exhausted: false,
            daily_limit: 1,
            daily_used: 1,
            daily_remaining: 0
        });
        fixture.detectChanges();

        expect(document.body.textContent).toContain('Pro gives your team 100 answers a day');
        expect(document.body.textContent).toContain('Ask a team admin about upgrading');
        expect(document.body.textContent).not.toContain('Explore Pro');
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl');
        component.openPlanComparison();
        expect(navigate).not.toHaveBeenCalled();
    });

    it('does not pitch Pro to paid Cloud teams or show daily limits on self-hosted instances', async () => {
        await fixture.whenStable();
        setCloudTeam('pro', 'owner');
        component.siteStatus.set({ enabled: true, available: true, status: 'available', budget_exhausted: false, daily_limit: 100, daily_used: 1, daily_remaining: 99 });
        fixture.detectChanges();
        expect(document.body.textContent).toContain('99 of 100 team questions left today');
        expect(document.body.textContent).not.toContain('Pro gives your team 100 answers a day');

        setCloudTeam('free', 'owner');
        component.siteStatus.set({
            enabled: true,
            available: false,
            status: 'budget_exhausted',
            budget_exhausted: true,
            daily_limit: 1,
            daily_used: 1,
            daily_remaining: 0,
            daily_reset_at: '2026-09-27T00:00:00Z'
        });
        fixture.detectChanges();
        expect(document.body.textContent).not.toContain('Pro gives your team 100 answers a day');

        TestBed.inject(DashboardBootstrapService).status.set(systemStatus());
        fixture.detectChanges();
        expect(document.body.textContent).not.toContain('team questions left today');
        expect(document.body.textContent).not.toContain('Pro gives your team 100 answers a day');
        expect(document.body.textContent).toContain('AI usage limit reached');
        expect(document.body.textContent).toContain('configured AI request or token allowance');
        expect(document.body.textContent).toContain('Ask your instance operator');
        expect(document.body.textContent).not.toContain('Open System Status');
        expect(document.body.textContent).toContain('Check availability');
        expect(document.body.querySelector<HTMLAnchorElement>('.ai-budget-help a')?.href).toContain('/guides/admin/ai-model-configuration/');
        expect(document.body.querySelector<HTMLTextAreaElement>('textarea[name="ask-ai-panel-query"]')?.disabled).toBe(true);
        expect(document.body.querySelector('.ask-ai-drawer')).toBeTruthy();
    });

    it('opens System Status only for instance operators when the self-hosted budget is exhausted', async () => {
        await fixture.whenStable();
        TestBed.inject(PermissionService).applyPermissions({ instance_role: 'admin', permissions: {}, active_team_id: '', active_team_role: '' });
        component.siteStatus.set({ enabled: true, available: false, status: 'budget_exhausted', budget_exhausted: true });
        fixture.detectChanges();

        expect(document.body.textContent).toContain('Open System Status');
        expect(document.body.textContent).toContain('Review AI usage in System Status');
        expect(document.body.textContent).not.toContain('Ask your instance operator');
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        component.openSystemStatus();
        expect(navigate).toHaveBeenCalledWith('/admin/status');
    });

    it('releases a pending export after a follow-up request settles', () => {
        const action: AskAIAction = { type: 'download_export', label: 'Download export', target: '/api/sites/site-1/takeout?format=json', format: 'json' };
        const takeout = TestBed.inject(TakeoutDownloadService) as unknown as FakeTakeoutDownloadService;
        component.runAction(action);
        expect(component.runningActionKey()).not.toBe('');

        component.query.set('What changed next?');
        component.submit();
        takeout.download.complete();

        expect(component.runningActionKey()).toBe('');
    });

    it('uses validated source chips without mangling unknown markers or tool identifiers', async () => {
        await fixture.whenStable();
        component.response.set({
            ...responseWithCharts(),
            answer_markdown: 'Visits rose. [Source: hitkeep_get_site_overview]\n\nKeep [Source: unknown_tool] and hitkeep_other_tool intact.',
            // The server labels citations with English tool titles; known tools are translated.
            citations: [{ label: 'Get HitKeep Site Overview', tool_call_id: 'hitkeep_get_site_overview' }]
        });
        fixture.detectChanges();

        const answer = document.body.querySelector('.ai-answer-body')?.textContent ?? '';
        expect(answer).toContain('Visits rose.');
        expect(answer).not.toContain('[Source: hitkeep_get_site_overview]');
        expect(answer).toContain('[Source: unknown_tool]');
        expect(answer).toContain('hitkeep_other_tool');
        expect(document.body.querySelector('.ai-citation')?.textContent?.trim()).toBe('Site overview');
    });

    it('renders useful Markdown while dropping active HTML, image loads, and unsafe links', async () => {
        await fixture.whenStable();
        component.response.set({
            ...responseWithCharts(),
            answer_markdown:
                '## Summary\n\n**Bold** and _emphasis_ with [safe](https://example.com/report) and [unsafe](javascript:alert(1)).\n\n- First\n- Second\n\n3. Third\n4. Fourth\n\n```ts\nconst value = "<img src=x>";\n```\n\n| Day | Visits |\n| --- | ---: |\n| Monday | 42 |\n\n![remote](https://example.com/pixel)\n\n<script>window.__askAiXss = true</script><img src="https://example.com/track" onerror="window.__askAiXss = true">',
            citations: []
        });
        fixture.detectChanges();

        const markdown = document.body.querySelector('.ai-markdown')!;
        expect(markdown.querySelector('h2')?.textContent).toBe('Summary');
        expect(markdown.querySelector('strong')?.textContent).toBe('Bold');
        expect(markdown.querySelector('em')?.textContent).toBe('emphasis');
        expect(markdown.querySelectorAll('li').length).toBe(4);
        expect(markdown.querySelector('ol')?.getAttribute('start')).toBe('3');
        expect(markdown.querySelector('pre code')?.textContent).toContain('<img src=x>');
        expect(markdown.querySelector('table td')?.textContent).toBe('Monday');
        expect(markdown.querySelector('a[href="https://example.com/report"]')).not.toBeNull();
        expect(markdown.querySelectorAll('a').length).toBe(1);
        expect(markdown.querySelector('img, script, iframe, svg')).toBeNull();
        expect((window as unknown as { __askAiXss?: boolean }).__askAiXss).toBeUndefined();
    });

    it('reformats a table-only answer when the language changes', async () => {
        await fixture.whenStable();
        const transloco = TestBed.inject(TranslocoService);
        component.response.set({
            ...responseWithCharts(),
            answer_markdown: 'Counts',
            charts: [{ ...responseWithCharts().charts[0]!, type: 'table', title: 'Counts', rows: [{ count: 1234.5 }] }],
            citations: [],
            actions: []
        });
        fixture.detectChanges();
        const cell = document.body.querySelector('.ai-chart tbody td')!;
        expect(cell.textContent).toContain('1,234.5');

        transloco.setActiveLang('de');
        fixture.detectChanges();
        await fixture.whenStable();
        expect(cell.textContent).toContain('1.234,5');
    });

    it('renders ECharts for Ask AI line and bar charts instead of the OptimusUI chart element', async () => {
        await fixture.whenStable();
        fixture.detectChanges();

        const chartHosts = new Set([...Array.from(document.body.querySelectorAll('.ai-canvas .ai-echarts')), ...Array.from(fixture.nativeElement.querySelectorAll('.ai-canvas .ai-echarts'))]);
        expect(chartHosts.size).toBe(2);
        expect(document.body.querySelector('p-' + 'chart')).toBeNull();

        const answer = fixture.debugElement.query(By.directive(AskAIAnswer)).componentInstance as { answer(): AskAIResponse | null; chartOptions(chart: AskAIChart): unknown };
        const barOptions = answer.chartOptions(answer.answer()!.charts[1]!) as InspectableChartOption;
        expect(barOptions.aria.label.description).toBe('Top sources');
        expect(barOptions.series[0]?.type).toBe('bar');
        expect(barOptions.series[0]?.name).toBe('Visits');
    });
});

function setCloudTeam(planCode: 'free' | 'pro', role: 'owner' | 'member'): void {
    TestBed.inject(DashboardBootstrapService).status.set({ ...systemStatus(), cloud: { hosted: true, signup_enabled: true } });
    TestBed.inject(TeamService).applyTeams({
        teams: [
            {
                id: 'team-1',
                name: 'Team',
                logo_url: '',
                role,
                created_at: '2026-07-01T00:00:00Z',
                plan: { code: planCode, name: planCode }
            }
        ],
        active_team_id: 'team-1'
    });
    TestBed.inject(PermissionService).applyPermissions({ instance_role: 'user', permissions: {}, active_team_id: 'team-1', active_team_role: role });
}

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
