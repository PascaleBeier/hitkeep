import { Signal, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { TranslocoService } from '@jsverse/transloco';
import { map, switchMap } from 'rxjs';

/**
 * Returns a signal that emits the current active language code whenever
 * the language changes and the translation file is fully loaded.
 *
 * Safe to use without null guards: `preloadUserLang()` (APP_INITIALIZER)
 * loads the browser/default language before any component mounts. User
 * preferences can switch the active language after dashboard bootstrap.
 */
export function injectActiveLang(): Signal<string> {
    const transloco = inject(TranslocoService);
    return toSignal(transloco.langChanges$.pipe(switchMap((lang) => transloco.selectTranslation(lang).pipe(map(() => lang)))), { initialValue: transloco.getActiveLang() }) as Signal<string>;
}
