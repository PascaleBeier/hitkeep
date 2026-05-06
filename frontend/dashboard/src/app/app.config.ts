import { ApplicationConfig, inject, isDevMode, provideBrowserGlobalErrorListeners, provideEnvironmentInitializer, provideZonelessChangeDetection } from '@angular/core';
import { provideRouter, withViewTransitions } from '@angular/router';
import { providePrimeNG } from 'primeng/config';
import Aura from '@primeuix/themes/aura';

import { routes } from './app.routes';
import { provideHttpClient, withFetch, withInterceptors } from '@angular/common/http';
import { authInterceptor } from '@core/interceptors/auth.interceptor';
import { shareInterceptor } from '@core/interceptors/share.interceptor';
import { provideTransloco } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { TranslocoHttpLoader } from './transloco-loader';
import { providePreloadUserLang } from '@core/i18n/preload-user-lang';
import { PrimeLocaleSyncService } from '@core/i18n/prime-locale-sync.service';

export const appConfig: ApplicationConfig = {
    providers: [
        provideBrowserGlobalErrorListeners(),
        provideZonelessChangeDetection(),
        provideHttpClient(withFetch(), withInterceptors([shareInterceptor, authInterceptor])),
        provideRouter(routes, withViewTransitions()),
        providePrimeNG({
            theme: {
                preset: Aura,
                options: { darkModeSelector: '.p-dark' }
            }
        }),
        provideTransloco({
            config: {
                availableLangs: ['en', 'de', 'es', 'fr', 'it', 'nl'],
                defaultLang: 'en',
                fallbackLang: 'en',
                reRenderOnLangChange: true,
                flatten: {
                    aot: !isDevMode()
                },
                prodMode: !isDevMode()
            },
            loader: TranslocoHttpLoader
        }),
        provideTranslocoLocale({
            defaultLocale: 'en-US',
            langToLocaleMapping: {
                en: 'en-US',
                de: 'de-DE',
                es: 'es-ES',
                fr: 'fr-FR',
                it: 'it-IT',
                nl: 'nl-NL',
                'en-US': 'en-US',
                'de-DE': 'de-DE',
                'es-ES': 'es-ES',
                'fr-FR': 'fr-FR',
                'it-IT': 'it-IT',
                'nl-NL': 'nl-NL'
            }
        }),
        provideEnvironmentInitializer(() => inject(PrimeLocaleSyncService)),
        providePreloadUserLang()
    ]
};
