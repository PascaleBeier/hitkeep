import { DOCUMENT } from '@angular/common';
import { inject, Service } from '@angular/core';

const CLOUD_SIGNUP_TRACKER_ID = 'hk-cloud-signup-tracker';
const MAX_PENDING_EVENTS = 20;

type EventProperties = Record<string, unknown>;
type HitKeepWindow = Window &
    typeof globalThis & {
        hk?: {
            event?: (name: string, properties?: EventProperties) => void;
        };
    };

interface PendingEvent {
    name: string;
    properties: EventProperties;
}

@Service()
export class CloudSignupTrackingService {
    private readonly document = inject(DOCUMENT);
    private readonly pendingEvents: PendingEvent[] = [];

    install(): void {
        const existingScript = this.document.getElementById(CLOUD_SIGNUP_TRACKER_ID);
        if (existingScript) {
            this.flushPendingEvents();
            return;
        }

        const script = this.document.createElement('script');
        script.id = CLOUD_SIGNUP_TRACKER_ID;
        script.async = true;
        script.src = '/hk.js';
        script.setAttribute('data-disable-spa-tracking', 'true');
        script.setAttribute('data-disable-outbound-tracking', 'true');
        script.setAttribute('data-disable-download-tracking', 'true');
        script.setAttribute('data-disable-form-tracking', 'true');
        script.addEventListener('load', () => this.flushPendingEvents());

        this.document.head.appendChild(script);
    }

    trackEvent(name: string, properties: EventProperties = {}): void {
        const win = this.document.defaultView as HitKeepWindow | null;
        if (win?.hk?.event) {
            win.hk.event(name, properties);
            return;
        }

        if (this.pendingEvents.length >= MAX_PENDING_EVENTS) {
            this.pendingEvents.shift();
        }
        this.pendingEvents.push({ name, properties });
    }

    private flushPendingEvents(): void {
        const win = this.document.defaultView as HitKeepWindow | null;
        if (!win?.hk?.event) {
            return;
        }

        while (this.pendingEvents.length > 0) {
            const event = this.pendingEvents.shift();
            if (event) {
                win.hk.event(event.name, event.properties);
            }
        }
    }
}
