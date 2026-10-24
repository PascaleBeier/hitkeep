import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideRouter, Router } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { MenuItem } from '@openng/optimus-ui/api';
import { vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { NavigationSearch } from './navigation-search';
import { SidebarMenuService } from './sidebar-menu.service';

describe('NavigationSearch', () => {
    let fixture: ComponentFixture<NavigationSearch>;
    const items = signal<MenuItem[]>([]);
    beforeEach(async () => {
        items.set([
            { label: 'UTM', routerLink: '/utm' },
            { label: 'API Clients', routerLink: '/integration/api-clients' }
        ]);
        await TestBed.configureTestingModule({
            imports: [
                NavigationSearch,
                TranslocoTestingModule.forRoot({
                    langs: { en: { nav: { search: 'Search navigation', searchPlaceholder: 'Find a page', noSearchResults: 'No pages found' } } },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ],
            providers: [provideRouter([]), { provide: SidebarMenuService, useValue: { searchableItems: items } }]
        }).compileComponents();
        fixture = TestBed.createComponent(NavigationSearch);
        await fixture.whenStable();
    });

    afterEach(async () => {
        fixture.componentInstance.close();
        await fixture.whenStable();
        await expect.poll(() => document.querySelector('.p-dialog-mask')).toBeNull();
        vi.restoreAllMocks();
    });

    it('opens one search dialog from Ctrl K while editing and ignores handled/composition events', async () => {
        const input = document.createElement('input');
        document.body.append(input);
        input.focus();
        const handled = new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, cancelable: true });
        handled.preventDefault();
        document.dispatchEvent(handled);
        document.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, isComposing: true }));
        await fixture.whenStable();
        expect(document.querySelector('#navigation-search-input')).toBeNull();
        for (let i = 0; i < 2; i++) document.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, cancelable: true }));
        await fixture.whenStable();
        expect(document.querySelectorAll('#navigation-search-input')).toHaveLength(1);
        input.remove();
    });

    it('filters current translated links and opens a keyboard-selected result', async () => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        await userEvent.click(fixture.nativeElement.querySelector('button'));
        await fixture.whenStable();
        await userEvent.fill(document.querySelector('#navigation-search-input')!, 'utm');
        await expect.poll(() => document.querySelector('[role="option"]')?.textContent).toContain('UTM');
        await userEvent.keyboard('{Enter}');
        await fixture.whenStable();
        expect(navigate).toHaveBeenCalledWith('/utm');
        expect(document.querySelector('#navigation-search-input')).toBeNull();
    });

    it('uses updated scope and translated labels without a duplicate navigation model', async () => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        fixture.componentInstance.open();
        items.set([{ label: 'Kampagnen', routerLink: '/share/token/utm' }]);
        await fixture.whenStable();
        await userEvent.fill(document.querySelector('#navigation-search-input')!, 'kamp');
        await expect.poll(() => document.querySelector('[role="option"] a')?.getAttribute('href')).toBe('/share/token/utm');
        await userEvent.keyboard('{Enter}');
        await fixture.whenStable();
        expect(navigate).toHaveBeenCalledWith('/share/token/utm');
    });

    it('shows an empty state and restores trigger focus on close', async () => {
        const trigger = fixture.nativeElement.querySelector('button') as HTMLButtonElement;
        await userEvent.click(trigger);
        await fixture.whenStable();
        await userEvent.fill(document.querySelector('#navigation-search-input')!, 'missing-page');
        await expect.poll(() => document.body.textContent).toContain('No pages found');
        fixture.componentInstance.close();
        await fixture.whenStable();
        await expect.poll(() => document.activeElement).toBe(trigger);
    });
});
