import { HttpTestingController } from '@angular/common/http/testing';
import type { ComponentFixture } from '@angular/core/testing';
import type { SiteSetupState } from '@services/setup-state.service';

/** Fully configured site: a spec only spells out the flag it asserts on. */
export function configuredSetupState(overrides: Partial<SiteSetupState> = {}): SiteSetupState {
    return {
        has_ai_fetches: true,
        has_chatbot_events: true,
        has_custom_events: true,
        has_ecommerce_events: true,
        has_web_vitals: true,
        ...overrides
    };
}

/**
 * Answers the shared setup-state lookup a page fires once its range comes back
 * empty, then re-renders so the callout decision is visible. Safe to call when
 * no lookup happened, which lets `afterEach` drain leftovers before `verify()`.
 */
export function flushSetupState(httpMock: HttpTestingController, siteId: string, overrides: Partial<SiteSetupState> = {}, fixture?: ComponentFixture<unknown>): void {
    for (const request of httpMock.match(`/api/sites/${siteId}/setup-state`)) {
        request.flush(configuredSetupState(overrides));
    }
    fixture?.detectChanges();
}
