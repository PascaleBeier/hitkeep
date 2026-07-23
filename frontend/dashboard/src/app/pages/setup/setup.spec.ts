import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { Setup } from '@pages/setup/setup';

describe('Setup', () => {
    let component: Setup;
    let fixture: ComponentFixture<Setup>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                Setup,
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [provideHttpClient(), provideRouter([])]
        }).compileComponents();

        fixture = TestBed.createComponent(Setup);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('uses shared OptimusUI auth surfaces', async () => {
        component['errorMessage'].set('setup.errors.unexpected');

        await fixture.whenStable();

        const element = fixture.nativeElement as HTMLElement;
        expect(element.querySelector('app-auth-card p-card.p-card')).toBeTruthy();
        expect(element.querySelector('p-message.p-message')).toBeTruthy();
        expect(element.querySelector('#setup-password-help')?.getAttribute('role')).not.toBe('alert');
    });
});
