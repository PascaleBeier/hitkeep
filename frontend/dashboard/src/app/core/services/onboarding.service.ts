import { inject, signal, Service } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { defer, finalize, tap } from 'rxjs';

export interface OnboardingStep {
    key: 'create_site' | 'verify_tracking' | 'automatic_events' | 'invite_teammate' | 'schedule_report';
    complete: boolean;
    current?: number;
    target?: number;
    site_id?: string;
    site_domain?: string;
}

export interface UserOnboarding {
    dismissed: boolean;
    complete: boolean;
    steps: OnboardingStep[];
}

@Service()
export class OnboardingService {
    private http = inject(HttpClient);
    private pendingLoads = 0;
    private loadRequestID = 0;
    private dismissedLocally = false;

    readonly onboarding = signal<UserOnboarding | null>(null);
    readonly isLoading = signal(false);

    load() {
        return defer(() => {
            const requestID = ++this.loadRequestID;
            this.pendingLoads += 1;
            this.isLoading.set(true);
            return this.http.get<UserOnboarding>('/api/user/onboarding').pipe(
                tap((onboarding) => {
                    if (requestID === this.loadRequestID) {
                        this.onboarding.set(this.dismissedLocally ? { ...onboarding, dismissed: true } : onboarding);
                    }
                }),
                finalize(() => {
                    this.pendingLoads = Math.max(this.pendingLoads - 1, 0);
                    this.isLoading.set(this.pendingLoads > 0);
                })
            );
        });
    }

    dismiss() {
        return this.http.post<void>('/api/user/onboarding/dismiss', {}).pipe(
            tap(() => {
                this.dismissedLocally = true;
                this.loadRequestID += 1;
                const current = this.onboarding();
                if (current) {
                    this.onboarding.set({ ...current, dismissed: true });
                }
            })
        );
    }
}
