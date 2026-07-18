import { TestBed } from '@angular/core/testing';

import { SystemStatusCard } from './system-status-card';

describe('SystemStatusCard', () => {
    it('connects its title and description to the card region', () => {
        const fixture = TestBed.createComponent(SystemStatusCard);
        fixture.componentRef.setInput('title', 'Database');
        fixture.componentRef.setInput('titleId', 'database-title');
        fixture.componentRef.setInput('description', 'Recovery, files, memory, and disk availability.');
        fixture.componentRef.setInput('refreshLabel', 'Refresh status');
        fixture.componentRef.setInput('wide', true);
        fixture.detectChanges();

        const section = fixture.nativeElement.querySelector('section') as HTMLElement;
        const description = fixture.nativeElement.querySelector('#database-title-description') as HTMLElement;

        expect(section.getAttribute('aria-labelledby')).toBe('database-title');
        expect(section.getAttribute('aria-describedby')).toBe('database-title-description');
        expect(description.textContent?.trim()).toBe('Recovery, files, memory, and disk availability.');
        expect(fixture.nativeElement.classList.contains('col-span-full')).toBe(true);
    });
});
