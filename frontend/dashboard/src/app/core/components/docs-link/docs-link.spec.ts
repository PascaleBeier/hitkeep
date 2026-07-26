import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { DocsLink } from './docs-link';

describe('DocsLink', () => {
    let fixture: ComponentFixture<DocsLink>;
    const anchor = () => fixture.nativeElement.querySelector('a') as HTMLAnchorElement;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                DocsLink,
                TranslocoTestingModule.forRoot({
                    langs: { en: { docs: { guide: 'Read the guide' } } },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(DocsLink);
        fixture.componentRef.setInput('href', 'https://hitkeep.com/guides/tracking/npm-package/');
        fixture.componentRef.setInput('labelKey', 'docs.guide');
        fixture.detectChanges();
    });

    it('opens the guide in a new tab without leaking the referrer', () => {
        expect(anchor().getAttribute('href')).toBe('https://hitkeep.com/guides/tracking/npm-package/');
        expect(anchor().getAttribute('target')).toBe('_blank');
        expect(anchor().getAttribute('rel')).toBe('noreferrer');
    });

    it('uses the translated label as the accessible name', () => {
        expect(anchor().textContent).toContain('Read the guide');
        expect(anchor().getAttribute('aria-label')).toBe('Read the guide');
    });

    it('defaults to the outlined variant and omits an empty test id', () => {
        expect(anchor().classList).toContain('p-button-outlined');
        expect(anchor().hasAttribute('data-testid')).toBe(false);
    });

    it('renders supporting links as text buttons and carries a test id through', () => {
        // Built fresh rather than mutated: pButton fixes its variant classes at init,
        // and every call site picks a variant once.
        const supporting = TestBed.createComponent(DocsLink);
        supporting.componentRef.setInput('href', 'https://www.npmjs.com/package/@hitkeep/tracker');
        supporting.componentRef.setInput('labelKey', 'docs.guide');
        supporting.componentRef.setInput('variant', 'text');
        supporting.componentRef.setInput('testId', 'npm-registry-link');
        supporting.detectChanges();

        const link: HTMLAnchorElement = supporting.nativeElement.querySelector('a');
        expect(link.classList).toContain('p-button-text');
        expect(link.getAttribute('data-testid')).toBe('npm-registry-link');
    });
});
