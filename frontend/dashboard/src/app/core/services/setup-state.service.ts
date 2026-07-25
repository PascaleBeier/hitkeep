import { HttpClient } from '@angular/common/http';
import { Injectable, inject, signal } from '@angular/core';

/** Which analytics features a site has ever produced data for. */
export interface SiteSetupState {
    has_ai_fetches: boolean;
    has_chatbot_events: boolean;
    has_custom_events: boolean;
    has_ecommerce_events: boolean;
    has_web_vitals: boolean;
}

/** One feature flag of the setup state, e.g. `has_web_vitals`. */
export type SetupFeature = keyof SiteSetupState;

/** A site we could not ask counts as fully set up: never accuse the user. */
const CONFIGURED: SiteSetupState = {
    has_ai_fetches: true,
    has_chatbot_events: true,
    has_custom_events: true,
    has_ecommerce_events: true,
    has_web_vitals: true
};

/**
 * Distinguishes "nothing in this range" from "this feature was never set up"
 * with one `GET /api/sites/{id}/setup-state` per site, shared by every
 * analytics page. The answer is cached, so range changes, page switches and
 * realtime refreshes never ask again, and a failed lookup counts as configured
 * rather than accusing the user of a missing setup.
 */
@Injectable({ providedIn: 'root' })
export class SetupStateService {
    private readonly http = inject(HttpClient);
    private readonly states = signal<Record<string, SiteSetupState>>({});
    private readonly inFlight = new Set<string>();

    /**
     * The site's full setup state, or `null` while it is still unknown. The
     * first call triggers the single cached lookup, so a page can gate its own
     * requests on the answer instead of firing them speculatively.
     */
    state(siteId: string | null | undefined): SiteSetupState | null {
        if (!siteId) return null;
        const known = this.states()[siteId];
        if (known) return known;
        this.load(siteId);
        return null;
    }

    /**
     * True only once we know the feature never produced data at all: an empty
     * range plus an empty setup state. Pass `null` for `inRangeTotal` while the
     * page has no numbers yet so a pending load never shows the callout. The
     * first empty range triggers the lookup; ranges with data never ask.
     */
    needsSetup(siteId: string | null | undefined, feature: SetupFeature, inRangeTotal: number | null, loading: boolean): boolean {
        if (loading || inRangeTotal === null || inRangeTotal > 0) return false;
        const state = this.state(siteId);
        return state ? !state[feature] : false;
    }

    private load(siteId: string): void {
        if (this.inFlight.has(siteId)) return;
        this.inFlight.add(siteId);
        this.http.get<SiteSetupState | null>(`/api/sites/${siteId}/setup-state`).subscribe({
            next: (state) => this.remember(siteId, state ?? CONFIGURED),
            error: () => this.remember(siteId, CONFIGURED)
        });
    }

    private remember(siteId: string, state: SiteSetupState): void {
        this.inFlight.delete(siteId);
        this.states.update((current) => ({ ...current, [siteId]: state }));
    }
}
