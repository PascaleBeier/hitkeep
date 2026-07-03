import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { RouterLink, RouterLinkActive } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { MenuItem } from 'primeng/api';
import { DrawerModule } from 'primeng/drawer';
import { Brand } from '@components/brand/brand';
import { TeamSwitcher } from '@components/team-switcher/team-switcher';
import { SiteSelector } from '@features/sites/components/site-selector';
import { AskAIControl } from '@layout/ask-ai-control';
import { MainLayoutContextService } from '@layout/main-layout-context.service';
import { SidebarMenuService } from '@layout/sidebar-menu.service';

@Component({
    selector: 'app-layout-sidebar',
    imports: [NgTemplateOutlet, Brand, SiteSelector, TeamSwitcher, AskAIControl, DrawerModule, RouterLink, RouterLinkActive, TranslocoPipe],
    templateUrl: './layout-sidebar.html',
    styleUrl: './layout-sidebar.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class LayoutSidebar {
    protected readonly context = inject(MainLayoutContextService);
    private readonly sidebarMenu = inject(SidebarMenuService);

    protected readonly siteService = this.context.siteService;
    protected readonly shareService = this.context.shareService;
    protected readonly teamService = this.context.teamService;
    protected readonly canCreateTeams = this.context.canCreateTeams;
    protected readonly isMobileDrawerOpen = this.context.isMobileDrawerOpen;
    protected readonly isAddSiteVisible = this.context.isAddSiteVisible;
    protected readonly isCreateTeamVisible = this.context.isCreateTeamVisible;
    protected readonly beforeTeamSwitch = this.context.beforeTeamSwitch;
    private readonly expandedMenuLabels = signal<ReadonlySet<string>>(new Set());
    protected readonly desktopMenuItems = computed(() => this.applyExpandedState(this.sidebarMenu.desktopItems()));
    private readonly closeMobileMenuCommand = () => this.closeMobileDrawer();
    protected readonly mobileMenuItems = computed(() => this.applyExpandedState(this.sidebarMenu.mobileItems(this.closeMobileMenuCommand)));

    protected openSiteSettings(tab = '0') {
        this.context.openSiteSettings(tab);
    }

    protected closeMobileDrawer() {
        this.isMobileDrawerOpen.set(false);
    }

    protected onMenuItemNavigate(event: Event) {
        event.stopPropagation();
        this.closeMobileDrawer();
    }

    protected toggleMenuItem(item: MenuItem, isExpanded: boolean | undefined, event: Event) {
        event.preventDefault();
        event.stopPropagation();
        this.expandedMenuLabels.update((labels) => {
            const next = new Set(labels);
            const key = this.getMenuItemKey(item);
            if (isExpanded) {
                next.delete(key);
            } else {
                next.add(key);
            }
            return next;
        });
    }

    private applyExpandedState(items: MenuItem[]): MenuItem[] {
        const expandedLabels = this.expandedMenuLabels();
        return items.map((item) => this.withExpandedState(item, expandedLabels));
    }

    private withExpandedState(item: MenuItem, expandedLabels: ReadonlySet<string>): MenuItem {
        const children = item.items?.map((child) => this.withExpandedState(child, expandedLabels));
        return {
            ...item,
            expanded: item.expanded || expandedLabels.has(this.getMenuItemKey(item)),
            items: children
        };
    }

    private getMenuItemKey(item: MenuItem): string {
        if (typeof item.routerLink === 'string') {
            return item.routerLink;
        }
        return item.url ?? item.label ?? '';
    }
}
