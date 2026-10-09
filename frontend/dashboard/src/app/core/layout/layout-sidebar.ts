import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { RouterLink, RouterLinkActive } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { MenuItem } from '@openng/optimus-ui/api';
import { DrawerModule } from '@openng/optimus-ui/drawer';
import { Brand } from '@components/brand/brand';
import { TeamSwitcher } from '@components/team-switcher/team-switcher';
import { FreePlanChip } from '@layout/free-plan-chip';
import { MainLayoutContextService } from '@layout/main-layout-context.service';
import { SidebarMenuService, type SidebarMenuSectionItem } from '@layout/sidebar-menu.service';

interface SectionExpansionState {
    overrides: Record<string, boolean>;
    activeId: string | null;
}

@Component({
    selector: 'app-layout-sidebar',
    imports: [NgTemplateOutlet, Brand, TeamSwitcher, FreePlanChip, DrawerModule, RouterLink, RouterLinkActive, TranslocoPipe],
    templateUrl: './layout-sidebar.html',
    styleUrl: './layout-sidebar.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class LayoutSidebar {
    private static readonly SECTION_STATE_STORAGE_KEY = 'hk_sidebar_sections';

    protected readonly context = inject(MainLayoutContextService);
    private readonly sidebarMenu = inject(SidebarMenuService);

    protected readonly shareService = this.context.shareService;
    protected readonly teamService = this.context.teamService;
    protected readonly canCreateTeams = this.context.canCreateTeams;
    protected readonly isMobileDrawerOpen = this.context.isMobileDrawerOpen;
    protected readonly isCreateTeamVisible = this.context.isCreateTeamVisible;
    protected readonly beforeTeamSwitch = this.context.beforeTeamSwitch;
    private readonly expandedMenuLabels = signal<ReadonlySet<string>>(new Set());
    private readonly sectionOverrides = signal<Record<string, boolean>>(this.readStoredSectionOverrides());
    private readonly sectionExpansion = computed<SectionExpansionState>(() => ({
        overrides: this.sectionOverrides(),
        activeId: this.sidebarMenu.activeSectionId()
    }));
    protected readonly desktopMenuItems = computed(() => this.applyExpandedState(this.sidebarMenu.desktopItems()));
    private readonly closeMobileMenuCommand = () => this.closeMobileDrawer();
    protected readonly mobileMenuItems = computed(() => this.applyExpandedState(this.sidebarMenu.mobileItems(this.closeMobileMenuCommand)));

    protected closeMobileDrawer() {
        this.isMobileDrawerOpen.set(false);
    }

    protected onMenuItemNavigate(event: Event) {
        event.stopPropagation();
        this.closeMobileDrawer();
    }

    protected isSectionOpen(section: SidebarMenuSectionItem): boolean {
        if (!section.collapsible || !section.sectionId) {
            return true;
        }
        const expansion = this.sectionExpansion();
        const override = expansion.overrides[section.sectionId];
        if (override !== undefined) {
            return override;
        }
        return expansion.activeId === section.sectionId;
    }

    protected toggleSection(section: SidebarMenuSectionItem, event: Event) {
        event.preventDefault();
        event.stopPropagation();
        const sectionId = section.sectionId;
        if (!sectionId) {
            return;
        }
        const next = !this.isSectionOpen(section);
        this.sectionOverrides.update((overrides) => {
            const updated = { ...overrides, [sectionId]: next };
            this.storeSectionOverrides(updated);
            return updated;
        });
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

    private readStoredSectionOverrides(): Record<string, boolean> {
        try {
            const raw = localStorage.getItem(LayoutSidebar.SECTION_STATE_STORAGE_KEY);
            if (!raw) {
                return {};
            }
            const parsed: unknown = JSON.parse(raw);
            if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
                return {};
            }
            return Object.fromEntries(Object.entries(parsed as Record<string, unknown>).filter(([, value]) => typeof value === 'boolean')) as Record<string, boolean>;
        } catch {
            return {};
        }
    }

    private storeSectionOverrides(overrides: Record<string, boolean>) {
        try {
            localStorage.setItem(LayoutSidebar.SECTION_STATE_STORAGE_KEY, JSON.stringify(overrides));
        } catch {
            // Storage can be unavailable (private mode, quotas); the accordion still works in-memory.
        }
    }
}
