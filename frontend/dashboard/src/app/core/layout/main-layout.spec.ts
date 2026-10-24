import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { MainLayout } from '@layout/main-layout';
import { NavigationEnd, Router, provideRouter } from '@angular/router';
import { By } from '@angular/platform-browser';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { Observable, Subject, of } from 'rxjs';
import { AskAIRequest, AskAIResponse, AskAIStatus, AskAIStreamEvent, Team } from '@models/analytics.types';
import { AskAIService, AskAIStreamStatusError } from '@services/ask-ai.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { NavigationNoticeService } from '@services/navigation-notice.service';
import { PermissionService } from '@services/permission.service';
import { ShareService } from '@services/share.service';
import { TeamService } from '@services/team.service';
import { UserProfileService } from '@services/user-profile.service';
import { UserMenuService } from '@services/user-menu.service';
import { SiteService } from '@features/sites/services/site.service';
import { AskAIControl } from '@features/ask-ai/ask-ai-control';
import { AskAIControlService } from '@features/ask-ai/ask-ai-control.service';
import { LayoutPageBar } from './layout-page-bar';
import { LayoutSidebar } from './layout-sidebar';
import { MainLayoutContextService } from './main-layout-context.service';
import { MenuItem } from '@openng/optimus-ui/api';
import { vi } from 'vitest';

interface LayoutSidebarTestAccess {
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

    getStatus(): Observable<AskAIStatus> {
        return of(TestBed.inject(DashboardBootstrapService).status()?.ask_ai as AskAIStatus);
    }

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

@Component({ selector: 'app-layout-title-probe', template: '' })
class LayoutTitleProbe {}

describe('MainLayout', () => {
    let component: MainLayout;
    let fixture: ComponentFixture<MainLayout>;
    let httpMock: HttpTestingController;
    let bootstrap: DashboardBootstrapService;
    let layoutContext: MainLayoutContextService;
    let askAI: FakeAskAIService;
    let existingDrawerMasks: Set<Element>;

    beforeEach(async () => {
        existingDrawerMasks = new Set(document.querySelectorAll('.p-drawer-mask'));
        await TestBed.configureTestingModule({
            imports: [
                MainLayout,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: {
                                resizeSidebar: 'Resize sidebar',
                                dashboard: 'Dashboard',
                                overview: 'Overview',
                                utm: 'UTM',
                                utmBuilder: 'UTM Builder',
                                qrCodes: 'QR codes',
                                integration: 'Integration',
                                apiClients: 'API Clients',
                                apiReference: 'API Reference',
                                account: 'Account',
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
                                budget: {
                                    trigger: 'Ask AI paused',
                                    title: 'AI usage limit reached',
                                    description: 'This instance has reached its configured AI request or token allowance.',
                                    operator: 'Review AI usage in System Status.',
                                    askAdmin: 'Ask your instance operator to review AI usage.',
                                    statusAction: 'Open System Status',
                                    guide: 'AI setup and limits'
                                },
                                quota: { refresh: 'Check availability' },
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
        fixture.destroy();
        // Destroying a fixture aborts native exit motion before Drawer can remove its body mask.
        document.querySelectorAll('.p-drawer-mask').forEach((mask) => {
            if (!existingDrawerMasks.has(mask)) mask.remove();
        });
        httpMock.verify();
        vi.restoreAllMocks();
        localStorage.removeItem('hk_sidebar_layout');
        localStorage.removeItem('hk_sidebar_sections');
    });

    it('bounds sidebar width and restores it after collapsing without replacing the outlet', async () => {
        const outlet = fixture.nativeElement.querySelector('router-outlet');
        expect(layoutContext.sidebarWidthPx()).toBe(240);
        layoutContext.setSidebarWidthPx(900);
        expect(layoutContext.sidebarWidthPx()).toBe(320);
        layoutContext.setSidebarWidthPx(120);
        expect(layoutContext.sidebarWidthPx()).toBe(220);
        layoutContext.setSidebarWidthPx(280);
        layoutContext.toggleSidebar();
        await fixture.whenStable();
        expect(layoutContext.isSidebarCollapsed()).toBe(true);
        expect(fixture.nativeElement.querySelector('router-outlet')).toBe(outlet);
        layoutContext.isMobileDrawerOpen.set(true);
        layoutContext.isMobileDrawerOpen.set(false);
        layoutContext.toggleSidebar();
        await fixture.whenStable();
        expect(layoutContext.sidebarWidthPx()).toBe(280);
        expect(fixture.nativeElement.querySelector('router-outlet')).toBe(outlet);
        expect(JSON.parse(localStorage.getItem('hk_sidebar_layout')!)).toEqual({ version: 1, collapsed: false, widthPx: 280 });
    });

    it('restores valid preferences and safely defaults malformed, obsolete and blocked storage', () => {
        for (const [record, width, collapsed] of [
            ['{"version":1,"widthPx":300,"collapsed":true}', 300, true],
            ['{"version":1,"widthPx":900,"collapsed":false}', 320, false],
            ['{"version":2,"widthPx":300,"collapsed":true}', 240, false],
            ['{"version":1,"widthPx":"wide","collapsed":"yes"}', 240, false],
            ['{broken', 240, false]
        ] as const) {
            localStorage.setItem('hk_sidebar_layout', record);
            const other = TestBed.createComponent(MainLayout);
            const context = other.debugElement.injector.get(MainLayoutContextService);
            expect(context.sidebarWidthPx()).toBe(width);
            expect(context.isSidebarCollapsed()).toBe(collapsed);
            other.destroy();
        }
        vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
            throw new Error('blocked');
        });
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
            throw new Error('blocked');
        });
        const other = TestBed.createComponent(MainLayout);
        const context = other.debugElement.injector.get(MainLayoutContextService);
        context.setSidebarWidthPx(Number.NaN);
        expect(context.sidebarWidthPx()).toBe(240);
        context.toggleSidebar();
        expect(context.isSidebarCollapsed()).toBe(true);
        other.destroy();
    });

    it('renders the active route title as one page heading in the content area', async () => {
        const router = TestBed.inject(Router);
        router.resetConfig([{ path: 'dashboard', component: LayoutTitleProbe, data: { titleKey: 'nav.dashboard', titleScope: 'site' } }]);
        await router.navigateByUrl('/dashboard');
        await fixture.whenStable();
        const headings = fixture.nativeElement.querySelectorAll('main h1');
        expect(headings).toHaveLength(1);
        expect(headings[0].textContent).toContain('Dashboard');
    });

    it('offers contextual site actions under the existing permission and share boundaries', () => {
        const site = seedActiveSite();
        expect(layoutContext.siteActions().map((item) => item.icon)).toEqual(['pi pi-cog', 'pi pi-share-alt', 'pi pi-plus']);
        layoutContext.onSiteSelected({ ...site, id: 'removed-site' });
        expect(TestBed.inject(SiteService).activeSite()?.id).toBe(site.id);
        TestBed.inject(PermissionService).applyPermissions({ instance_role: 'user', permissions: { [site.id]: 'viewer' } });
        expect(layoutContext.siteActions().some((item) => item.icon === 'pi pi-share-alt')).toBe(false);
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        layoutContext.openSiteSettings('share');
        expect(navigate).not.toHaveBeenCalled();
        TestBed.inject(ShareService).setToken('public-token');
        expect(layoutContext.siteActions()).toEqual([]);
        layoutContext.openAddSite();
        expect(layoutContext.isAddSiteVisible()).toBe(false);
    });

    it('exposes the resize handle as a named keyboard separator with pixel bounds', async () => {
        await fixture.whenStable();
        const handle = fixture.nativeElement.querySelector('[role="separator"][aria-label="Resize sidebar"]');
        expect(handle).toBeTruthy();
        expect(handle?.getAttribute('aria-valuemin')).toBe('220');
        expect(handle?.getAttribute('aria-valuemax')).toBe('320');
        expect(handle?.getAttribute('aria-orientation')).toBe('vertical');
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
        expect(visibleHrefs).not.toContain('/share/share-token/utm/qr-codes');
    });

    it('keeps readable team and site labels in the narrow global context', async () => {
        seedActiveSite();
        fixture.nativeElement.style.width = '320px';
        await fixture.whenStable();
        const teamLabel = fixture.nativeElement.querySelector('app-team-switcher .p-select-label .truncate') as HTMLElement;
        const siteLabel = fixture.nativeElement.querySelector('app-site-selector .site-select-option__domain') as HTMLElement;
        await expect.poll(() => teamLabel.getBoundingClientRect().width).toBeGreaterThan(64);
        expect(siteLabel.getBoundingClientRect().width).toBeGreaterThan(64);
    });

    it('should hide Ask AI by default', () => {
        seedActiveSite();
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('.ask-ai-trigger')).toBeNull();
    });

    it('opens Ask AI directly from the persistent header when available', async () => {
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

        const menuItems = TestBed.inject(UserMenuService).menuItems();
        const askAIItem = menuItems.find((item) => item.icon === 'pi pi-sparkles');
        expect(askAIItem).toBeUndefined();
        const trigger = fixture.nativeElement.querySelector('.ask-ai-trigger') as HTMLButtonElement;
        expect(trigger?.textContent).toContain('Ask AI');
        trigger.click();
        await fixture.whenStable();
        expect(document.body.querySelector('#ask-ai-drawer')).toBeTruthy();
    });

    it('closes Ask AI on entering a shared scope and requires a new explicit request afterward', async () => {
        seedActiveSite();
        bootstrap.status.set({ needs_setup: false, version: 'test', ask_ai: { enabled: true, available: true, status: 'available', budget_exhausted: false } });
        fixture.detectChanges();
        openAskAIDrawer();
        await fixture.whenStable();
        expect(document.body.querySelector('#ask-ai-drawer')?.getAttribute('data-p-open')).toBe('true');

        const share = TestBed.inject(ShareService);
        share.setToken('shared-site');
        await fixture.whenStable();
        expect(document.body.querySelector('#ask-ai-drawer')?.getAttribute('data-p-open')).not.toBe('true');
        share.clear();
        await fixture.whenStable();
        expect(document.body.querySelector('#ask-ai-drawer')?.getAttribute('data-p-open')).not.toBe('true');
        openAskAIDrawer();
        await fixture.whenStable();
        expect(document.body.querySelector('#ask-ai-drawer')?.getAttribute('data-p-open')).toBe('true');
    });

    it('keeps direct Ask AI access when enabled without a configured model', () => {
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

        const menuItems = TestBed.inject(UserMenuService).menuItems();
        expect(menuItems.some((item) => item.icon === 'pi pi-sparkles')).toBe(false);
        expect(fixture.nativeElement.querySelector('.ask-ai-trigger')).toBeTruthy();

        openAskAIDrawer();
        fixture.detectChanges();

        expect(document.body.textContent).toContain('Ask AI not configured');
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
        expect(document.body.textContent).toContain('AI usage limit reached');
        expect(document.body.textContent).toContain('configured AI request or token allowance');
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

    it('should focus the Ask AI drawer composer when opened from the user menu', async () => {
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

        const promptInput = document.body.querySelector('textarea[name="ask-ai-panel-query"]') as HTMLTextAreaElement | null;
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

            const promptInput = dictateButton?.closest('.ask-ai-drawer')?.querySelector('textarea[name="ask-ai-panel-query"]') as HTMLTextAreaElement | null;
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
        expect(userAvatar).toBeNull();
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

    it('keeps multiple sections open and persists each independent choice', () => {
        const integrationToggle = fixture.nativeElement.querySelector('aside .layout-sidebar-menu__section-toggle[aria-label="Integration"][aria-expanded="false"]') as HTMLButtonElement | null;
        integrationToggle?.click();
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('aside .layout-sidebar-menu__section-toggle[aria-label="Integration"][aria-expanded="true"]')).toBeTruthy();
        expect(fixture.nativeElement.querySelector('aside a[href="/integration/api-clients"]')).toBeTruthy();
        expect(fixture.nativeElement.querySelector('aside a[href="/integration/api-reference"]')).toBeNull();

        const accountToggle = fixture.nativeElement.querySelector('aside .layout-sidebar-menu__section-toggle[aria-label="Account"][aria-expanded="false"]') as HTMLButtonElement | null;
        accountToggle?.click();
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('aside .layout-sidebar-menu__section-toggle[aria-label="Integration"][aria-expanded="true"]')).toBeTruthy();
        expect(fixture.nativeElement.querySelector('aside .layout-sidebar-menu__section-toggle[aria-label="Account"][aria-expanded="true"]')).toBeTruthy();

        expect(JSON.parse(localStorage.getItem('hk_sidebar_sections')!)).toMatchObject({ 's:integration': true, 's:account': true });

        localStorage.removeItem('hk_sidebar_sections');
    });

    it('should fold a route-active sidebar section when its toggle is clicked', () => {
        vi.spyOn(TestBed.inject(Router), 'url', 'get').mockReturnValue('/integration/api-clients');
        const localFixture = TestBed.createComponent(MainLayout);
        localFixture.detectChanges();

        try {
            const sectionToggle = localFixture.nativeElement.querySelector('aside .layout-sidebar-menu__section-toggle[aria-label="Integration"][aria-expanded="true"]') as HTMLButtonElement | null;
            const apiClientsLink = localFixture.nativeElement.querySelector('aside a[href="/integration/api-clients"]') as HTMLAnchorElement | null;

            expect(sectionToggle).toBeTruthy();
            expect(apiClientsLink).toBeTruthy();

            sectionToggle?.click();
            localFixture.detectChanges();

            expect(localFixture.nativeElement.querySelector('aside .layout-sidebar-menu__section-toggle[aria-label="Integration"][aria-expanded="false"]')).toBeTruthy();
            expect(sectionToggle?.getAttribute('aria-expanded')).toBe('false');
        } finally {
            localStorage.removeItem('hk_sidebar_sections');
            localFixture.destroy();
        }
    });

    it('restores independent groups and reveals a closed group only on a new route path', async () => {
        localStorage.setItem('hk_sidebar_sections', JSON.stringify({ 's:integration': false, 's:resources': true, unknown: true }));
        const router = TestBed.inject(Router);
        const events = new Subject<NavigationEnd>();
        vi.spyOn(router, 'events', 'get').mockReturnValue(events);
        vi.spyOn(router, 'url', 'get').mockReturnValue('/integration/api-clients');
        const other = TestBed.createComponent(MainLayout);
        await other.whenStable();
        const header = (label: string) => other.nativeElement.querySelector(`aside [role="button"][aria-label="${label}"]`) as HTMLElement;
        expect(header('Integration').getAttribute('aria-expanded')).toBe('true');
        const integration = header('Integration');
        integration.click();
        await other.whenStable();
        expect(header('Integration')).toBe(integration);
        expect(integration.getAttribute('aria-expanded')).toBe('false');
        events.next(new NavigationEnd(1, '/integration/api-clients?tab=reference', '/integration/api-clients?tab=reference'));
        await other.whenStable();
        expect(header('Integration').getAttribute('aria-expanded')).toBe('false');
        events.next(new NavigationEnd(2, '/settings/reports', '/settings/reports'));
        await other.whenStable();
        expect(header('Account').getAttribute('aria-expanded')).toBe('true');
        events.next(new NavigationEnd(3, '/integration/api-clients', '/integration/api-clients'));
        await other.whenStable();
        expect(header('Integration').getAttribute('aria-expanded')).toBe('true');
        expect(JSON.parse(localStorage.getItem('hk_sidebar_sections')!)).toMatchObject({ 's:resources': true, 's:account': true, 's:integration': true });
        expect(JSON.parse(localStorage.getItem('hk_sidebar_sections')!)).not.toHaveProperty('unknown');
        other.destroy();
    });

    it('keeps global site and team context on Overview with collapsed navigation', () => {
        vi.spyOn(TestBed.inject(Router), 'url', 'get').mockReturnValue('/overview');
        const localFixture = TestBed.createComponent(MainLayout);
        localFixture.detectChanges();

        const context = localFixture.debugElement.injector.get(MainLayoutContextService);
        context.toggleSidebar();
        localFixture.detectChanges();
        expect(localFixture.nativeElement.querySelector('app-layout-page-bar app-site-selector')).toBeTruthy();
        expect(localFixture.nativeElement.querySelectorAll('app-team-switcher')).toHaveLength(1);
        expect(localFixture.nativeElement.querySelector('app-layout-mobile-header')).toBeNull();
        localFixture.destroy();
    });

    it('keeps the route-active section open when another section is opened', () => {
        vi.spyOn(TestBed.inject(Router), 'url', 'get').mockReturnValue('/integration/api-clients');
        const localFixture = TestBed.createComponent(MainLayout);
        localFixture.detectChanges();

        try {
            expect(localFixture.nativeElement.querySelector('aside .layout-sidebar-menu__section-toggle[aria-label="Integration"][aria-expanded="true"]')).toBeTruthy();

            const accountToggle = localFixture.nativeElement.querySelector('aside .layout-sidebar-menu__section-toggle[aria-label="Account"][aria-expanded="false"]') as HTMLButtonElement | null;
            accountToggle?.click();
            localFixture.detectChanges();

            expect(localFixture.nativeElement.querySelector('aside .layout-sidebar-menu__section-toggle[aria-label="Integration"][aria-expanded="true"]')).toBeTruthy();
        } finally {
            localStorage.removeItem('hk_sidebar_sections');
            localFixture.destroy();
        }
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

    it('should keep sidebar drawer actions inside the sidebar component', () => {
        const site = seedActiveSite();
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        const sidebar = fixture.debugElement.query(By.directive(LayoutSidebar)).componentInstance as LayoutSidebarTestAccess;
        layoutContext.isMobileDrawerOpen.set(true);

        layoutContext.openSiteSettings('general');
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

    it('selects a site from Overview and opens its dashboard', () => {
        const site = seedActiveSite();
        const router = TestBed.inject(Router);
        vi.spyOn(router, 'url', 'get').mockReturnValue('/overview');
        const navigate = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        layoutContext.onSiteSelected(site);
        expect(TestBed.inject(SiteService).activeSite()).toEqual(site);
        expect(navigate).toHaveBeenCalledWith(['/dashboard']);
    });

    it('rejects duplicate team switches and stale site actions, then recovers after failure', () => {
        const site = seedActiveSite();
        const teams = TestBed.inject(TeamService);
        const previousTeam = teams.activeTeamId();
        const next: Team = { id: 'team-2', name: 'Second team', logo_url: '', role: 'owner', created_at: '2026-01-01T00:00:00Z' };
        teams.teams.update((items) => [...items, next]);
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        layoutContext.onTeamSelected(next);
        layoutContext.onTeamSelected(next);
        expect(layoutContext.isTeamSwitchPending()).toBe(true);
        layoutContext.onSiteSelected({ ...site, id: 'stale-site' });
        layoutContext.openSiteSettings();
        expect(TestBed.inject(SiteService).activeSite()).toEqual(site);
        expect(navigate).toHaveBeenCalledTimes(1);
        const request = httpMock.expectOne('/api/user/teams/active');
        request.flush({}, { status: 500, statusText: 'Server Error' });
        expect(teams.activeTeamId()).toBe(previousTeam);
        expect(layoutContext.isTeamSwitchPending()).toBe(false);
        expect(TestBed.inject(NavigationNoticeService).key()).toBe('teams.switcher.switchError');
    });

    it('discards old permissions after a committed team switch even when permission refresh fails', () => {
        seedActiveSite();
        const teams = TestBed.inject(TeamService);
        const next = teams.teams()[1];
        const permissions = TestBed.inject(PermissionService);
        vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        layoutContext.onTeamSelected(next);
        httpMock.expectOne('/api/user/teams/active').flush({ active_team_id: next.id });
        httpMock.expectOne('/api/user/teams').flush({ teams: teams.teams(), active_team_id: next.id });
        httpMock.expectOne('/api/user/permissions').flush({}, { status: 500, statusText: 'Server Error' });
        httpMock.expectOne('/api/sites').flush([]);
        expect(teams.activeTeamId()).toBe(next.id);
        expect(permissions.permissions()).toBeNull();
        expect(layoutContext.isTeamAdmin()).toBe(false);
        expect(layoutContext.isContextPending()).toBe(false);
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
        TestBed.inject(AskAIControlService).requestOpen();
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
