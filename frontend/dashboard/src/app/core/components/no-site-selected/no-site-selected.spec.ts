import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { NoSiteSelected } from '@components/no-site-selected/no-site-selected';

describe('NoSiteSelected', () => {
    let fixture: ComponentFixture<NoSiteSelected>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                NoSiteSelected,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: { noSiteSelected: 'No site selected' },
                            aiAgents: { noSiteDescription: 'Pick a site to inspect AI agent traffic.' }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(NoSiteSelected);
    });

    it('renders the shared headline and the page specific description', () => {
        fixture.componentRef.setInput('icon', 'pi pi-sparkles');
        fixture.componentRef.setInput('descriptionKey', 'aiAgents.noSiteDescription');
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('h2')?.textContent).toContain('No site selected');
        expect(fixture.nativeElement.querySelector('p')?.textContent).toContain('Pick a site to inspect AI agent traffic.');
    });

    it('keeps the centred layout and applies the requested icon', () => {
        fixture.componentRef.setInput('icon', 'pi pi-comments');
        fixture.componentRef.setInput('descriptionKey', 'aiAgents.noSiteDescription');
        fixture.detectChanges();

        const wrapper = fixture.nativeElement.querySelector('div') as HTMLElement;
        expect(wrapper.className).toBe('flex flex-col items-center justify-center h-[50vh] gap-4');

        const icon = fixture.nativeElement.querySelector('i') as HTMLElement;
        expect([...icon.classList].sort()).toEqual(['opacity-20', 'pi', 'pi-comments', 'text-6xl', 'text-primary']);
        expect(fixture.nativeElement.querySelector('h2')?.className).toBe('text-2xl font-semibold text-muted-color');
        expect(fixture.nativeElement.querySelector('p')?.className).toBe('text-muted-color');
    });
});
