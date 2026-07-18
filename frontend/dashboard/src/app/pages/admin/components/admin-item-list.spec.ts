import { TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { AdminItemList } from './admin-item-list';

describe('AdminItemList', () => {
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                AdminItemList,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                copyControl: {
                                    copy: 'Copy',
                                    copied: 'Copied',
                                    failed: 'Copy failed',
                                    ariaLabel: 'Copy to clipboard'
                                }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();
    });

    it('renders labelled inventory items with accessible metadata', async () => {
        const fixture = TestBed.createComponent(AdminItemList);
        fixture.componentRef.setInput('ariaLabel', 'Tenant database files');
        fixture.componentRef.setInput('emptyMessage', 'No tenant databases');
        fixture.componentRef.setInput('items', [
            {
                id: 'tenant-1',
                label: 'Acme Analytics',
                description: '/data/tenants/tenant-1/hitkeep.db',
                descriptionMonospace: true,
                copyValue: '/data/tenants/tenant-1/hitkeep.db',
                meta: '12 MB',
                metaLabel: 'Size'
            }
        ]);

        await fixture.whenStable();

        const list = fixture.nativeElement.querySelector('ul') as HTMLUListElement;
        const description = fixture.nativeElement.querySelector('[title]') as HTMLElement;

        expect(list.getAttribute('aria-label')).toBe('Tenant database files');
        expect(list.querySelectorAll('li').length).toBe(1);
        expect(list.textContent).toContain('Acme Analytics');
        expect(list.textContent).toContain('Size:');
        expect(list.textContent).toContain('12 MB');
        expect(description.classList.contains('font-mono')).toBe(true);
        expect(description.getAttribute('title')).toBe('/data/tenants/tenant-1/hitkeep.db');
        expect(list.querySelector('app-copy-control')).not.toBeNull();
    });

    it('renders a useful empty state inside the list', async () => {
        const fixture = TestBed.createComponent(AdminItemList);
        fixture.componentRef.setInput('ariaLabel', 'Tenant database files');
        fixture.componentRef.setInput('emptyMessage', 'No tenant databases');
        fixture.componentRef.setInput('items', []);

        await fixture.whenStable();

        const emptyItem = fixture.nativeElement.querySelector('li') as HTMLLIElement;

        expect(emptyItem.textContent?.trim()).toBe('No tenant databases');
    });
});
