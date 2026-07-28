import { Injectable, effect, inject } from '@angular/core';
import { Translation } from '@openng/optimus-ui/api';
import { Optimus } from '@openng/optimus-ui/config';
import { de } from '@openng/optimus-ui-locale/js/de.js';
import { en } from '@openng/optimus-ui-locale/js/en.js';
import { es } from '@openng/optimus-ui-locale/js/es.js';
import { fr } from '@openng/optimus-ui-locale/js/fr.js';
import { it } from '@openng/optimus-ui-locale/js/it.js';
import { nl } from '@openng/optimus-ui-locale/js/nl.js';
import { pt_BR } from '@openng/optimus-ui-locale/js/pt_BR.js';
import { injectActiveLang } from '@core/i18n/active-lang';
import { DASHBOARD_LANGUAGE_CODES, DashboardLanguage, DEFAULT_DASHBOARD_LANGUAGE } from '@core/i18n/supported-locales';

const OPTIMUS_TRANSLATIONS = {
    en,
    de,
    es,
    fr,
    it,
    nl,
    pt: pt_BR
} satisfies Record<DashboardLanguage, Translation>;

@Injectable({ providedIn: 'root' })
export class OptimusLocaleSyncService {
    private readonly optimus = inject(Optimus);
    private readonly activeLanguage = injectActiveLang();

    constructor() {
        effect(() => {
            const language = this.activeLanguage();
            const dashboardLanguage = DASHBOARD_LANGUAGE_CODES.includes(language as DashboardLanguage) ? (language as DashboardLanguage) : DEFAULT_DASHBOARD_LANGUAGE;
            this.optimus.setTranslation(OPTIMUS_TRANSLATIONS[dashboardLanguage]);
        });
    }
}
