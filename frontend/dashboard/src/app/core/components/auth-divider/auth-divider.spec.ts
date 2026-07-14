import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { AuthDivider } from './auth-divider';

describe('AuthDivider', () => {
    let fixture: ComponentFixture<AuthDivider>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                AuthDivider,
                TranslocoTestingModule.forRoot({
                    langs: { en: { login: { or: 'or' } } },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();
        fixture = TestBed.createComponent(AuthDivider);
    });

    it('renders localized content through PrimeNG divider', async () => {
        fixture.componentRef.setInput('labelKey', 'login.or');

        await fixture.whenStable();

        const divider = fixture.nativeElement.querySelector('p-divider.p-divider');
        expect(divider).toBeTruthy();
        expect(divider.textContent.trim()).toBe('or');
    });
});
