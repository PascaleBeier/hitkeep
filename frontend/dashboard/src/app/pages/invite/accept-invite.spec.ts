import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { AuthService } from '@services/auth.service';

import { AcceptInvite } from './accept-invite';

describe('AcceptInvite', () => {
    let fixture: ComponentFixture<AcceptInvite>;
    let queryParams: Record<string, string>;

    beforeEach(async () => {
        queryParams = {};

        await TestBed.configureTestingModule({
            imports: [
                AcceptInvite,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            invite: {
                                accept: {
                                    title: 'Accept invitation',
                                    description: 'Set a password for the email address that received this invite. You will be taken to your dashboard after accepting.',
                                    passwordLabel: 'Password',
                                    minimumLength: 'Must be at least 8 characters.',
                                    submit: 'Accept invitation',
                                    checkingSession: 'Checking invitation...',
                                    loginRequired: 'Sign in with the email address that received this invite to accept it.',
                                    signIn: 'Sign in',
                                    errors: {
                                        tokenMissing: 'Invalid invitation link. The token is missing.',
                                        expiredOrInvalid: 'This invitation link has expired or is invalid.',
                                        teamLimit: 'This invite cannot be accepted because the account is already linked to another team.',
                                        acceptFailed: 'We could not accept this invitation. Please try again.'
                                    }
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                provideHttpClient(),
                provideHttpClientTesting(),
                provideRouter([]),
                {
                    provide: ActivatedRoute,
                    useValue: {
                        snapshot: {
                            get queryParamMap() {
                                return convertToParamMap(queryParams);
                            }
                        }
                    }
                }
            ]
        }).compileComponents();
    });

    afterEach(() => {
        TestBed.inject(HttpTestingController).verify();
    });

    it('shows an invitation-specific error when the token is missing', () => {
        fixture = TestBed.createComponent(AcceptInvite);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Invalid invitation link. The token is missing.');
        TestBed.inject(HttpTestingController).expectNone('/api/auth/accept-invite');
    });

    it('keeps invite acceptance local when the password is invalid', () => {
        queryParams = { token: 'invite-token' };
        fixture = TestBed.createComponent(AcceptInvite);
        fixture.detectChanges();
        flushUnauthenticatedSession(fixture);

        const form = fixture.nativeElement.querySelector('form') as HTMLFormElement;
        form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Must be at least 8 characters.');
        TestBed.inject(HttpTestingController).expectNone('/api/auth/accept-invite');
    });

    it('accepts an invitation with the token and password, signs in, and enters the dashboard', () => {
        const navigateSpy = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        queryParams = { token: 'invite-token' };
        fixture = TestBed.createComponent(AcceptInvite);
        fixture.detectChanges();
        flushUnauthenticatedSession(fixture);

        const input = fixture.nativeElement.querySelector('input[type="password"]') as HTMLInputElement;
        input.value = 'password123';
        input.dispatchEvent(new Event('input'));
        fixture.detectChanges();

        const form = fixture.nativeElement.querySelector('form') as HTMLFormElement;
        form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));

        const request = TestBed.inject(HttpTestingController).expectOne('/api/auth/accept-invite');
        expect(request.request.method).toBe('POST');
        expect(request.request.body).toEqual({ token: 'invite-token', password: 'password123' });
        request.flush({ status: 'ok' });
        fixture.detectChanges();

        expect(TestBed.inject(AuthService).isAuthenticated()).toBe(true);
        expect(navigateSpy).toHaveBeenCalledWith('/dashboard');
    });

    it('accepts an invitation with only the token when the user is already signed in', () => {
        const navigateSpy = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
        TestBed.inject(AuthService).markAuthenticated();
        queryParams = { token: 'existing-user-token' };
        fixture = TestBed.createComponent(AcceptInvite);
        fixture.detectChanges();

        const request = TestBed.inject(HttpTestingController).expectOne('/api/auth/accept-invite');
        expect(request.request.method).toBe('POST');
        expect(request.request.body).toEqual({ token: 'existing-user-token' });
        request.flush({ status: 'ok' });
        fixture.detectChanges();

        TestBed.inject(HttpTestingController).expectNone('/api/auth/session');
        expect(navigateSpy).toHaveBeenCalledWith('/dashboard');
    });

    it('shows a sign-in CTA when an existing user invite requires login', () => {
        const navigateSpy = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        queryParams = { token: 'existing-user-token' };
        fixture = TestBed.createComponent(AcceptInvite);
        fixture.detectChanges();
        flushUnauthenticatedSession(fixture);

        enterPasswordAndSubmit(fixture, 'password123');

        const request = TestBed.inject(HttpTestingController).expectOne('/api/auth/accept-invite');
        expect(request.request.body).toEqual({ token: 'existing-user-token', password: 'password123' });
        request.flush('Sign in to accept this invitation', { status: 401, statusText: 'Unauthorized' });
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Sign in with the email address that received this invite to accept it.');
        const signInButton = fixture.nativeElement.querySelector('button[type="button"]') as HTMLButtonElement;
        signInButton.click();

        expect(navigateSpy).toHaveBeenCalledWith(['/login'], {
            queryParams: { returnUrl: '/accept-invite?token=existing-user-token' }
        });
    });

    it('shows an expired invitation error for bad invite tokens', () => {
        queryParams = { token: 'expired-token' };
        fixture = TestBed.createComponent(AcceptInvite);
        fixture.detectChanges();
        flushUnauthenticatedSession(fixture);

        enterPasswordAndSubmit(fixture, 'password123');

        const request = TestBed.inject(HttpTestingController).expectOne('/api/auth/accept-invite');
        request.flush('Invalid or expired link', { status: 400, statusText: 'Bad Request' });
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('This invitation link has expired or is invalid.');
    });

    it('shows cloud team-limit guidance when the invite cannot be accepted', () => {
        queryParams = { token: 'team-limit-token' };
        fixture = TestBed.createComponent(AcceptInvite);
        fixture.detectChanges();
        flushUnauthenticatedSession(fixture);

        enterPasswordAndSubmit(fixture, 'password123');

        const request = TestBed.inject(HttpTestingController).expectOne('/api/auth/accept-invite');
        request.flush('Managed cloud accounts are limited to one team', { status: 403, statusText: 'Forbidden' });
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('This invite cannot be accepted because the account is already linked to another team.');
    });

    it('shows a generic invitation error for unexpected failures', () => {
        queryParams = { token: 'server-error-token' };
        fixture = TestBed.createComponent(AcceptInvite);
        fixture.detectChanges();
        flushUnauthenticatedSession(fixture);

        enterPasswordAndSubmit(fixture, 'password123');

        const request = TestBed.inject(HttpTestingController).expectOne('/api/auth/accept-invite');
        request.flush('Internal server error', { status: 500, statusText: 'Internal Server Error' });
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('We could not accept this invitation. Please try again.');
    });
});

function flushUnauthenticatedSession(fixture: ComponentFixture<AcceptInvite>): void {
    const request = TestBed.inject(HttpTestingController).expectOne('/api/auth/session');
    expect(request.request.method).toBe('GET');
    request.flush('Unauthorized', { status: 401, statusText: 'Unauthorized' });
    fixture.detectChanges();
}

function enterPasswordAndSubmit(fixture: ComponentFixture<AcceptInvite>, password: string): void {
    const input = fixture.nativeElement.querySelector('input[type="password"]') as HTMLInputElement;
    input.value = password;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();

    const form = fixture.nativeElement.querySelector('form') as HTMLFormElement;
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
}
