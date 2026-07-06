import { ApplicationConfig, inject, isDevMode, provideBrowserGlobalErrorListeners, provideEnvironmentInitializer, provideZonelessChangeDetection } from '@angular/core';
import { provideRouter, withViewTransitions } from '@angular/router';
import { providePrimeNG } from 'primeng/config';
import Aura from '@primeuix/themes/aura';

import { routes } from './app.routes';
import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { authInterceptor } from '@core/interceptors/auth.interceptor';
import { basePathInterceptor } from '@core/interceptors/base-path.interceptor';
import { shareInterceptor } from '@core/interceptors/share.interceptor';
import { provideTransloco } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { TranslocoHttpLoader } from './transloco-loader';
import { providePreloadUserLang } from '@core/i18n/preload-user-lang';
import { PrimeLocaleSyncService } from '@core/i18n/prime-locale-sync.service';
import { DASHBOARD_LANGUAGE_CODES, DASHBOARD_LOCALE_MAPPING, DEFAULT_DASHBOARD_LANGUAGE, SOURCE_LOCALE } from '@core/i18n/supported-locales';
import { DashboardTitleService } from '@services/dashboard-title.service';

export const appConfig: ApplicationConfig = {
    providers: [
        provideBrowserGlobalErrorListeners(),
        provideZonelessChangeDetection(),
        provideHttpClient(withInterceptors([shareInterceptor, authInterceptor, basePathInterceptor])),
        provideRouter(routes, withViewTransitions()),
        providePrimeNG({
            theme: {
                preset: Aura,
                options: { darkModeSelector: '.p-dark' }
            }
        }),
        provideTransloco({
            config: {
                availableLangs: DASHBOARD_LANGUAGE_CODES,
                defaultLang: DEFAULT_DASHBOARD_LANGUAGE,
                fallbackLang: DEFAULT_DASHBOARD_LANGUAGE,
                reRenderOnLangChange: true,
                flatten: {
                    aot: !isDevMode()
                },
                prodMode: !isDevMode()
            },
            loader: TranslocoHttpLoader
        }),
        provideTranslocoLocale({
            defaultLocale: SOURCE_LOCALE,
            langToLocaleMapping: DASHBOARD_LOCALE_MAPPING
        }),
        provideEnvironmentInitializer(() => inject(PrimeLocaleSyncService)),
        provideEnvironmentInitializer(() => inject(DashboardTitleService)),
        providePreloadUserLang()
    ]
};
