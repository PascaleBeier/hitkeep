import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { MenuItem } from '@openng/optimus-ui/api';
import { INSTANCE_CAPABILITIES, TEAM_CAPABILITIES } from '@core/access/capabilities';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { PermissionService } from '@services/permission.service';
import { ShareService } from '@services/share.service';
import { SidebarMenuService } from './sidebar-menu.service';

describe('SidebarMenuService', () => {
    let service: SidebarMenuService;
    let permissions: PermissionService;
    let share: ShareService;
    let bootstrap: DashboardBootstrapService;

    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: {
                                analytics: 'Analytics',
                                overview: 'Overview',
                                dashboard: 'Dashboard',
                                opportunities: 'Opportunities',
                                goals: 'Goals',
                                funnels: 'Funnels',
                                events: 'Events',
                                webVitals: 'Web Vitals',
                                aiAgents: 'AI Agents',
                                aiChatbots: 'Chatbots',
                                ecommerce: 'Ecommerce',
                                utm: 'UTM',
                                utmBuilder: 'UTM Builder',
                                qrCodes: 'QR codes',
                                importExport: 'Import & Export',
                                integration: 'Integration',
                                apiClients: 'API Clients',
                                apiReference: 'API Reference',
                                googleSearchConsole: 'Google Search Console',
                                account: 'Account',
                                emailReports: 'Email Reports',
                                resources: 'Resources',
                                docs: 'Docs',
                                support: 'Support',
                                administration: 'Administration',
                                systemStatus: 'System Status',
                                systemSettings: 'System Settings',
                                team: 'Team'
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ],
            providers: [SidebarMenuService, provideRouter([])]
        });
        service = TestBed.inject(SidebarMenuService);
        permissions = TestBed.inject(PermissionService);
        share = TestBed.inject(ShareService);
        bootstrap = TestBed.inject(DashboardBootstrapService);
    });

    it('builds capability-filtered desktop menu sections', () => {
        permissions.applyPermissions({
            instance_role: 'admin',
            permissions: {},
            instance_capabilities: [INSTANCE_CAPABILITIES.viewSystem, INSTANCE_CAPABILITIES.manageUsers],
            active_team_role: 'admin',
            active_team_capabilities: [TEAM_CAPABILITIES.manageSettings, TEAM_CAPABILITIES.manageIntegrations]
        });
        bootstrap.status.set({ needs_setup: false, setup_complete: true, version: 'test', cloud: { hosted: true, support_url: 'https://support.example.test' } } as never);

        const items = service.desktopItems();
        const labels = flattenLabels(items);

        expect(labels).toContain('Google Search Console');
        expect(labels).toContain('System Status');
        expect(labels).toContain('System Settings');
        expect(labels).toContain('Team');
        expect(findByLabel(items, 'Support')?.url).toBe('https://support.example.test');
    });

    it('nests related utility pages under collapsed parent menu items', () => {
        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {},
            active_team_role: 'member',
            active_team_capabilities: []
        });

        const items = service.desktopItems();
        const analytics = findByLabel(items, 'Analytics');
        const integration = findByLabel(items, 'Integration');
        const utm = findByLabel(items, 'UTM');
        const utmBuilder = findByLabel(utm?.items ?? [], 'UTM Builder');
        const qrCodes = findByLabel(utm?.items ?? [], 'QR codes');
        const apiClients = findByLabel(items, 'API Clients');
        const apiReference = findByLabel(apiClients?.items ?? [], 'API Reference');

        expect(analytics?.expanded).toBe(true);
        expect(integration?.expanded).toBe(true);
        expect(findByLabel(items, 'Overview')?.routerLink).toBe('/overview');
        expect(findByLabel(items, 'AI Agents')?.routerLink).toBe('/ai-agents');
        expect(utm?.routerLink).toBe('/utm');
        expect(utm?.expanded).toBe(false);
        expect(utmBuilder?.routerLink).toBe('/utm/builder');
        expect(qrCodes?.routerLink).toBe('/utm/qr-codes');
        expect(apiClients?.routerLink).toBe('/integration/api-clients');
        expect(apiClients?.expanded).toBe(false);
        expect(apiReference?.routerLink).toBe('/integration/api-reference');
    });

    it('does not expose system settings to users who can only view system status', () => {
        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {},
            instance_capabilities: [INSTANCE_CAPABILITIES.viewSystem],
            active_team_role: 'member',
            active_team_capabilities: []
        });

        const labels = flattenLabels(service.desktopItems());

        expect(labels).toContain('System Status');
        expect(labels).not.toContain('System Settings');
    });

    it('uses the public support fallback for hosted cloud without a custom support URL', () => {
        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {},
            active_team_role: 'member',
            active_team_capabilities: []
        });
        bootstrap.status.set({ needs_setup: false, setup_complete: true, version: 'test', cloud: { hosted: true, support_url: ' ' } } as never);

        expect(findByLabel(service.desktopItems(), 'Support')?.url).toBe('https://hitkeep.com/support/help/');
    });

    it('hides admin, integration setup, and support when capabilities or cloud support are absent', () => {
        permissions.applyPermissions({
            instance_role: 'user',
            permissions: {},
            active_team_role: 'member',
            active_team_capabilities: []
        });
        bootstrap.status.set({ needs_setup: false, setup_complete: true, version: 'test', cloud: { hosted: false } } as never);

        const labels = flattenLabels(service.desktopItems());

        expect(labels).not.toContain('Google Search Console');
        expect(labels).not.toContain('Administration');
        expect(labels).not.toContain('Support');
    });

    it('rewrites share-aware links and closes the mobile menu from commands', () => {
        share.setToken('share-token');
        let closed = false;

        const items = service.mobileItems(() => {
            closed = true;
        });
        const analytics = findByLabel(items, 'Analytics');
        const analyticsItems = analytics?.items ?? [];
        const dashboard = findByLabel(items, 'Dashboard');
        const utm = analyticsItems.find((item) => item.label === 'UTM');
        const webVitals = findByLabel(items, 'Web Vitals');
        const utmBuilder = findByLabel(items, 'UTM Builder');
        const qrCodes = analyticsItems.find((item) => item.label === 'QR codes');

        expect(dashboard?.routerLink).toBe('/share/share-token/dashboard');
        expect(webVitals?.routerLink).toBe('/share/share-token/web-vitals');
        expect(findByLabel(items, 'AI Agents')?.routerLink).toBe('/share/share-token/ai-agents');
        expect(utm?.routerLink).toBe('/share/share-token/utm');
        expect(utm?.items).toBeUndefined();
        expect(utmBuilder).toBeUndefined();
        expect(qrCodes?.routerLink).toBe('/share/share-token/utm/qr-codes');

        dashboard?.command?.({ originalEvent: new Event('click'), item: dashboard });
        expect(closed).toBe(true);
    });
});

function flattenLabels(items: MenuItem[]): string[] {
    return items.flatMap((item) => [item.label, ...flattenLabels(item.items ?? [])]).filter((label): label is string => !!label);
}

function findByLabel(items: MenuItem[], label: string): MenuItem | undefined {
    for (const item of items) {
        if (item.label === label) {
            return item;
        }
        const child = findByLabel(item.items ?? [], label);
        if (child) {
            return child;
        }
    }
    return undefined;
}
