import { Service, signal } from '@angular/core';

/**
 * Lets distant UI (e.g. the user profile menu) open the Ask AI drawer without
 * coupling to the drawer-host component. The host component watches
 * `openCount` and opens the drawer when it changes.
 */
@Service()
export class AskAIControlService {
    private readonly openCountSignal = signal(0);

    readonly openCount = this.openCountSignal.asReadonly();

    requestOpen() {
        this.openCountSignal.update((count) => count + 1);
    }
}
