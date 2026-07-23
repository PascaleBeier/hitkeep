import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Component } from '@angular/core';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { TableRowActions, TableRowActionItem } from './table-row-actions';

describe('TableRowActions', () => {
    let fixture: ComponentFixture<TableRowActions>;
    const editCommand = vi.fn();
    const deleteCommand = vi.fn();

    beforeEach(async () => {
        editCommand.mockReset();
        deleteCommand.mockReset();

        await TestBed.configureTestingModule({
            imports: [
                TableRowActions,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                actions: {
                                    more: 'More actions'
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
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(TableRowActions);
        fixture.componentRef.setInput('items', rowItems());
        fixture.detectChanges();
    });

    afterEach(() => {
        document.querySelectorAll('.table-row-actions-menu').forEach((element) => element.remove());
    });

    it('renders an accessible ellipsis trigger', () => {
        const button = triggerButton();

        expect(button.getAttribute('aria-label')).toBe('More actions');
        expect(button.querySelector('.pi-ellipsis-h')).not.toBeNull();
    });

    it('opens a OptimusUI menu and executes item commands', async () => {
        triggerButton().click();
        fixture.detectChanges();
        await fixture.whenStable();

        const menu = document.querySelector('.table-row-actions-menu');
        expect(menu?.textContent).toContain('Edit');
        expect(menu?.textContent).toContain('Delete');

        (menu?.querySelectorAll('a')[0] as HTMLElement).click();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(editCommand).toHaveBeenCalledTimes(1);
    });

    it('supports disabled state', () => {
        fixture.componentRef.setInput('disabled', true);
        fixture.detectChanges();

        expect(triggerButton().disabled).toBe(true);
    });

    it('applies danger styling to destructive menu items', async () => {
        triggerButton().click();
        fixture.detectChanges();
        await fixture.whenStable();

        const dangerItem = document.querySelector('.table-row-actions-menu .table-row-actions-menu__item--danger');

        expect(dangerItem?.textContent).toContain('Delete');
    });

    it('opens a different row menu on the first click when another row menu is already open', async () => {
        const hostFixture = TestBed.createComponent(TableRowActionsHost);
        hostFixture.detectChanges();

        const buttons = hostFixture.nativeElement.querySelectorAll('button') as NodeListOf<HTMLButtonElement>;
        buttons[0]?.click();
        hostFixture.detectChanges();
        await hostFixture.whenStable();

        expect(document.querySelector('.table-row-actions-menu')?.textContent).toContain('First');

        buttons[1]?.click();
        hostFixture.detectChanges();
        await hostFixture.whenStable();

        const menus = Array.from(document.querySelectorAll('.table-row-actions-menu'));
        expect(menus.filter((menu) => menu.textContent?.includes('Second')).length).toBe(1);
        expect(menus.some((menu) => menu.textContent?.includes('First'))).toBe(false);
    });

    it('closes an open menu when the same trigger is clicked again', async () => {
        triggerButton().click();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.querySelector('.table-row-actions-menu')?.textContent).toContain('Edit');

        triggerButton().click();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.querySelector('.table-row-actions-menu')?.textContent ?? '').not.toContain('Edit');
    });

    function triggerButton(): HTMLButtonElement {
        return fixture.nativeElement.querySelector('button');
    }

    function rowItems(): TableRowActionItem[] {
        return [{ label: 'Edit', icon: 'pi pi-pencil', command: editCommand }, { separator: true }, { label: 'Delete', icon: 'pi pi-trash', danger: true, command: deleteCommand }];
    }
});

@Component({
    imports: [TableRowActions],
    template: `
        <app-table-row-actions [items]="firstItems()" />
        <app-table-row-actions [items]="secondItems()" />
    `
})
class TableRowActionsHost {
    firstItems(): TableRowActionItem[] {
        return [{ label: 'First', icon: 'pi pi-pencil', command: () => undefined }];
    }

    secondItems(): TableRowActionItem[] {
        return [{ label: 'Second', icon: 'pi pi-pencil', command: () => undefined }];
    }
}
