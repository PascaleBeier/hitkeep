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

interface ExpansionState {
    overrides: Record<string, boolean>;
    activeSectionId: string | null;
}

const SECTION_ID_PREFIX = 's:';
const ITEM_KEY_PREFIX = 'i:';

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
    private readonly expansionOverrides = signal<Record<string, boolean>>(this.readStoredOverrides());
    private readonly expansionState = computed<ExpansionState>(() => ({
        overrides: this.expansionOverrides(),
        activeSectionId: this.sidebarMenu.activeSectionId()
    }));
    protected readonly desktopMenuItems = computed(() => this.sidebarMenu.desktopItems());
    private readonly closeMobileMenuCommand = () => this.closeMobileDrawer();
    protected readonly mobileMenuItems = computed(() => this.sidebarMenu.mobileItems(this.closeMobileMenuCommand));

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
        const expansion = this.expansionState();
        const override = expansion.overrides[SECTION_ID_PREFIX + section.sectionId];
        if (override !== undefined) {
            return override;
        }
        return expansion.activeSectionId === section.sectionId;
    }

    protected toggleSection(section: SidebarMenuSectionItem, event: Event) {
        event.preventDefault();
        event.stopPropagation();
        const sectionId = section.sectionId;
        if (!sectionId) {
            return;
        }
        const next = !this.isSectionOpen(section);
        this.expansionOverrides.update((overrides) => {
            // Exclusive accordion: opening one section folds the others. Item-level
            // expansion (nested menus) lives in its own namespace and survives.
            // Closes are stored explicitly so folding a section whose route is
            // active actually sticks instead of being re-expanded by the
            // active-route fallback.
            const kept = Object.fromEntries(Object.entries(overrides).filter(([key]) => key.startsWith(ITEM_KEY_PREFIX)));
            const updated = { ...kept, [SECTION_ID_PREFIX + sectionId]: next };
            this.storeOverrides(updated);
            return updated;
        });
    }

    protected isItemOpen(item: MenuItem): boolean {
        if (!item.items?.length) {
            return true;
        }
        const override = this.expansionOverrides()[ITEM_KEY_PREFIX + this.getMenuItemKey(item)];
        return override ?? item.expanded === true;
    }

    protected toggleMenuItem(item: MenuItem, event: Event) {
        event.preventDefault();
        event.stopPropagation();
        const key = this.getMenuItemKey(item);
        const next = !this.isItemOpen(item);
        this.expansionOverrides.update((overrides) => {
            // Same exclusive behavior as sections, within the item namespace, and
            // with explicit closes so an active-route branch can be folded.
            const kept = Object.fromEntries(Object.entries(overrides).filter(([stored]) => stored.startsWith(SECTION_ID_PREFIX)));
            const updated = { ...kept, [ITEM_KEY_PREFIX + key]: next };
            this.storeOverrides(updated);
            return updated;
        });
    }

    private getMenuItemKey(item: MenuItem): string {
        if (typeof item.routerLink === 'string') {
            return item.routerLink;
        }
        return item.url ?? item.label ?? '';
    }

    private readStoredOverrides(): Record<string, boolean> {
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

    private storeOverrides(overrides: Record<string, boolean>) {
        try {
            localStorage.setItem(LayoutSidebar.SECTION_STATE_STORAGE_KEY, JSON.stringify(overrides));
        } catch {
            // Storage can be unavailable (private mode, quotas); the accordion still works in-memory.
        }
    }
}
