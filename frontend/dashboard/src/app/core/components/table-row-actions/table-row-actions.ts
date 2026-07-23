import { ChangeDetectionStrategy, Component, computed, input, signal } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { MenuItem } from '@openng/optimus-ui/api';
import { ButtonModule } from '@openng/optimus-ui/button';
import { Menu, MenuModule } from '@openng/optimus-ui/menu';

export type TableRowActionItem = Omit<MenuItem, 'items'> & {
    danger?: boolean;
    items?: TableRowActionItem[];
};

@Component({
    selector: 'app-table-row-actions',
    imports: [ButtonModule, MenuModule, TranslocoPipe],
    templateUrl: './table-row-actions.html',
    styleUrl: './table-row-actions.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TableRowActions {
    private static activeMenu: Menu | null = null;
    private static activeComponent: TableRowActions | null = null;

    readonly items = input<readonly TableRowActionItem[]>([]);
    readonly disabled = input(false);
    readonly loading = input(false);
    readonly ariaLabelKey = input('common.actions.more');

    protected readonly hasItems = computed(() => this.items().length > 0);
    protected readonly openMenuItems = signal<MenuItem[]>([]);
    private readonly isOpen = signal(false);

    protected toggleMenu(event: Event, menu: Menu): void {
        event.preventDefault();
        event.stopPropagation();

        if (TableRowActions.activeMenu === menu && this.isOpen()) {
            menu.hide();
            this.clearActiveMenu(menu);
            return;
        }

        if (TableRowActions.activeMenu && TableRowActions.activeMenu !== menu) {
            TableRowActions.activeMenu.hide();
            TableRowActions.activeComponent?.openMenuItems.set([]);
            TableRowActions.activeComponent?.isOpen.set(false);
        }

        this.openMenuItems.set(this.items().map((item) => this.toMenuItem(item)));
        menu.show(event);
        this.isOpen.set(true);
        TableRowActions.activeMenu = menu;
        TableRowActions.activeComponent = this;
    }

    protected clearActiveMenu(menu: Menu): void {
        if (TableRowActions.activeMenu === menu) {
            TableRowActions.activeMenu = null;
            TableRowActions.activeComponent = null;
        }
        this.isOpen.set(false);
        this.openMenuItems.set([]);
    }

    private toMenuItem(item: TableRowActionItem): MenuItem {
        const styleClass = [item['styleClass'], item.danger ? 'table-row-actions-menu__item--danger' : ''].filter(Boolean).join(' ');
        return {
            ...item,
            styleClass,
            items: item.items?.map((child) => this.toMenuItem(child))
        };
    }
}
