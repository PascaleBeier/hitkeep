import { ComponentFixture, TestBed } from '@angular/core/testing';
import { MainLayout } from '@layout/main-layout';
import { Router, provideRouter } from '@angular/router';
import { By } from '@angular/platform-browser';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { Observable, Subject, of } from 'rxjs';
import { AskAIRequest, AskAIResponse, AskAIStreamEvent } from '@models/analytics.types';
import { AskAIService, AskAIStreamStatusError } from '@services/ask-ai.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { PermissionService } from '@services/permission.service';
import { ShareService } from '@services/share.service';
import { TeamService } from '@services/team.service';
import { UserProfileService } from '@services/user-profile.service';
import { SiteService } from '@features/sites/services/site.service';
import { AskAIControl } from './ask-ai-control';
import { LayoutPageBar } from './layout-page-bar';
import { LayoutSidebar } from './layout-sidebar';
import { MainLayoutContextService } from './main-layout-context.service';
import { MenuItem } from '@openng/optimus-ui/api';
import { vi } from 'vitest';

interface LayoutSidebarTestAccess {
    openSiteSettings(section?: string): void;
    closeMobileDrawer(): void;
    mobileMenuItems(): MenuItem[];
    canCreateTeams(): boolean;
}

interface LayoutPageBarTestAccess {
    canCreateTeams(): boolean;
}

interface AskAIStreamRequest {
    siteId: string;
    request: AskAIRequest;
    stream: Subject<AskAIStreamEvent>;
    aborted: boolean;
}

class FakeAskAIService {
    readonly requests: AskAIStreamRequest[] = [];

    askStream(siteId: string, request: AskAIRequest): Observable<AskAIStreamEvent> {
        const stream = new Subject<AskAIStreamEvent>();
        const streamRequest = { siteId, request, stream, aborted: false };
        this.requests.push(streamRequest);
        return new Observable<AskAIStreamEvent>((subscriber) => {
            const subscription = stream.subscribe(subscriber);
            return () => {
                streamRequest.aborted = true;
                subscription.unsubscribe();
            };
        });
    }
}

describe('MainLayout', () => {
    let component: MainLayout;
    let fixture: ComponentFixture<MainLayout>;
    let httpMock: HttpTestingController;
    let bootstrap: DashboardBootstrapService;
    let layoutContext: MainLayoutContextService;
    let askAI: FakeAskAIService;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                MainLayout,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: {
                                utm: 'UTM',
                                utmBuilder: 'UTM Builder',
                                qrCodes: 'QR codes',
                                importExport: 'Import & Export',
                                importExportAria: 'Go to import and export',
                                expandItem: 'Expand {{item}}',
                                collapseItem: 'Collapse {{item}}'
                            },
                            askAi: {
                                triggerPlaceholder: 'Ask AI',
                                triggerAria: 'Ask AI about this site',
                                submitAria: 'Send Ask AI question',
                                title: 'Ask AI',
                                promptAria: 'Ask AI prompt',
                                promptPlaceholder: 'Ask about this site',
                                followUpTriggerPlaceholder: 'Ask a follow-up',
                                followUpPlaceholder: 'Ask a follow-up about this result',
                                askAction: 'Ask',
                                stop: 'Stop',
                                stopAria: 'Stop Ask AI response',
                                stopped: 'Response stopped.',
                                newChat: 'New chat',
                                newChatAria: 'Start a new Ask AI chat',
                                dictation: {
                                    startAria: 'Start voice dictation',
                                    stopAria: 'Stop voice dictation'
                                },
                                currentQuestion: 'Current question',
                                conversation: 'Conversation',
                                questionLabel: 'You',
                                answerLabel: 'Ask AI',
                                loading: 'Working on it...',
                                progress: {
                                    accepted: 'Starting request...',
                                    generating: 'Checking site analytics...',
                                    readingAnalytics: 'Reading analytics...',
                                    composing: 'Writing answer...',
                                    complete: 'Ready.'
                                },
                                tools: {
                                    siteOverview: 'Site overview',
                                    eventNames: 'Event names',
                                    eventBreakdown: 'Event breakdown',
                                    ecommerce: 'Ecommerce',
                                    webVitals: 'Web Vitals',
                                    aiVisibility: 'AI visibility',
                                    analytics: 'Analytics'
                                },
                                status: {
                                    ready: 'Ready'
                                },
                                answer: 'Answer',
                                citations: 'Sources',
                                charts: 'Charts',
                                actions: 'Actions',
                                emptyTitle: 'Suggested prompts',
                                suggestionsLabel: 'Ask AI suggested prompts',
                                suggestions: {
                                    traffic: 'What changed in traffic?',
                                    events: 'Which events drove conversions?',
                                    export: 'Prepare an export for the current view'
                                },
                                exportSuccess: 'Export download started.',
                                disabled: {
                                    notConfigured: 'Ask AI not configured',
                                    budget: 'AI budget exhausted',
                                    unavailable: 'Ask AI unavailable'
                                },
                                errors: {
                                    request: 'Ask AI could not answer right now.',
                                    action: 'Action could not be opened.',
                                    export: 'Export could not be downloaded.',
                                    budget: 'AI budget exhausted.',
                                    notConfigured: 'Ask AI not configured.'
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

        fixture = TestBed.createComponent(MainLayout);
        component = fixture.componentInstance;
        httpMock = TestBed.inject(HttpTestingController);
        bootstrap = TestBed.inject(DashboardBootstrapService);
        layoutContext = fixture.debugElement.injector.get(MainLayoutContextService);
        askAI = TestBed.inject(AskAIService) as unknown as FakeAskAIService;
        seedLayoutState();
        fixture.detectChanges();
        fixture.detectChanges();
    });

    afterEach(() => {
        httpMock.verify();
        vi.restoreAllMocks();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('renders and dismisses one-shot navigation feedback', () => {
        const notice = TestBed.inject(NavigationNoticeService);
        notice.show('sites.settings.notices.siteUnavailable');
        fixture.detectChanges();

        const message = fixture.nativeElement.querySelector('p-message');
        expect(message?.textContent).toContain('sites.settings.notices.siteUnavailable');

        const close = message?.querySelector('button') as HTMLButtonElement | null;
        expect(close).toBeTruthy();
        close?.click();
        fixture.detectChanges();
        expect(fixture.nativeElement.querySelector('p-message')).toBeNull();
    });

    it('A11Y: should have correct landmarks', () => {
        const aside = fixture.debugElement.query(By.css('aside'));
        const main = fixture.debugElement.query(By.css('main'));
        const nav = fixture.debugElement.query(By.css('nav'));

        expect(aside).toBeTruthy();
        expect(main).toBeTruthy();
        expect(nav).toBeTruthy();

        // Check labels
        expect(aside.attributes['aria-label']).toBeTruthy();
        expect(main.attributes['role']).toBe('main');
    });

    it('A11Y: buttons should have accessible labels', () => {
        const buttons = fixture.debugElement.queryAll(By.css('button'));
        const buttonsWithAria = buttons.filter((btn) => !!btn.attributes['aria-label']);
        expect(buttonsWithAria.length).toBeGreaterThan(0);
    });

    it('should always render team switcher', () => {
        const switchers = fixture.debugElement.queryAll(By.css('app-team-switcher'));
        expect(switchers.length).toBeGreaterThan(0);
    });

    it('should not render the legacy floating desktop account cluster', () => {
        const topbarCluster = fixture.nativeElement.querySelector('.layout-topbar__account-cluster');
        expect(topbarCluster).toBeNull();
    });

    it('should show administration section for team owner/admin role', () => {
        const adminLinks = Array.from(fixture.nativeElement.querySelectorAll('nav a')) as HTMLElement[];
        const hasTeamLink = adminLinks.some((link: HTMLElement) => link.getAttribute('href') === '/admin/team');
        expect(hasTeamLink).toBeTruthy();
    });

    it('should hide administration section for team member role', () => {
        const teamService = TestBed.inject(TeamService);
        teamService.teams.set([
            {
                id: '00000000-0000-0000-0000-000000000001',
                name: 'Alpha Team',
                logo_url: '',
                role: 'member',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);
        teamService.activeTeamId.set('00000000-0000-0000-0000-000000000001');
        fixture.detectChanges();
        const adminLinks = Array.from(fixture.nativeElement.querySelectorAll('nav a')) as HTMLElement[];
        const hasTeamLink = adminLinks.some((link: HTMLElement) => link.getAttribute('href') === '/admin/team');
        expect(hasTeamLink).toBeFalsy();
    });

    it('should show Search Console setup navigation for team owner/admin role', () => {
        const navLinks = Array.from(fixture.nativeElement.querySelectorAll('nav a')) as HTMLElement[];
        const hasSearchConsoleLink = navLinks.some((link: HTMLElement) => link.getAttribute('href') === '/integration/google-search-console');
        expect(hasSearchConsoleLink).toBeTruthy();
    });

    it('should hide Search Console setup navigation for team member role', () => {
        const teamService = TestBed.inject(TeamService);
        teamService.teams.set([
            {
                id: '00000000-0000-0000-0000-000000000001',
                name: 'Alpha Team',
                logo_url: '',
                role: 'member',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);
        teamService.activeTeamId.set('00000000-0000-0000-0000-000000000001');
        fixture.detectChanges();

        const navLinks = Array.from(fixture.nativeElement.querySelectorAll('nav a')) as HTMLElement[];
        const hasSearchConsoleLink = navLinks.some((link: HTMLElement) => link.getAttribute('href') === '/integration/google-search-console');
        expect(hasSearchConsoleLink).toBeFalsy();
    });

    it('should show Import & Export navigation to site viewers through the smart hub route', () => {
        const permissions = TestBed.inject(PermissionService);
        const siteService = TestBed.inject(SiteService);
        siteService.applySites([
            {
                id: '00000000-0000-0000-0000-0000000000aa',
                user_id: '00000000-0000-0000-0000-000000000001',
                domain: 'viewer.example.com',
                created_at: '2026-01-01T00:00:00Z'
            }
        ]);
        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {
                '00000000-0000-0000-0000-0000000000aa': 'viewer'
            }
        });
        fixture.detectChanges();

        const navLinks = Array.from(fixture.nativeElement.querySelectorAll('nav a')) as HTMLAnchorElement[];
        const importExportLink = navLinks.find((link) => link.textContent?.includes('Import & Export'));

        expect(importExportLink).toBeTruthy();
        expect(importExportLink?.getAttribute('href')).toBe('/import-export');
        expect(importExportLink?.getAttribute('aria-label') ?? importExportLink?.closest('[role="treeitem"]')?.getAttribute('aria-label')).toBe('Import & Export');
    });

    it('should hide Import & Export navigation in share mode', () => {
        TestBed.inject(ShareService).setToken('share-token');
        fixture.detectChanges();

        const navLinks = Array.from(fixture.nativeElement.querySelectorAll('nav a')) as HTMLAnchorElement[];
        const importExportLink = navLinks.find((link) => link.getAttribute('href') === '/import-export');

        expect(importExportLink).toBeFalsy();
    });

    it('should show UTM and QR Codes navigation in share mode', () => {
        TestBed.inject(ShareService).setToken('share-token');
        fixture.detectChanges();

        const navLinks = Array.from(fixture.nativeElement.querySelectorAll('nav a')) as HTMLAnchorElement[];
        const visibleHrefs = navLinks.map((link) => link.getAttribute('href'));

        expect(visibleHrefs).toContain('/share/share-token/utm');
        expect(visibleHrefs).toContain('/share/share-token/utm/qr-codes');
    });

    it('should hide Ask AI by default', () => {
        seedActiveSite();
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('.ask-ai-trigger')).toBeNull();
    });

    it('should show Ask AI below the site selector when available', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'amazon.nova-lite-v1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        const trigger = fixture.nativeElement.querySelector('.ask-ai-trigger') as HTMLButtonElement | null;
        expect(trigger).toBeTruthy();
        expect(trigger?.textContent).toContain('Ask AI');
        expect(trigger?.disabled).toBe(false);
    });

    it('should show Ask AI as unavailable when the product flag is on but no model is configured', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: false,
                status: 'not_configured',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        const trigger = fixture.nativeElement.querySelector('.ask-ai-trigger') as HTMLButtonElement | null;
        expect(trigger).toBeTruthy();
        expect(trigger?.textContent).toContain('Ask AI not configured');
        expect(trigger?.classList.contains('ask-ai-trigger--unavailable')).toBe(true);
    });

    it('should show unavailable Ask AI as a chat-only drawer without history or model details', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: false,
                status: 'budget_exhausted',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: true
            }
        });
        fixture.detectChanges();

        openAskAIDrawer();
        fixture.detectChanges();

        expect(askAI.requests).toEqual([]);
        expect(document.body.textContent).toContain('AI budget exhausted');
        expect(document.body.textContent).not.toContain('Run history');
        expect(document.body.textContent).not.toContain('History');
        expect(document.body.textContent).not.toContain('openai.gpt-oss-120b-1:0');
    });

    it('should clear Ask AI state and stale responses when the active site changes', () => {
        const siteService = TestBed.inject(SiteService);
        const firstSite = {
            id: '00000000-0000-0000-0000-0000000000bb',
            user_id: '00000000-0000-0000-0000-000000000001',
            domain: 'active.example.com',
            created_at: '2026-01-01T00:00:00Z'
        };
        const secondSite = {
            id: '00000000-0000-0000-0000-0000000000cc',
            user_id: '00000000-0000-0000-0000-000000000001',
            domain: 'next.example.com',
            created_at: '2026-01-01T00:00:00Z'
        };
        siteService.applySites([firstSite, secondSite]);
        siteService.selectSite(firstSite);
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'amazon.nova-lite-v1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        submitAskAIQuestion('What changed on site A?');
        const firstRequest = expectAskAIRequest(firstSite.id);
        expect(firstRequest.request.query).toBe('What changed on site A?');
        expect(firstRequest.request.from).toBeUndefined();
        expect(firstRequest.request.to).toBeUndefined();

        siteService.selectSite(secondSite);
        fixture.detectChanges();
        completeAskAIRequest(firstRequest, {
            run_id: '11111111-1111-4111-8111-111111111111',
            answer_markdown: 'Site A answer that must not appear.',
            citations: [],
            charts: [],
            actions: []
        });
        fixture.detectChanges();

        expect(document.body.textContent).not.toContain('Site A answer that must not appear.');

        submitAskAIQuestion('What changed on site B?');
        const secondRequest = expectAskAIRequest(secondSite.id);
        expect(secondRequest.request.query).toBe('What changed on site B?');
        expect(secondRequest.request.history ?? []).toEqual([]);
        completeAskAIRequest(secondRequest, {
            run_id: '22222222-2222-4222-8222-222222222222',
            answer_markdown: 'Site B answer.',
            citations: [],
            charts: [],
            actions: []
        });
        fixture.detectChanges();

        expect(document.body.textContent).toContain('Site B answer.');
    });

    it('should keep follow-up context and let users start a new Ask AI chat', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'amazon.nova-lite-v1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        submitAskAIQuestion('What changed in traffic?');
        const firstRequest = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        expect(firstRequest.request.history ?? []).toEqual([]);
        completeAskAIRequest(firstRequest, {
            run_id: '11111111-1111-4111-8111-111111111111',
            answer_markdown: 'Traffic increased.',
            citations: [],
            charts: [],
            actions: []
        });
        fixture.detectChanges();

        expect(document.body.textContent).toContain('New chat');

        submitAskAIQuestion('Why did that happen?');
        const followUpRequest = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        expect(followUpRequest.request.history).toEqual([
            { role: 'user', content: 'What changed in traffic?' },
            { role: 'assistant', content: 'Traffic increased.' }
        ]);
        completeAskAIRequest(followUpRequest, {
            run_id: '22222222-2222-4222-8222-222222222222',
            answer_markdown: 'Search referrals lifted the period.',
            citations: [],
            charts: [],
            actions: []
        });
        fixture.detectChanges();

        expect(document.body.textContent).toContain('What changed in traffic?');
        askAIControl().startNewChat();
        fixture.detectChanges();
        expect(document.body.textContent).not.toContain('Search referrals lifted the period.');
        expect(document.body.textContent).not.toContain('New chat');

        submitAskAIQuestion('Fresh analysis please');
        const freshRequest = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        expect(freshRequest.request.history ?? []).toEqual([]);
        completeAskAIRequest(freshRequest, {
            run_id: '33333333-3333-4333-8333-333333333333',
            answer_markdown: 'Fresh answer.',
            citations: [],
            charts: [],
            actions: []
        });
        fixture.detectChanges();
    });

    it('should render streamed Ask AI answer deltas before the final response', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        submitAskAIQuestion('What changed in traffic?');
        const request = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        request.stream.next({
            type: 'progress',
            status: 'generating',
            message_key: 'askAi.progress.generating'
        });
        fixture.detectChanges();

        expect(document.body.textContent).toContain('Checking site analytics...');
        expect(document.body.textContent).not.toContain('Traffic increased.');
        request.stream.next({
            type: 'delta',
            status: 'streaming',
            delta_markdown: 'Traffic '
        });
        fixture.detectChanges();
        expect(document.body.textContent).toContain('Traffic');
        expect(document.body.textContent).not.toContain('Open events');

        request.stream.next({
            type: 'delta',
            status: 'streaming',
            delta_markdown: 'increased.'
        });
        fixture.detectChanges();
        expect(document.body.textContent).toContain('Traffic increased.');

        completeAskAIRequest(request, {
            run_id: '11111111-1111-4111-8111-111111111111',
            answer_markdown: 'Traffic increased.',
            citations: [],
            charts: [],
            actions: [{ type: 'navigate', label: 'Open events', target: '/events' }]
        });
        fixture.detectChanges();

        expect(document.body.textContent).toContain('Traffic increased.');
        expect(document.body.textContent).toContain('Open events');
        expect(document.body.textContent).not.toContain('Checking site analytics...');
    });

    it('should render Ask AI tool call progress rows inside the chat thread', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        submitAskAIQuestion('How many hits did I get from ChatGPT?');
        const request = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        request.stream.next({
            type: 'progress',
            status: 'tool_call_start',
            message_key: 'askAi.progress.readingAnalytics',
            tool_call_id: 'tool-1',
            tool_name: 'hitkeep_get_event_names'
        });
        fixture.detectChanges();

        const runningRow = document.body.querySelector('.ai-tool-row') as HTMLElement | null;
        expect(runningRow?.textContent).toContain('Event names');
        expect(runningRow?.querySelector('.pi-spinner')).not.toBeNull();

        request.stream.next({
            type: 'progress',
            status: 'tool_call_finish',
            message_key: 'askAi.progress.composing',
            tool_call_id: 'tool-1',
            tool_name: 'hitkeep_get_event_names'
        });
        fixture.detectChanges();

        const doneRow = document.body.querySelector('.ai-tool-row') as HTMLElement | null;
        expect(doneRow?.textContent).toContain('Event names');
        expect(doneRow?.querySelector('.pi-check')).not.toBeNull();
        expect(document.body.querySelectorAll('.ai-tool-row').length).toBe(1);
    });

    it('should show an error when the Ask AI stream closes without a terminal event', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        submitAskAIQuestion('How many hits did I get from ChatGPT in the last 14 days?');
        const request = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        request.stream.next({
            type: 'progress',
            status: 'generating',
            message_key: 'askAi.progress.generating'
        });
        request.stream.complete();
        fixture.detectChanges();

        expect(document.body.textContent).toContain('Ask AI could not answer right now.');
        expect(document.body.textContent).not.toContain('Checking site analytics...');
    });

    it('should not send a hardcoded Ask AI date range with the next question', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        submitAskAIQuestion('What changed in traffic?');
        const request = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');

        expect(request.request.from).toBeUndefined();
        expect(request.request.to).toBeUndefined();
    });

    it('should pass explicit route date params through to Ask AI without preset scopes', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: false
            }
        });
        vi.spyOn(TestBed.inject(Router), 'url', 'get').mockReturnValue('/dashboard?from=2026-06-01&to=2026-06-20');
        fixture.detectChanges();

        submitAskAIQuestion('Summarize the selected view');
        const request = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');

        expect(request.request.from).toBe('2026-06-01');
        expect(request.request.to).toBe('2026-06-20');
    });

    it('should focus the Ask AI drawer composer when opened from the sidebar', async () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        openAskAIDrawer();
        await waitForScheduledFocus();
        fixture.detectChanges();

        const promptInput = document.body.querySelector('input[name="ask-ai-panel-query"]') as HTMLInputElement | null;
        expect(promptInput).toBeTruthy();
        expect(document.activeElement).toBe(promptInput);
    });

    it('should fill the Ask AI prompt from browser-native dictation', () => {
        const speechWindow = (document.defaultView ?? window) as typeof window & {
            SpeechRecognition?: unknown;
            webkitSpeechRecognition?: unknown;
        };
        const originalSpeechRecognition = speechWindow.SpeechRecognition;
        const originalWebkitSpeechRecognition = speechWindow.webkitSpeechRecognition;
        const recognitionInstances: MockSpeechRecognition[] = [];
        class MockSpeechRecognition {
            continuous = false;
            interimResults = false;
            lang = '';
            onend: (() => void) | null = null;
            onerror: (() => void) | null = null;
            onresult: ((event: unknown) => void) | null = null;
            readonly start = vi.fn();
            readonly stop = vi.fn(() => this.onend?.());
            readonly abort = vi.fn();

            constructor() {
                recognitionInstances.push(this);
            }
        }
        Object.defineProperty(speechWindow, 'SpeechRecognition', {
            configurable: true,
            value: MockSpeechRecognition
        });
        Object.defineProperty(speechWindow, 'webkitSpeechRecognition', {
            configurable: true,
            value: MockSpeechRecognition
        });

        try {
            seedActiveSite();
            bootstrap.status.set({
                needs_setup: false,
                version: 'v2.0.0',
                cloud: { hosted: false, signup_enabled: false },
                ask_ai: {
                    enabled: true,
                    available: true,
                    status: 'available',
                    provider: 'bedrock',
                    model: 'openai.gpt-oss-120b-1:0',
                    budget_exhausted: false
                }
            });
            fixture.detectChanges();

            openAskAIDrawer();
            const dictateButton = document.body.querySelector('[data-testid="ask-ai-dictate"]') as HTMLButtonElement | null;
            expect(dictateButton).toBeTruthy();

            dictateButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
            fixture.detectChanges();

            const recognition = recognitionInstances[0];
            expect(recognition).toBeTruthy();
            expect(recognition?.start).toHaveBeenCalled();
            expect(dictateButton?.getAttribute('aria-label')).toBe('Stop voice dictation');

            recognition?.onresult?.({
                results: {
                    length: 1,
                    0: {
                        isFinal: true,
                        0: { transcript: ' ChatGPT hits last 14 days ' }
                    }
                }
            });
            fixture.detectChanges();

            const promptInput = dictateButton?.closest('.ask-ai-drawer')?.querySelector('input[name="ask-ai-panel-query"]') as HTMLInputElement | null;
            expect(promptInput?.value).toBe('ChatGPT hits last 14 days');

            dictateButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
            fixture.detectChanges();
            expect(recognition?.stop).toHaveBeenCalled();
            expect(dictateButton?.getAttribute('aria-label')).toBe('Start voice dictation');
        } finally {
            if (originalSpeechRecognition === undefined) {
                delete speechWindow.SpeechRecognition;
            } else {
                Object.defineProperty(speechWindow, 'SpeechRecognition', {
                    configurable: true,
                    value: originalSpeechRecognition
                });
            }
            if (originalWebkitSpeechRecognition === undefined) {
                delete speechWindow.webkitSpeechRecognition;
            } else {
                Object.defineProperty(speechWindow, 'webkitSpeechRecognition', {
                    configurable: true,
                    value: originalWebkitSpeechRecognition
                });
            }
        }
    });

    it('should render Ask AI as a distinct user and HitKeep chat thread', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'openai-compatible',
                model: 'openai.gpt-oss-120b',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        submitAskAIQuestion('What changed in traffic?');
        const request = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        completeAskAIRequest(request, {
            run_id: '11111111-1111-4111-8111-111111111111',
            answer_markdown: 'Traffic increased.',
            citations: [],
            charts: [],
            actions: []
        });
        fixture.detectChanges();

        const userMessage = document.body.querySelector('.ai-message--user') as HTMLElement | null;
        const assistantMessage = document.body.querySelector('.ai-message--assistant') as HTMLElement | null;
        const userAvatar = userMessage?.querySelector('.ai-avatar--user img') as HTMLImageElement | null;
        const hitkeepAvatar = assistantMessage?.querySelector('.ai-avatar--assistant img') as HTMLImageElement | null;

        expect(userMessage?.textContent).toContain('What changed in traffic?');
        expect(assistantMessage?.textContent).toContain('Traffic increased.');
        expect(userAvatar?.getAttribute('src') ?? '').toContain('/api/user/avatar?s=96');
        expect(hitkeepAvatar?.getAttribute('src') ?? '').toContain('/favicon.svg');
        expect(document.body.textContent).not.toContain('11111111');
        expect(document.body.textContent).not.toContain('openai.gpt-oss-120b');
    });

    it('should not expose saved Ask AI history controls in the drawer', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'openai-compatible',
                model: 'openai.gpt-oss-120b',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        openAskAIDrawer();
        fixture.detectChanges();

        expect(document.body.querySelector('[data-testid="ask-ai-history-toggle"]')).toBeNull();
        expect(document.body.querySelector('[data-testid="ask-ai-history-load-more"]')).toBeNull();
        expect(document.body.textContent).not.toContain('Run history');
        expect(document.body.textContent).not.toContain('Model');
        expect(document.body.textContent).not.toContain('openai.gpt-oss-120b');
    });

    it('should reject unsafe Ask AI navigation actions', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();
        const navigateSpy = vi.spyOn(TestBed.inject(Router), 'navigateByUrl');

        submitAskAIQuestion('Show me the page');
        const request = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        completeAskAIRequest(request, {
            run_id: '11111111-1111-4111-8111-111111111111',
            answer_markdown: 'Open this.',
            citations: [],
            charts: [],
            actions: [
                {
                    type: 'navigate',
                    label: 'Open unsafe URL',
                    target: 'https://evil.example/phish'
                }
            ]
        });
        fixture.detectChanges();

        clickAskAIAction('Open unsafe URL');
        fixture.detectChanges();

        expect(navigateSpy).not.toHaveBeenCalled();
        expect(document.body.textContent).toContain('Action could not be opened.');
        expect(document.body.textContent).toContain('Open unsafe URL');
    });

    it('should run safe Ask AI navigation actions', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();
        const navigateSpy = vi.spyOn(TestBed.inject(Router), 'navigateByUrl');

        submitAskAIQuestion('Show me events');
        const request = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        completeAskAIRequest(request, {
            run_id: '11111111-1111-4111-8111-111111111111',
            answer_markdown: 'Open events.',
            citations: [],
            charts: [],
            actions: [{ type: 'navigate', label: 'Open events', target: '/events' }]
        });
        fixture.detectChanges();

        clickAskAIAction('Open events');
        fixture.detectChanges();

        expect(navigateSpy).toHaveBeenCalledWith('/events');
        expect(document.body.textContent).not.toContain('Action could not be opened.');
    });

    it('should cancel the active Ask AI stream when starting a new chat', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        submitAskAIQuestion('What changed in traffic?');
        const request = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        expect(request.aborted).toBe(false);

        askAIControl().startNewChat();
        fixture.detectChanges();

        expect(request.aborted).toBe(true);
        completeAskAIRequest(request, {
            run_id: '11111111-1111-4111-8111-111111111111',
            answer_markdown: 'Stale answer.',
            citations: [],
            charts: [],
            actions: []
        });
        fixture.detectChanges();
        expect(document.body.textContent).not.toContain('Stale answer.');
    });

    it('should let users stop an active Ask AI response', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        submitAskAIQuestion('What changed in traffic?');
        const request = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        expect(request.aborted).toBe(false);
        expect(document.body.textContent).toContain('Stop');

        clickByTestId('ask-ai-stop');
        fixture.detectChanges();

        expect(request.aborted).toBe(true);
        expect(document.body.textContent).toContain('Response stopped.');
        expect(document.body.textContent).not.toContain('Starting request...');
        completeAskAIRequest(request, {
            run_id: '11111111-1111-4111-8111-111111111111',
            answer_markdown: 'Stale stopped answer.',
            citations: [],
            charts: [],
            actions: []
        });
        fixture.detectChanges();

        expect(document.body.textContent).not.toContain('Stale stopped answer.');
    });

    it('should show budget-specific Ask AI stream errors', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                provider: 'bedrock',
                model: 'openai.gpt-oss-120b-1:0',
                budget_exhausted: false
            }
        });
        fixture.detectChanges();

        submitAskAIQuestion('What changed in traffic?');
        const request = expectAskAIRequest('00000000-0000-0000-0000-0000000000bb');
        request.stream.error(
            new AskAIStreamStatusError(429, {
                enabled: true,
                available: false,
                status: 'budget_exhausted',
                budget_exhausted: true
            })
        );
        fixture.detectChanges();

        expect(document.body.textContent).toContain('AI budget exhausted.');
        expect(document.body.textContent).not.toContain('Ask AI could not answer right now.');
    });

    it('should hide Ask AI in share mode', () => {
        seedActiveSite();
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: false, signup_enabled: false },
            ask_ai: {
                enabled: true,
                available: true,
                status: 'available',
                budget_exhausted: false
            }
        });
        TestBed.inject(ShareService).setToken('share-token');
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('.ask-ai-trigger')).toBeNull();
    });

    it('should keep collapsible sidebar parents navigable while the chevron expands children', () => {
        let utmLink = fixture.nativeElement.querySelector('aside a[href="/utm"]') as HTMLAnchorElement | null;
        let utmBuilderLink = fixture.nativeElement.querySelector('aside a[href="/utm/builder"]') as HTMLAnchorElement | null;
        let utmTreeItem = utmLink?.closest('[role="treeitem"]') as HTMLElement | null;
        let toggle = utmTreeItem?.querySelector('button.layout-sidebar-menu__toggle') as HTMLButtonElement | null;

        expect(utmLink).toBeTruthy();
        expect(utmBuilderLink).toBeNull();
        expect(utmTreeItem?.getAttribute('aria-expanded')).toBe('false');
        expect(toggle?.getAttribute('aria-label')).toBe('Expand UTM');

        toggle?.click();
        fixture.detectChanges();

        utmLink = fixture.nativeElement.querySelector('aside a[href="/utm"]') as HTMLAnchorElement | null;
        utmBuilderLink = fixture.nativeElement.querySelector('aside a[href="/utm/builder"]') as HTMLAnchorElement | null;
        utmTreeItem = utmLink?.closest('[role="treeitem"]') as HTMLElement | null;
        toggle = utmTreeItem?.querySelector('button.layout-sidebar-menu__toggle') as HTMLButtonElement | null;

        expect(utmLink).toBeTruthy();
        expect(utmBuilderLink).toBeTruthy();
        expect(utmTreeItem?.getAttribute('aria-expanded')).toBe('true');
        expect(toggle?.getAttribute('aria-label')).toBe('Collapse UTM');
        expect(utmTreeItem?.querySelector('ul.layout-sidebar-menu__list--nested')?.getAttribute('role')).toBe('group');
    });

    it('should hide create team actions in hosted cloud for non-owners', () => {
        TestBed.inject(PermissionService).applyPermissions({
            instance_role: 'user',
            permissions: {}
        });
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: true, signup_enabled: false }
        });
        fixture.detectChanges();

        const switchers = fixture.debugElement.queryAll(By.css('app-team-switcher'));
        expect(switchers.length).toBeGreaterThan(0);
        for (const switcher of switchers) {
            expect(switcher.componentInstance.showAdd()).toBe(false);
        }
    });

    it('should show create team actions in hosted cloud when the server grants team creation', () => {
        TestBed.inject(PermissionService).applyPermissions({
            instance_role: 'owner',
            permissions: {},
            can_create_teams: true
        });
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: true, signup_enabled: false }
        });
        fixture.detectChanges();

        const switchers = fixture.debugElement.queryAll(By.css('app-team-switcher'));
        expect(switchers.length).toBeGreaterThan(0);
        for (const switcher of switchers) {
            expect(switcher.componentInstance.showAdd()).toBe(true);
        }
    });

    it('should show docs link in oss mode and hide support link', () => {
        const links = Array.from(fixture.nativeElement.querySelectorAll('a[href]')) as HTMLAnchorElement[];

        const docsLink = links.find((link) => link.href === 'https://hitkeep.com/guides/introduction/');
        const supportLink = links.find((link) => link.href === 'https://hitkeep.com/support/help/');

        expect(docsLink).toBeTruthy();
        expect(docsLink?.querySelector('.pi-external-link')).toBeTruthy();
        expect(supportLink).toBeFalsy();
    });

    it('should show support link in hosted cloud mode', () => {
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: {
                hosted: true,
                signup_enabled: false,
                support_url: 'https://hitkeep.com/support/help/'
            }
        });
        fixture.detectChanges();

        const links = Array.from(fixture.nativeElement.querySelectorAll('a[href]')) as HTMLAnchorElement[];
        const supportLink = links.find((link) => link.href === 'https://hitkeep.com/support/help/');

        expect(supportLink).toBeTruthy();
        expect(supportLink?.querySelector('.pi-headphones')).toBeTruthy();
        expect(supportLink?.querySelector('.pi-external-link')).toBeTruthy();
    });

    it('allows team switching without the retired drawer confirmation', () => {
        const confirmSpy = vi.spyOn(window, 'confirm');
        const result = layoutContext.beforeTeamSwitch();
        expect(result).toBe(true);
        expect(confirmSpy).not.toHaveBeenCalled();
    });

    it('should open site settings from keyboard shortcut when an active site exists', () => {
        const site = seedActiveSite();
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        const event = new KeyboardEvent('keydown', { key: 'k', metaKey: true });
        const preventDefault = vi.spyOn(event, 'preventDefault');

        component.handleKeyboard(event);

        expect(preventDefault).toHaveBeenCalled();
        expect(navigate).toHaveBeenCalledWith(['/sites', site.id, 'settings', 'general']);
    });

    it('should handle the document keyboard shortcut binding and ctrl-key variant', () => {
        const site = seedActiveSite();
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        const event = new KeyboardEvent('keydown', { key: 'k', ctrlKey: true });
        const preventDefault = vi.spyOn(event, 'preventDefault');

        document.dispatchEvent(event);

        expect(preventDefault).toHaveBeenCalled();
        expect(navigate).toHaveBeenCalledWith(['/sites', site.id, 'settings', 'general']);
    });

    it('should ignore unrelated keyboard shortcuts', () => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        const event = new KeyboardEvent('keydown', { key: 'x', metaKey: true });
        const preventDefault = vi.spyOn(event, 'preventDefault');

        component.handleKeyboard(event);

        expect(preventDefault).not.toHaveBeenCalled();
        expect(navigate).not.toHaveBeenCalled();
    });

    it('should keep sidebar drawer actions inside the sidebar component', () => {
        const site = seedActiveSite();
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        const sidebar = fixture.debugElement.query(By.directive(LayoutSidebar)).componentInstance as LayoutSidebarTestAccess;
        layoutContext.isMobileDrawerOpen.set(true);

        sidebar.openSiteSettings();
        sidebar.closeMobileDrawer();
        const menuItems = sidebar.mobileMenuItems();
        const firstItem = menuItems[0]?.items?.[0];
        firstItem?.command?.({
            originalEvent: new Event('click'),
            item: firstItem
        });

        expect(navigate).toHaveBeenCalledWith(['/sites', site.id, 'settings', 'general']);
        expect(layoutContext.isMobileDrawerOpen()).toBe(false);
        expect(menuItems.length).toBeGreaterThan(0);
        expect(sidebar.canCreateTeams()).toBe(true);
    });

    it('keeps the current settings section when selecting another site', () => {
        const siteService = TestBed.inject(SiteService);
        const firstSite = seedActiveSite();
        const secondSite = { ...firstSite, id: '00000000-0000-0000-0000-0000000000cc', domain: 'next.example.com' };
        siteService.applySites([firstSite, secondSite]);
        siteService.selectSite(firstSite);
        const router = TestBed.inject(Router);
        vi.spyOn(router, 'url', 'get').mockReturnValue(`/sites/${firstSite.id}/settings/access`);
        const navigate = vi.spyOn(router, 'navigate').mockResolvedValue(true);

        layoutContext.onSiteSelected(secondSite);

        expect(siteService.activeSite()).toEqual(secondSite);
        expect(navigate).toHaveBeenCalledWith(['/sites', secondSite.id, 'settings', 'access']);
    });

    it('returns to the team overview before switching team context', () => {
        const router = TestBed.inject(Router);
        const navigate = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        const teamService = TestBed.inject(TeamService);
        vi.spyOn(teamService, 'setActiveTeam').mockReturnValue(of({ status: 'ok', active_team_id: 'team-2' }));
        vi.spyOn(teamService, 'loadTeams').mockReturnValue(of({ teams: [], active_team_id: 'team-2' }));
        vi.spyOn(TestBed.inject(SiteService), 'loadSites').mockImplementation(() => undefined);
        vi.spyOn(TestBed.inject(PermissionService), 'loadPermissions').mockReturnValue(of({ instance_role: 'user', permissions: {} }));

        layoutContext.onTeamSelected({
            id: 'team-2',
            name: 'Second team',
            logo_url: '',
            role: 'owner',
            created_at: '2026-01-01T00:00:00Z'
        });

        expect(navigate).toHaveBeenCalledWith(['/overview']);
    });

    it('should keep the mobile OptimusUI menu model stable between change detection passes', () => {
        const sidebar = fixture.debugElement.query(By.directive(LayoutSidebar)).componentInstance as LayoutSidebarTestAccess;

        const firstItems = sidebar.mobileMenuItems();
        fixture.detectChanges();
        const secondItems = sidebar.mobileMenuItems();

        expect(secondItems).toBe(firstItems);
    });

    it('should derive page-bar team creation affordance from cloud mode and the server permission flag', () => {
        const pageBar = fixture.debugElement.query(By.directive(LayoutPageBar)).componentInstance as LayoutPageBarTestAccess;

        // Self-hosted deployments have no plan limits.
        expect(pageBar.canCreateTeams()).toBe(true);

        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: true, signup_enabled: false }
        });

        // Hosted cloud without the server-derived flag stays hidden.
        expect(pageBar.canCreateTeams()).toBe(false);

        TestBed.inject(PermissionService).applyPermissions({
            instance_role: 'user',
            permissions: {},
            can_create_teams: true
        });

        expect(pageBar.canCreateTeams()).toBe(true);

        TestBed.inject(PermissionService).applyPermissions({
            instance_role: 'user',
            permissions: {},
            can_create_teams: false
        });

        expect(pageBar.canCreateTeams()).toBe(false);
    });

    function seedLayoutState() {
        const teamService = TestBed.inject(TeamService);
        const permissions = TestBed.inject(PermissionService);
        const profile = TestBed.inject(UserProfileService);

        teamService.applyTeams({
            active_team_id: '00000000-0000-0000-0000-000000000001',
            teams: [
                {
                    id: '00000000-0000-0000-0000-000000000001',
                    name: 'Alpha Team',
                    logo_url: '',
                    role: 'owner',
                    created_at: '2026-01-01T00:00:00Z'
                },
                {
                    id: '00000000-0000-0000-0000-000000000002',

                    name: 'Beta Team',
                    logo_url: '',
                    role: 'admin',
                    created_at: '2026-01-02T00:00:00Z'
                }
            ]
        });

        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: {
                hosted: false,
                signup_enabled: false
            }
        });

        permissions.applyPermissions({
            instance_role: 'owner',
            permissions: {}
        });

        profile.applyProfile({
            id: '00000000-0000-0000-0000-000000000001',
            email: 'demo@example.com',
            display_name: 'Demo User',
            avatar_url: '/api/user/avatar?s=96'
        });
    }

    function seedActiveSite() {
        const site = {
            id: '00000000-0000-0000-0000-0000000000bb',
            user_id: '00000000-0000-0000-0000-000000000001',
            domain: 'active.example.com',
            created_at: '2026-01-01T00:00:00Z'
        };
        TestBed.inject(SiteService).applySites([site]);
        return site;
    }

    function submitAskAIQuestion(question: string) {
        const control = askAIControl();
        control.query.set(question);
        fixture.detectChanges();
        control.submit(new Event('submit', { cancelable: true }));
        fixture.detectChanges();
    }

    function openAskAIDrawer() {
        const trigger = fixture.nativeElement.querySelector('.ask-ai-trigger') as HTMLElement | null;
        expect(trigger).toBeTruthy();
        trigger?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
        fixture.detectChanges();
    }

    function waitForScheduledFocus() {
        return new Promise((resolve) => setTimeout(resolve, 0));
    }

    function expectAskAIRequest(siteId: string): AskAIStreamRequest {
        const request = askAI.requests[askAI.requests.length - 1];
        expect(request).toBeTruthy();
        expect(request.siteId).toBe(siteId);
        return request;
    }

    function completeAskAIRequest(request: AskAIStreamRequest, response: AskAIResponse) {
        request.stream.next({ type: 'final', status: 'success', response });
        request.stream.complete();
    }

    function clickByTestId(testId: string) {
        const host = document.body.querySelector(`[data-testid="${testId}"]`) as HTMLElement | null;
        expect(host).toBeTruthy();
        (host?.querySelector('button') ?? host)?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    }

    function clickAskAIAction(label: string) {
        const buttons = Array.from(document.body.querySelectorAll('button')) as HTMLButtonElement[];
        const button = buttons.find((candidate) => candidate.textContent?.includes(label));
        expect(button).toBeTruthy();
        button?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    }

    function askAIControl() {
        const control = fixture.debugElement.query(By.directive(AskAIControl))?.componentInstance as
            | (AskAIControl & {
                  query: { set(value: string): void };
                  submit(event?: Event): void;
                  startNewChat(): void;
              })
            | undefined;
        expect(control).toBeTruthy();
        return control!;
    }
});
