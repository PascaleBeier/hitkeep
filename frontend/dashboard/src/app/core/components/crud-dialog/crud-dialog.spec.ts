import { ComponentFixture, TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import { CrudDialog } from './crud-dialog';

describe('CrudDialog', () => {
    let fixture: ComponentFixture<CrudDialog>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [CrudDialog]
        }).compileComponents();

        fixture = TestBed.createComponent(CrudDialog);
        fixture.componentRef.setInput('title', 'Edit client');
        fixture.componentRef.setInput('visible', true);
        fixture.componentRef.setInput('submitLabel', 'Save');
        fixture.componentRef.setInput('cancelLabel', 'Cancel');
        fixture.detectChanges();
    });

    afterEach(() => {
        document.querySelectorAll('.p-dialog-mask, .p-dialog').forEach((element) => element.remove());
    });

    it('renders a modal dialog with standardized footer actions', async () => {
        await fixture.whenStable();

        const dialog = document.body.querySelector('.p-dialog') as HTMLElement | null;

        expect(dialog?.textContent).toContain('Edit client');
        expect(dialog?.textContent).toContain('Cancel');
        expect(dialog?.textContent).toContain('Save');
    });

    it('emits submit and cancel events from footer actions', async () => {
        const submitted = vi.fn();
        const cancelled = vi.fn();
        const visibleChanged = vi.fn();
        fixture.componentInstance.submitted.subscribe(submitted);
        fixture.componentInstance.cancelled.subscribe(cancelled);
        fixture.componentInstance.visibleChange.subscribe(visibleChanged);
        await fixture.whenStable();

        const buttons = Array.from(document.body.querySelectorAll('.dialog-shell-footer button')) as HTMLButtonElement[];
        buttons.find((button) => button.textContent?.includes('Save'))?.click();
        buttons.find((button) => button.textContent?.includes('Cancel'))?.click();

        expect(submitted).toHaveBeenCalledTimes(1);
        expect(cancelled).toHaveBeenCalledTimes(1);
        expect(visibleChanged).toHaveBeenCalledWith(false);
    });

    it('disables footer actions while saving', async () => {
        fixture.componentRef.setInput('saving', true);
        fixture.detectChanges();
        await fixture.whenStable();

        const buttons = Array.from(document.body.querySelectorAll('.dialog-shell-footer button')) as HTMLButtonElement[];

        expect(buttons.every((button) => button.disabled)).toBe(true);
    });

    it('disables submit independently of the cancel action', async () => {
        fixture.componentRef.setInput('submitDisabled', true);
        fixture.detectChanges();
        await fixture.whenStable();

        const buttons = Array.from(document.body.querySelectorAll('.dialog-shell-footer button')) as HTMLButtonElement[];
        const submit = buttons.find((button) => button.textContent?.includes('Save'));
        const cancel = buttons.find((button) => button.textContent?.includes('Cancel'));

        expect(submit?.disabled).toBe(true);
        expect(cancel?.disabled).toBe(false);
    });

    it('removes dismiss actions while saving', async () => {
        fixture.componentRef.setInput('saving', true);
        fixture.detectChanges();
        await fixture.whenStable();

        const nonFooterButtons = Array.from(document.body.querySelectorAll('.p-dialog button')).filter((button) => !button.closest('.dialog-shell-footer'));

        expect(nonFooterButtons.length).toBe(0);
    });
});
