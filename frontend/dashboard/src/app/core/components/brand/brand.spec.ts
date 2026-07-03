import { ComponentFixture, TestBed } from '@angular/core/testing';
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
            imports: [Brand]
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

    it('loads the logo from the configured browser base path', () => {
        const component = fixture.componentInstance as unknown as { iconUrl: () => string };

        expect(component.iconUrl()).toBe('/hitkeep/icon.png');
    });

    it('links the brand to the configured app root', () => {
        const link = fixture.nativeElement.querySelector('a');

        expect(link?.getAttribute('href')).toBe('/hitkeep/');
        expect(link?.textContent).toContain('HitKeep');
    });
});
