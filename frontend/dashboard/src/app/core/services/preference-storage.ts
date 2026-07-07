import { DOCUMENT } from '@angular/common';
import { Service, inject } from '@angular/core';

/**
 * Safe, share-route-aware persistence for UI preferences.
 *
 * Reads and writes JSON values in localStorage, treating persistence as a
 * convenience: storage failures never throw, and public share routes never
 * persist so viewers cannot overwrite the owner's preferences on a shared browser.
 */
@Service()
export class PreferenceStorage {
    private readonly document = inject(DOCUMENT);

    available(): boolean {
        return this.storage() !== null;
    }

    read<T>(key: string): T | null {
        const storage = this.storage();
        if (!storage) {
            return null;
        }
        try {
            const raw = storage.getItem(key);
            return raw === null ? null : (JSON.parse(raw) as T);
        } catch {
            return null;
        }
    }

    write(key: string, value: unknown): void {
        const storage = this.storage();
        if (!storage) {
            return;
        }
        try {
            storage.setItem(key, JSON.stringify(value));
        } catch {
            // Preference persistence must never block the app.
        }
    }

    private storage(): Storage | null {
        const view = this.document.defaultView;
        if (!view || this.isShareRoute(view.location.pathname)) {
            return null;
        }
        try {
            return view.localStorage;
        } catch {
            return null;
        }
    }

    private isShareRoute(pathname: string): boolean {
        return pathname.includes('/share/') || pathname.includes('/qr-share/');
    }
}
