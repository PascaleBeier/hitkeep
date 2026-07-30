import { DOCUMENT } from '@angular/common';
import { DestroyRef, WritableSignal, inject, Service } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { RealtimeEvent, RealtimeKind, RealtimeService } from '@services/realtime.service';

interface RealtimeRegistration {
    siteId: () => string | null;
    kinds: readonly RealtimeKind[];
    refresh: () => void;
    enabled: () => boolean;
    debounceMs: number;
    timer: ReturnType<typeof setTimeout> | null;
}

export interface RealtimeRefreshOptions {
    siteId: () => string | null;
    kinds: readonly RealtimeKind[];
    refresh: () => void;
    enabled?: () => boolean;
    debounceMs?: number;
}

export interface RealtimeRefreshSignalOptions extends Omit<RealtimeRefreshOptions, 'refresh'> {
    signal: WritableSignal<number>;
}

const FALLBACK_REFRESH_MS = 30000;
const DEFAULT_DEBOUNCE_MS = 500;

@Service()
export class RealtimeRefreshCoordinator {
    private readonly realtime = inject(RealtimeService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly document = inject(DOCUMENT);
    private readonly registrations = new Set<RealtimeRegistration>();
    private readonly fallbackTimer = setInterval(() => this.runFallbackRefresh(), FALLBACK_REFRESH_MS);

    constructor() {
        this.realtime.events$.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((event) => this.handleEvent(event));
        this.destroyRef.onDestroy(() => {
            clearInterval(this.fallbackTimer);
            for (const registration of this.registrations) {
                this.clearRegistrationTimer(registration);
            }
            this.registrations.clear();
        });
    }

    register(options: RealtimeRefreshOptions): () => void {
        const registration: RealtimeRegistration = {
            siteId: options.siteId,
            kinds: options.kinds,
            refresh: options.refresh,
            enabled: options.enabled ?? (() => true),
            debounceMs: options.debounceMs ?? DEFAULT_DEBOUNCE_MS,
            timer: null
        };
        this.registrations.add(registration);
        return () => {
            this.clearRegistrationTimer(registration);
            this.registrations.delete(registration);
        };
    }

    registerUntilDestroyed(destroyRef: DestroyRef, options: RealtimeRefreshOptions): void {
        const unregister = this.register(options);
        destroyRef.onDestroy(unregister);
    }

    registerSignal(options: RealtimeRefreshSignalOptions): () => void {
        return this.register({
            ...options,
            refresh: () => options.signal.update((key) => key + 1)
        });
    }

    registerSignalUntilDestroyed(destroyRef: DestroyRef, options: RealtimeRefreshSignalOptions): void {
        const unregister = this.registerSignal(options);
        destroyRef.onDestroy(unregister);
    }

    private handleEvent(event: RealtimeEvent): void {
        for (const registration of this.registrations) {
            if (!this.shouldRefresh(registration, event)) continue;
            this.scheduleRefresh(registration);
        }
    }

    private shouldRefresh(registration: RealtimeRegistration, event: RealtimeEvent): boolean {
        if (!registration.enabled()) return false;
        if (registration.siteId() !== event.site_id) return false;
        if (event.type === 'analytics.resync') return true;
        return registration.kinds.some((kind) => event.kinds.includes(kind));
    }

    private scheduleRefresh(registration: RealtimeRegistration): void {
        this.clearRegistrationTimer(registration);
        registration.timer = setTimeout(() => {
            registration.timer = null;
            if (registration.enabled() && this.isDocumentVisible()) {
                registration.refresh();
            }
        }, registration.debounceMs);
    }

    private runFallbackRefresh(): void {
        if (this.realtime.isOpen() || !this.isDocumentVisible()) return;
        const activeSiteId = this.realtime.activeSiteId();
        if (!activeSiteId) return;
        for (const registration of this.registrations) {
            if (registration.enabled() && registration.siteId() === activeSiteId) {
                this.scheduleRefresh(registration);
            }
        }
    }

    private clearRegistrationTimer(registration: RealtimeRegistration): void {
        if (registration.timer) {
            clearTimeout(registration.timer);
            registration.timer = null;
        }
    }

    private isDocumentVisible(): boolean {
        return this.document.visibilityState !== 'hidden';
    }
}
