import { ComponentFixture, TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import { DialogShell } from './dialog-shell';

describe('DialogShell', () => {
    let fixture: ComponentFixture<DialogShell>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [DialogShell]
        }).compileComponents();

        fixture = TestBed.createComponent(DialogShell);
        fixture.componentRef.setInput('title', 'Edit resource');
        fixture.componentRef.setInput('visible', true);
        fixture.componentRef.setInput('secondaryLabel', 'Cancel');
        fixture.componentRef.setInput('primaryLabel', 'Save changes');
        fixture.detectChanges();
    });

    afterEach(() => {
        document.querySelectorAll('.p-dialog-mask, .p-dialog').forEach((element) => element.remove());
    });

    it('renders a modal OptimusUI dialog with standardized footer actions', async () => {
        await fixture.whenStable();

        const dialog = document.body.querySelector('.p-dialog') as HTMLElement | null;
        const buttons = Array.from(document.body.querySelectorAll('.dialog-shell-footer button')) as HTMLButtonElement[];

        expect(dialog?.textContent).toContain('Edit resource');
        expect(buttons.map((button) => button.textContent?.trim())).toEqual(['Cancel', 'Save changes']);
        expect(buttons[0].className).toContain('p-button-secondary');
        expect(buttons[0].className).toContain('p-button-outlined');
    });

    it('emits secondary, primary, and visible-change events from the shell', async () => {
        const primary = vi.fn();
        const secondary = vi.fn();
        const visibleChanged = vi.fn();
        fixture.componentInstance.primaryAction.subscribe(primary);
        fixture.componentInstance.secondaryAction.subscribe(secondary);
        fixture.componentInstance.visibleChange.subscribe(visibleChanged);
        await fixture.whenStable();

        const buttons = Array.from(document.body.querySelectorAll('.dialog-shell-footer button')) as HTMLButtonElement[];
        buttons[1].click();
        buttons[0].click();

        expect(primary).toHaveBeenCalledTimes(1);
        expect(secondary).toHaveBeenCalledTimes(1);
        expect(visibleChanged).toHaveBeenCalledWith(false);
    });

    it('blocks close, escape, and footer dismiss while busy', async () => {
        const secondary = vi.fn();
        const visibleChanged = vi.fn();
        fixture.componentInstance.secondaryAction.subscribe(secondary);
        fixture.componentInstance.visibleChange.subscribe(visibleChanged);
        fixture.componentRef.setInput('busy', true);
        fixture.detectChanges();
        await fixture.whenStable();

        const buttons = Array.from(document.body.querySelectorAll('.dialog-shell-footer button')) as HTMLButtonElement[];
        buttons[0].click();
        document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));

        expect(buttons.every((button) => button.disabled)).toBe(true);
        expect(document.body.querySelector('.p-dialog-header-close')).toBeNull();
        expect(secondary).not.toHaveBeenCalled();
        expect(visibleChanged).not.toHaveBeenCalledWith(false);
    });

    it('supports alert dialog semantics and footerless manager dialogs', async () => {
        fixture.destroy();
        document.querySelectorAll('.p-dialog-mask, .p-dialog').forEach((element) => element.remove());

        const footerlessFixture = TestBed.createComponent(DialogShell);
        footerlessFixture.componentRef.setInput('title', 'Manager');
        footerlessFixture.componentRef.setInput('visible', true);
        footerlessFixture.componentRef.setInput('role', 'alertdialog');
        footerlessFixture.componentRef.setInput('showSecondary', false);
        footerlessFixture.componentRef.setInput('showPrimary', false);
        footerlessFixture.detectChanges();
        await footerlessFixture.whenStable();

        const dialog = document.body.querySelector('.p-dialog') as HTMLElement | null;

        expect(dialog?.getAttribute('role')).toBe('alertdialog');
        expect(document.body.querySelector('.dialog-shell-footer')).toBeNull();

        footerlessFixture.destroy();
    });
});
