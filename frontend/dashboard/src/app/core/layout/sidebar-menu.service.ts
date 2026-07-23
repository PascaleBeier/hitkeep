import { Injectable, computed, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router } from '@angular/router';
import { TranslocoService } from '@jsverse/transloco';
import { MenuItem } from '@openng/optimus-ui/api';
import { filter, map, startWith } from 'rxjs';
import { INSTANCE_CAPABILITIES, SITE_CAPABILITIES, TEAM_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { ShareService } from '@services/share.service';

interface SidebarItem {
    labelKey: string;
    icon: string;
    routerLink?: string;
    shareRouterLink?: string;
    url?: string;
    target?: string;
    exact?: boolean;
    visible?: () => boolean;
    items?: SidebarItem[];
}

interface SidebarSection {
    labelKey: string;
    visible?: () => boolean;
    items: SidebarItem[];
}

@Injectable()
export class SidebarMenuService {
    private static readonly docsURL = 'https://hitkeep.com/guides/introduction/';
    private static readonly supportFallbackURL = 'https://hitkeep.com/support/help/';

    private readonly transloco = inject(TranslocoService);
    private readonly access = inject(AccessService);
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly share = inject(ShareService);
    private readonly router = inject(Router);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });
    private readonly activeUrl = toSignal(
        this.router.events.pipe(
            filter((event): event is NavigationEnd => event instanceof NavigationEnd),
            map((event) => event.urlAfterRedirects),
            startWith(this.router.url)
        ),
        { initialValue: this.router.url }
    );

    readonly desktopItems = computed(() => this.buildItems());

    mobileItems(close: () => void): MenuItem[] {
        return this.buildItems(close);
    }

    private buildItems(close?: () => void): MenuItem[] {
        this.activeLanguage();
        const activeUrl = this.activeUrl();
        return this.sections()
            .filter((section) => section.visible?.() ?? true)
            .map((section) => this.sectionItem(section, close, activeUrl))
            .filter((section) => section.items?.length);
    }

    private sectionItem(section: SidebarSection, close: (() => void) | undefined, activeUrl: string): MenuItem {
        const items = section.items.map((item) => this.menuItem(item, close, activeUrl)).filter((item): item is MenuItem => !!item);
        return {
            label: this.transloco.translate(section.labelKey),
            expanded: true,
            items
        };
    }

    private menuItem(item: SidebarItem, close: (() => void) | undefined, activeUrl: string): MenuItem | null {
        if (item.visible && !item.visible()) {
            return null;
        }
        const routerLink = this.routerLinkFor(item);
        const items = item.items?.map((child) => this.menuItem(child, close, activeUrl)).filter((child): child is MenuItem => !!child);
        const hasChildren = !!items?.length;
        return {
            label: this.transloco.translate(item.labelKey),
            icon: item.icon,
            routerLink,
            url: item.url,
            target: item.target,
            items: hasChildren ? items : undefined,
            expanded: hasChildren ? this.isActiveBranch(item, activeUrl, routerLink) : undefined,
            routerLinkActiveOptions: item.exact ? { exact: true } : { exact: false },
            command: close && !hasChildren ? () => close() : undefined
        };
    }

    private routerLinkFor(item: SidebarItem): string | undefined {
        if (this.share.isShareMode() && item.shareRouterLink) {
            return `/share/${this.share.token()}${item.shareRouterLink}`;
        }
        return item.routerLink;
    }

    private isActiveBranch(item: SidebarItem, activeUrl: string, routerLink?: string): boolean {
        return (!!routerLink && this.urlMatches(activeUrl, routerLink, item.exact)) || this.hasActiveDescendant(item, activeUrl);
    }

    private hasActiveDescendant(item: SidebarItem, activeUrl: string): boolean {
        return (
            item.items?.some((child) => {
                const childLink = this.routerLinkFor(child);
                if (childLink && this.urlMatches(activeUrl, childLink, child.exact)) {
                    return true;
                }
                return this.hasActiveDescendant(child, activeUrl);
            }) ?? false
        );
    }

    private urlMatches(activeUrl: string, routerLink: string, exact = false): boolean {
        const normalizedActiveUrl = activeUrl.split(/[?#]/, 1)[0] || '/';
        const normalizedRouterLink = routerLink.split(/[?#]/, 1)[0] || '/';
        if (exact) {
            return normalizedActiveUrl === normalizedRouterLink;
        }
        return normalizedActiveUrl === normalizedRouterLink || normalizedActiveUrl.startsWith(`${normalizedRouterLink}/`);
    }

    private sections(): SidebarSection[] {
        const shareMode = this.share.isShareMode();
        const notShare = () => !shareMode;
        const canViewSystem = () => this.access.hasInstance(INSTANCE_CAPABILITIES.viewSystem);
        const canManageUsers = () => this.access.hasInstance(INSTANCE_CAPABILITIES.manageUsers);
        const canManageTeamSettings = () => this.access.canActiveTeam(TEAM_CAPABILITIES.manageSettings);
        const canManageIntegrations = () => this.access.canActiveTeam(TEAM_CAPABILITIES.manageIntegrations);
        const canManageWebhooks = () => this.access.hasInstance(INSTANCE_CAPABILITIES.manageWebhooks) || this.access.canActiveSite(SITE_CAPABILITIES.manageWebhooks);
        const supportURL = this.supportUrl();
        const utmItems: SidebarItem[] = shareMode
            ? [
                  { labelKey: 'nav.utm', icon: 'pi pi-tags', routerLink: '/utm', shareRouterLink: '/utm', exact: true },
                  { labelKey: 'nav.qrCodes', icon: 'pi pi-qrcode', routerLink: '/utm/qr-codes', shareRouterLink: '/utm/qr-codes' }
              ]
            : [
                  {
                      labelKey: 'nav.utm',
                      icon: 'pi pi-tags',
                      routerLink: '/utm',
                      shareRouterLink: '/utm',
                      exact: true,
                      items: [
                          { labelKey: 'nav.utmBuilder', icon: 'pi pi-link', routerLink: '/utm/builder', visible: notShare },
                          { labelKey: 'nav.qrCodes', icon: 'pi pi-qrcode', routerLink: '/utm/qr-codes', shareRouterLink: '/utm/qr-codes' }
                      ]
                  }
              ];

        return [
            {
                labelKey: 'nav.analytics',
                items: [
                    { labelKey: 'nav.overview', icon: 'pi pi-globe', routerLink: '/overview', visible: notShare, exact: true },
                    { labelKey: 'nav.dashboard', icon: 'pi pi-chart-bar', routerLink: '/dashboard', shareRouterLink: '/dashboard' },
                    { labelKey: 'nav.opportunities', icon: 'pi pi-compass', routerLink: '/opportunities', shareRouterLink: '/opportunities' },
                    { labelKey: 'nav.goals', icon: 'pi pi-flag', routerLink: '/goals', shareRouterLink: '/goals' },
                    { labelKey: 'nav.funnels', icon: 'pi pi-filter', routerLink: '/funnels', shareRouterLink: '/funnels' },
                    { labelKey: 'nav.events', icon: 'pi pi-bolt', routerLink: '/events', shareRouterLink: '/events' },
                    { labelKey: 'nav.webVitals', icon: 'pi pi-gauge', routerLink: '/web-vitals', shareRouterLink: '/web-vitals' },
                    { labelKey: 'nav.aiVisibility', icon: 'pi pi-sparkles', routerLink: '/ai-visibility', shareRouterLink: '/ai-visibility' },
                    { labelKey: 'nav.aiChatbots', icon: 'pi pi-comments', routerLink: '/ai-chatbots', shareRouterLink: '/ai-chatbots' },
                    { labelKey: 'nav.ecommerce', icon: 'pi pi-shopping-bag', routerLink: '/ecommerce', shareRouterLink: '/ecommerce' },
                    ...utmItems,
                    { labelKey: 'nav.importExport', icon: 'pi pi-sync', routerLink: '/import-export', visible: notShare }
                ]
            },
            {
                labelKey: 'nav.integration',
                visible: notShare,
                items: [
                    {
                        labelKey: 'nav.apiClients',
                        icon: 'pi pi-key',
                        routerLink: '/integration/api-clients',
                        exact: true,
                        items: [{ labelKey: 'nav.apiReference', icon: 'pi pi-book', routerLink: '/integration/api-reference' }]
                    },
                    { labelKey: 'nav.webhooks', icon: 'pi pi-send', routerLink: '/integration/webhooks', visible: canManageWebhooks },
                    { labelKey: 'nav.googleSearchConsole', icon: 'pi pi-search', routerLink: '/integration/google-search-console', visible: canManageIntegrations }
                ]
            },
            {
                labelKey: 'nav.account',
                visible: notShare,
                items: [{ labelKey: 'nav.emailReports', icon: 'pi pi-envelope', routerLink: '/settings/reports' }]
            },
            {
                labelKey: 'nav.resources',
                visible: notShare,
                items: [
                    { labelKey: 'nav.docs', icon: 'pi pi-bookmark pi-external-link', url: SidebarMenuService.docsURL, target: '_blank' },
                    { labelKey: 'nav.support', icon: 'pi pi-headphones pi-external-link', url: supportURL, target: '_blank', visible: () => supportURL !== '' }
                ]
            },
            {
                labelKey: 'nav.administration',
                visible: () => notShare() && (canViewSystem() || canManageTeamSettings()),
                items: [
                    { labelKey: 'nav.systemStatus', icon: 'pi pi-server', routerLink: '/admin/status', visible: canViewSystem },
                    { labelKey: 'nav.systemSettings', icon: 'pi pi-shield', routerLink: '/admin/system', visible: canManageUsers },
                    { labelKey: 'nav.team', icon: 'pi pi-users', routerLink: '/admin/team', visible: canManageTeamSettings }
                ]
            }
        ];
    }

    private supportUrl(): string {
        if (!this.bootstrap.cloudHosted()) {
            return '';
        }
        return this.bootstrap.cloudSupportUrl() || SidebarMenuService.supportFallbackURL;
    }
}
