import { TestBed } from '@angular/core/testing';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { Optimus, provideOptimus } from '@openng/optimus-ui/config';

import { DASHBOARD_LANGUAGE_CODES, DashboardLanguage } from './supported-locales';
import { OptimusLocaleSyncService } from './optimus-locale-sync.service';

describe('OptimusLocaleSyncService', () => {
    let transloco: TranslocoService;
    let optimus: Optimus;

    beforeEach(async () => {
        const languages = Object.fromEntries(DASHBOARD_LANGUAGE_CODES.map((language) => [language, {}]));

        await TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: languages,
                    translocoConfig: {
                        availableLangs: DASHBOARD_LANGUAGE_CODES,
                        defaultLang: 'en',
                        fallbackLang: 'en',
                        reRenderOnLangChange: true
                    },
                    preloadLangs: true
                })
            ],
            providers: [provideOptimus({}), OptimusLocaleSyncService]
        }).compileComponents();

        transloco = TestBed.inject(TranslocoService);
        optimus = TestBed.inject(Optimus);
        TestBed.inject(OptimusLocaleSyncService);
        TestBed.flushEffects();
    });

    it('installs the complete English locale at startup', () => {
        expect(optimus.translation.dateFormat).toBe('mm/dd/yy');
        expect(optimus.translation.firstDayOfWeek).toBe(0);
        expect(optimus.translation.aria?.firstPageLabel).toBe('First Page');
        expect(optimus.translation.aria?.rowsPerPageLabel).toBe('Rows per page');
    });

    it('switches to the packaged locale without reloading', () => {
        transloco.setActiveLang('de');
        TestBed.flushEffects();

        expect(optimus.translation.dateFormat).toBe('dd.mm.yy');
        expect(optimus.translation.firstDayOfWeek).toBe(1);
        expect(optimus.translation.aria?.firstPageLabel).toBe('Erste Seite');
    });

    it('maps the dashboard Portuguese language to Brazilian Portuguese', () => {
        transloco.setActiveLang('pt');
        TestBed.flushEffects();

        expect(optimus.translation.dateFormat).toBe('dd/mm/yy');
        expect(optimus.translation.aria?.rowsPerPageLabel).toBe('Linhas por página');
    });

    it('provides a date format for every supported dashboard language', () => {
        for (const language of DASHBOARD_LANGUAGE_CODES as DashboardLanguage[]) {
            transloco.setActiveLang(language);
            TestBed.flushEffects();
            expect(optimus.translation.dateFormat)
                .withContext(language)
                .toMatch(/^(dd|mm).*yy$/);
        }
    });

    it('falls back to English for an unexpected active language', () => {
        transloco.setActiveLang('xx');
        TestBed.flushEffects();

        expect(optimus.translation.dateFormat).toBe('mm/dd/yy');
        expect(optimus.translation.firstDayOfWeek).toBe(0);
    });
});
