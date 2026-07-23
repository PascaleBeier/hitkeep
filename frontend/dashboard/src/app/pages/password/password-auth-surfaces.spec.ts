import { provideHttpClient } from '@angular/common/http';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { ForgotPassword } from './forgot-password';
import { ResetPassword } from './reset-password';

const translocoTestingModule = TranslocoTestingModule.forRoot({
    langs: { en: {} },
    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
    preloadLangs: true
});

describe('Password auth surfaces', () => {
    it('renders forgot-password feedback with shared OptimusUI components', async () => {
        await TestBed.configureTestingModule({
            imports: [ForgotPassword, translocoTestingModule],
            providers: [provideHttpClient(), provideRouter([])]
        }).compileComponents();
        const fixture = TestBed.createComponent(ForgotPassword);
        fixture.componentInstance['errorMessage'].set('password.forgot.errors.unexpected');

        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        expect(element.querySelector('app-auth-card p-card.p-card')).toBeTruthy();
        expect(element.querySelector('p-message.p-message')).toBeTruthy();
    });

    it('renders reset-password feedback with shared OptimusUI components', async () => {
        TestBed.resetTestingModule();
        await TestBed.configureTestingModule({
            imports: [ResetPassword, translocoTestingModule],
            providers: [
                provideHttpClient(),
                provideRouter([]),
                {
                    provide: ActivatedRoute,
                    useValue: { snapshot: { queryParamMap: convertToParamMap({ token: 'reset-token' }) } }
                }
            ]
        }).compileComponents();
        const fixture = TestBed.createComponent(ResetPassword);
        fixture.componentInstance['errorMessage'].set('password.reset.errors.resetFailed');

        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        expect(element.querySelector('app-auth-card p-card.p-card')).toBeTruthy();
        expect(element.querySelector('p-message.p-message')).toBeTruthy();
        expect(element.querySelector('#reset-password-help')?.getAttribute('role')).not.toBe('alert');
    });
});
