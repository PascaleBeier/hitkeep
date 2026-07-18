import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { SettingsCard } from './settings-card';

@Component({
    imports: [SettingsCard],
    template: `
        <app-settings-card title="Archive team" subtitle="This cannot be undone." titleId="archive-title" icon="pi pi-box" tone="danger">
            <div settings-card-footer>Action</div>
        </app-settings-card>
    `
})
class SettingsCardTestHost {}

describe('SettingsCard', () => {
    it('exposes semantic heading and danger tone without page-local card markup', async () => {
        const fixture = TestBed.createComponent(SettingsCardTestHost);

        await fixture.whenStable();

        const card = fixture.nativeElement.querySelector('.settings-card') as HTMLElement;
        const heading = fixture.nativeElement.querySelector('h2') as HTMLHeadingElement;

        expect(card.classList.contains('settings-card--danger')).toBe(true);
        expect(card.getAttribute('aria-labelledby')).toBe('archive-title');
        expect(heading.id).toBe('archive-title');
        expect(heading.textContent).toContain('Archive team');
        expect(card.textContent).toContain('This cannot be undone.');
        expect(card.textContent).toContain('Action');
    });
});
