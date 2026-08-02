import { computed } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TranslocoService } from '@jsverse/transloco';
import { BehaviorSubject, Subject } from 'rxjs';

import { injectActiveLang } from './active-lang';

describe('injectActiveLang', () => {
    it('invalidates computed labels when the current language catalog finishes loading', () => {
        const langChanges = new BehaviorSubject('de');
        const translationLoaded = new Subject<Record<string, unknown>>();
        let loaded = false;
        TestBed.configureTestingModule({
            providers: [
                {
                    provide: TranslocoService,
                    useValue: {
                        getActiveLang: () => 'de',
                        langChanges$: langChanges.asObservable(),
                        selectTranslation: () => translationLoaded.asObservable()
                    }
                }
            ]
        });

        const activeLanguage = TestBed.runInInjectionContext(() => injectActiveLang());
        const label = computed(() => {
            activeLanguage();
            return loaded ? 'JSON' : 'common.exportFormats.json';
        });

        expect(label()).toBe('common.exportFormats.json');
        loaded = true;
        translationLoaded.next({});
        expect(label()).toBe('JSON');
    });
});
