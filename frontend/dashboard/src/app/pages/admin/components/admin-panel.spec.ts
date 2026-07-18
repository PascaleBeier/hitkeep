import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { AdminPanel } from './admin-panel';

@Component({
    imports: [AdminPanel],
    template: `
        <app-admin-panel titleId="users-title" title="Users" subtitle="12 accounts" [padded]="false">
            <button admin-panel-header type="button">Refresh</button>
            <p admin-panel-messages>Updated</p>
            <div data-testid="body">Table</div>
            <button admin-panel-footer type="button">Next</button>
        </app-admin-panel>
    `
})
class AdminPanelTestHost {}

describe('AdminPanel', () => {
    it('provides a labelled, token-backed frame for admin content', async () => {
        const fixture = TestBed.createComponent(AdminPanelTestHost);

        await fixture.whenStable();

        const section = fixture.nativeElement.querySelector('section') as HTMLElement;
        const body = fixture.nativeElement.querySelector('[data-testid="body"]')?.parentElement as HTMLElement;

        expect(section.getAttribute('aria-labelledby')).toBe('users-title');
        expect(section.querySelector('h2')?.textContent).toContain('Users');
        expect(section.textContent).toContain('12 accounts');
        expect(section.textContent).toContain('Refresh');
        expect(section.textContent).toContain('Updated');
        expect(section.textContent).toContain('Next');
        expect(body.classList.contains('p-4')).toBe(false);
    });
});
