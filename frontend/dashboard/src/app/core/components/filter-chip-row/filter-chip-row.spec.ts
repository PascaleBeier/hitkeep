import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { FilterChipItem, FilterChipRow } from '@components/filter-chip-row/filter-chip-row';

describe('FilterChipRow', () => {
    let fixture: ComponentFixture<FilterChipRow>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                FilterChipRow,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                noActiveFilter: 'No active filter',
                                removeFilterAria: 'Remove filter',
                                actions: { clearAll: 'Clear all' }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(FilterChipRow);
    });

    it('shows the empty hint when no chips are active', () => {
        fixture.componentRef.setInput('chips', []);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('No active filter');
        expect(fixture.nativeElement.querySelectorAll('button').length).toBe(0);
    });

    it('renders one chip per filter with an accessible remove button', () => {
        const removeCity = vi.fn();
        const removePath = vi.fn();
        const chips: FilterChipItem[] = [
            { key: 'city:Berlin', label: 'City: Berlin', remove: removeCity },
            { key: 'path:/docs', label: 'Page: /docs', remove: removePath }
        ];
        fixture.componentRef.setInput('chips', chips);
        fixture.detectChanges();

        const labels = [...fixture.nativeElement.querySelectorAll('span > span')].map((node) => (node as HTMLElement).textContent?.trim());
        expect(labels).toEqual(['City: Berlin', 'Page: /docs']);

        const removeButtons = [...fixture.nativeElement.querySelectorAll('span button')] as HTMLButtonElement[];
        expect(removeButtons.length).toBe(2);
        expect(removeButtons[0].getAttribute('aria-label')).toBe('Remove filter');
        expect(removeButtons[0].querySelector('.pi-times')).not.toBeNull();

        removeButtons[1].click();
        expect(removePath).toHaveBeenCalledTimes(1);
        expect(removeCity).not.toHaveBeenCalled();
    });

    it('emits clearAll from the trailing clear action', () => {
        const cleared = vi.fn();
        fixture.componentRef.setInput('chips', [{ key: 'city:Berlin', label: 'City: Berlin', remove: vi.fn() }]);
        fixture.componentInstance.clearAll.subscribe(cleared);
        fixture.detectChanges();

        const clearButton = [...fixture.nativeElement.querySelectorAll('button')].find((node) => (node as HTMLElement).textContent?.includes('Clear all')) as HTMLButtonElement;
        expect(clearButton).toBeDefined();

        clearButton.click();
        expect(cleared).toHaveBeenCalledTimes(1);
        expect(fixture.nativeElement.textContent).not.toContain('No active filter');
    });
});
