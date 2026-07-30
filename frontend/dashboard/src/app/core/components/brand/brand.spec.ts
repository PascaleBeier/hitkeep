import { APP_BASE_HREF } from '@angular/common';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { RouterLink, provideRouter } from '@angular/router';
import { afterEach } from 'vitest';
import { Brand } from './brand';

describe('Brand', () => {
    let fixture: ComponentFixture<Brand>;
    let base: HTMLBaseElement;
    let previousBaseHref: string | null;

    beforeEach(async () => {
        base = document.querySelector('base') ?? document.createElement('base');
        previousBaseHref = base.parentNode ? base.getAttribute('href') : null;
        base.href = '/hitkeep/';
        if (!base.parentNode) {
            document.head.append(base);
        }

        await TestBed.configureTestingModule({
            imports: [Brand],
            providers: [provideRouter([]), { provide: APP_BASE_HREF, useValue: '/hitkeep/' }]
        }).compileComponents();

        fixture = TestBed.createComponent(Brand);
        fixture.detectChanges();
    });

    afterEach(() => {
        if (previousBaseHref === null) {
            base.remove();
        } else {
            base.setAttribute('href', previousBaseHref);
        }
    });

    it('loads the small logo SVG from the configured browser base path', () => {
        const component = fixture.componentInstance as unknown as { iconUrl: () => string };

        expect(component.iconUrl()).toBe('/hitkeep/brand-icon.svg');
    });

    it('loads the large logo SVG from the configured browser base path', () => {
        const largeFixture = TestBed.createComponent(Brand);

        largeFixture.componentRef.setInput('size', 'large');
        largeFixture.detectChanges();

        const component = largeFixture.componentInstance as unknown as { iconUrl: () => string };

        expect(component.iconUrl()).toBe('/hitkeep/brand-icon.svg');
    });

    it('marks the logo image for dark-mode color treatment', () => {
        const image = fixture.nativeElement.querySelector('img');

        expect(image?.classList.contains('hk-brand-icon')).toBe(true);
    });

    it('routes the brand to the configured app root without a document navigation', () => {
        const link = fixture.nativeElement.querySelector('a');

        expect(fixture.debugElement.query(By.directive(RouterLink))).not.toBeNull();
        expect(link?.getAttribute('href')).toBe('/hitkeep/');
        expect(link?.textContent).toContain('HitKeep');
    });
});
