import { inject, signal, computed, Service } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { DOCUMENT } from '@angular/common';
import { catchError, finalize, tap, throwError } from 'rxjs';
import { AuthService } from '@services/auth.service';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';

export interface UserProfile {
    id: string;
    email: string;
    given_name?: string;
    last_name?: string;
    display_name: string;
    avatar_url: string;
}

@Service()
export class UserProfileService {
    private http = inject(HttpClient);
    private auth = inject(AuthService);
    private document = inject(DOCUMENT);

    readonly profile = signal<UserProfile | null>(null);
    readonly isLoading = signal(false);
    readonly isSaving = signal(false);
    readonly displayName = computed(() => {
        const profile = this.profile();
        if (!profile) return 'User';
        return profile.display_name || profile.email.split('@')[0] || 'User';
    });
    readonly avatarUrl = computed(() => browserAppUrl(this.document, this.profile()?.avatar_url || ''));

    loadProfile() {
        this.isLoading.set(true);
        return this.http.get<UserProfile>('/api/user/profile').pipe(
            tap((profile) => this.applyProfile(profile)),
            catchError((error) => {
                if (error?.status === 401) {
                    this.auth.markUnauthenticated();
                }
                return throwError(() => error);
            }),
            finalize(() => {
                this.isLoading.set(false);
            })
        );
    }

    updateProfile(payload: { email: string; given_name: string; last_name: string }) {
        this.isSaving.set(true);
        return this.http.put<UserProfile>('/api/user/profile', payload).pipe(
            tap((profile) => this.applyProfile(profile)),
            finalize(() => {
                this.isSaving.set(false);
            })
        );
    }

    applyProfile(profile: UserProfile) {
        this.profile.set(profile);
        this.auth.markAuthenticated();
    }
}
