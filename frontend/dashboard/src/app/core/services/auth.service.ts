import { Injectable, computed, inject, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable, catchError, finalize, of, tap } from 'rxjs';

import { PublicKeyCredentialAssertionJson, PublicKeyCredentialRequestOptionsJson } from '@core/utils/webauthn';

export type AuthStatus = 'unknown' | 'authenticated' | 'unauthenticated';

export interface PasskeyLoginStartResponse {
    challenge_token: string;
    publicKey: PublicKeyCredentialRequestOptionsJson;
}

export interface PasskeyLoginFinishRequest {
    challenge_token: string;
    credential: PublicKeyCredentialAssertionJson;
    remember_me?: boolean;
}

export interface LoginResponse {
    status: 'ok' | 'mfa_required';
    challenge_token?: string;
    factors?: ('totp' | 'passkey' | 'recovery_code' | 'email_link')[];
    passkey?: PasskeyLoginStartResponse['publicKey'];
}

export interface AuthSession {
    expires_at: string;
    issued_at: string;
    duration_seconds: number;
    warning_seconds: number;
    extendable: boolean;
    timing_adjustable: boolean;
    remembered: boolean;
    remember_expires_at?: string | null;
    remember_me_duration_days: number;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
    private http = inject(HttpClient);
    private ticker: ReturnType<typeof setInterval> | null = null;
    readonly status = signal<AuthStatus>('unknown');
    readonly session = signal<AuthSession | null>(null);
    readonly sessionLoading = signal(false);
    readonly sessionExtending = signal(false);
    readonly nowMs = signal(Date.now());
    readonly isAuthenticated = computed(() => this.status() === 'authenticated');
    readonly sessionRemainingMs = computed(() => {
        const session = this.session();
        if (!session) return 0;
        return Math.max(new Date(session.expires_at).getTime() - this.nowMs(), 0);
    });
    readonly sessionRemainingSeconds = computed(() => Math.ceil(this.sessionRemainingMs() / 1000));
    readonly sessionDisplayRemainingMs = computed(() => {
        const session = this.session();
        if (!session) return 0;
        const expiry = session.remembered && session.remember_expires_at ? session.remember_expires_at : session.expires_at;
        return Math.max(new Date(expiry).getTime() - this.nowMs(), 0);
    });
    readonly sessionDisplayRemainingSeconds = computed(() => Math.ceil(this.sessionDisplayRemainingMs() / 1000));
    readonly sessionWarningActive = computed(() => {
        const session = this.session();
        if (!session || this.status() !== 'authenticated') return false;
        const remaining = this.sessionDisplayRemainingSeconds();
        return remaining > 0 && remaining <= session.warning_seconds;
    });
    readonly sessionExpired = computed(() => !!this.session() && this.sessionDisplayRemainingMs() <= 0);

    login(credentials: { email: string; password: string; remember_me?: boolean }): Observable<LoginResponse> {
        return this.http.post<LoginResponse>('/api/login', credentials).pipe(
            tap((resp) => {
                if (resp.status === 'ok') {
                    this.status.set('authenticated');
                }
            })
        );
    }

    logout(): Observable<void> {
        return this.http.post<void>('/api/logout', {}).pipe(finalize(() => this.markUnauthenticated()));
    }

    loadSession(): Observable<AuthSession | null> {
        this.sessionLoading.set(true);
        return this.http.get<AuthSession>('/api/auth/session').pipe(
            tap((session) => this.applySession(session)),
            catchError(() => {
                this.clearSession();
                return of(null);
            }),
            finalize(() => this.sessionLoading.set(false))
        );
    }

    extendSession(): Observable<AuthSession> {
        this.sessionExtending.set(true);
        return this.http.post<AuthSession>('/api/auth/session/extend', {}).pipe(
            tap((session) => this.applySession(session)),
            finalize(() => this.sessionExtending.set(false))
        );
    }

    requestPasswordReset(email: string): Observable<void> {
        return this.http.post<void>('/api/auth/forgot-password', { email });
    }

    resetPassword(token: string, password: string): Observable<void> {
        return this.http.post<void>('/api/auth/reset-password', { token, password });
    }

    changePassword(current: string, newPass: string): Observable<void> {
        return this.http.post<void>('/api/user/password', {
            current_password: current,
            new_password: newPass
        });
    }

    startPasskeyLogin(): Observable<PasskeyLoginStartResponse> {
        return this.http.post<PasskeyLoginStartResponse>('/api/auth/passkey/login/start', {});
    }

    finishPasskeyLogin(payload: PasskeyLoginFinishRequest): Observable<void> {
        return this.http.post<void>('/api/auth/passkey/login/finish', payload).pipe(tap(() => this.status.set('authenticated')));
    }

    verifyMfaTotp(challengeToken: string, code: string): Observable<void> {
        return this.http
            .post<void>('/api/auth/mfa/totp/verify', {
                challenge_token: challengeToken,
                code
            })
            .pipe(tap(() => this.status.set('authenticated')));
    }

    verifyMfaRecoveryCode(challengeToken: string, code: string): Observable<void> {
        return this.http
            .post<void>('/api/auth/mfa/recovery-code/verify', {
                challenge_token: challengeToken,
                code
            })
            .pipe(tap(() => this.status.set('authenticated')));
    }

    requestMfaEmailLink(challengeToken: string, returnUrl: string): Observable<void> {
        return this.http.post<void>('/api/auth/mfa/email-link/request', {
            challenge_token: challengeToken,
            return_url: returnUrl
        });
    }

    markAuthenticated() {
        this.status.set('authenticated');
    }

    markUnauthenticated() {
        this.status.set('unauthenticated');
        this.clearSession();
    }

    applySession(session: AuthSession) {
        this.session.set(session);
        this.status.set('authenticated');
        this.startTicker();
    }

    private clearSession() {
        this.session.set(null);
        this.stopTicker();
    }

    private startTicker() {
        this.nowMs.set(Date.now());
        if (this.ticker) return;
        this.ticker = setInterval(() => {
            this.nowMs.set(Date.now());
            if (this.sessionExpired()) {
                this.markUnauthenticated();
            }
        }, 1000);
        const nodeTimer = this.ticker as ReturnType<typeof setInterval> & { unref?: () => void };
        nodeTimer.unref?.();
    }

    private stopTicker() {
        if (!this.ticker) return;
        clearInterval(this.ticker);
        this.ticker = null;
    }
}
