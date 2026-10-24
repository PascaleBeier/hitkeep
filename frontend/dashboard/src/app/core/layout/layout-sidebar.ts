import { afterNextRender, afterRenderEffect, ChangeDetectionStrategy, Component, computed, ElementRef, inject, Injector, input, viewChild, ViewEncapsulation } from '@angular/core';
import { DOCUMENT, NgTemplateOutlet } from '@angular/common';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router, RouterLink, RouterLinkActive } from '@angular/router';
import { filter } from 'rxjs';
import { TranslocoPipe } from '@jsverse/transloco';
import { DrawerModule } from '@openng/optimus-ui/drawer';
import { PanelMenuModule } from '@openng/optimus-ui/panelmenu';
import { FreePlanChip } from '@layout/free-plan-chip';
import { MainLayoutContextService } from '@layout/main-layout-context.service';
import { SidebarMenuService, type SidebarMenuSectionItem } from '@layout/sidebar-menu.service';

@Component({
    selector: 'app-layout-sidebar',
    imports: [NgTemplateOutlet, FreePlanChip, DrawerModule, PanelMenuModule, RouterLink, RouterLinkActive, TranslocoPipe],
    templateUrl: './layout-sidebar.html',
    styleUrl: './layout-sidebar.css',
    encapsulation: ViewEncapsulation.None,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class LayoutSidebar {
    readonly hiddenDesktop = input(false);
    protected readonly context = inject(MainLayoutContextService);
    private readonly sidebarMenu = inject(SidebarMenuService);
    private readonly router = inject(Router);
    private readonly document = inject(DOCUMENT);
    private readonly injector = inject(Injector);
    private readonly mobileNavigation = viewChild<ElementRef<HTMLElement>>('mobileNavigation');
    private mobileOpener: HTMLElement | null = null;
    protected readonly shareService = this.context.shareService;
    protected readonly teamService = this.context.teamService;
    protected readonly canCreateTeams = this.context.canCreateTeams;
    protected readonly isMobileDrawerOpen = this.context.isMobileDrawerOpen;
    protected readonly isCreateTeamVisible = this.context.isCreateTeamVisible;
    protected readonly beforeTeamSwitch = this.context.beforeTeamSwitch;
    private readonly expansionOverrides = this.readStoredOverrides();
    private lastPath = this.router.url.split(/[?#]/, 1)[0];
    protected readonly desktopMenuItems = computed(() => this.sidebarMenu.desktopItems().map((section) => this.sectionModel(section)));
    protected readonly primaryMenuItems = computed(() => this.desktopMenuItems().filter((section) => !section.collapsible));
    protected readonly secondaryMenuItems = computed(() => this.desktopMenuItems().filter((section) => section.collapsible));
    protected readonly mobileMenuItems = computed(() => this.sidebarMenu.mobileItems(() => this.closeMobileDrawer()));

    constructor() {
        afterRenderEffect({
            write: () => {
                if (!this.isMobileDrawerOpen() && this.mobileOpener) {
                    this.mobileOpener.focus();
                    this.mobileOpener = null;
                }
            }
        });
        this.revealActiveSection();
        this.router.events
            .pipe(
                filter((event): event is NavigationEnd => event instanceof NavigationEnd),
                takeUntilDestroyed()
            )
            .subscribe((event) => {
                const path = event.urlAfterRedirects.split(/[?#]/, 1)[0];
                if (path !== this.lastPath) {
                    this.lastPath = path;
                    this.revealActiveSection();
                }
            });
    }

    protected closeMobileDrawer(): void {
        this.isMobileDrawerOpen.set(false);
    }

    protected focusMobileNavigation(): void {
        this.mobileOpener = this.document.activeElement instanceof HTMLElement ? this.document.activeElement : null;
        afterNextRender(
            {
                write: () => {
                    if (this.isMobileDrawerOpen()) this.mobileNavigation()?.nativeElement.focus();
                }
            },
            { injector: this.injector }
        );
    }

    private sectionModel(section: SidebarMenuSectionItem): SidebarMenuSectionItem {
        const key = 's:' + section.sectionId;
        const model = {
            ...section,
            styleClass: 'layout-sidebar-menu__section-toggle',
            expanded: !section.collapsible || (this.expansionOverrides[key] ?? this.sidebarMenu.activeSectionId() === section.sectionId),
            items: section.items?.map((item) => ({ ...item, command: () => this.closeMobileDrawer() }))
        };
        if (section.collapsible) {
            model.command = () => {
                // PanelMenu invokes commands before toggling its model; keep its DOM and focus stable.
                this.expansionOverrides[key] = !model.expanded;
                this.storeOverrides();
            };
        }
        return model;
    }

    private revealActiveSection(): void {
        const id = this.sidebarMenu.activeSectionId();
        if (id) {
            this.expansionOverrides['s:' + id] = true;
            this.storeOverrides();
        }
    }

    private readStoredOverrides(): Record<string, boolean> {
        try {
            const parsed: unknown = JSON.parse(localStorage.getItem('hk_sidebar_sections') ?? 'null');
            if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
                return Object.fromEntries(Object.entries(parsed).filter(([key, value]) => /^(s:(integration|account|resources|administration)|i:.+)$/.test(key) && typeof value === 'boolean'));
            }
        } catch {
            // Navigation remains usable when browser storage is unavailable.
        }
        return {};
    }

    private storeOverrides(): void {
        try {
            localStorage.setItem('hk_sidebar_sections', JSON.stringify(this.expansionOverrides));
        } catch {
            // Keep the in-memory expansion state when storage is blocked or full.
        }
    }
}
