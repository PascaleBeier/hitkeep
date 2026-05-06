import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { UserPreferencesService } from '@services/user-preferences.service';
import { UserProfileService } from '@services/user-profile.service';
import { UserSettings } from './user-settings';

describe('UserSettings', () => {
    let fixture: ComponentFixture<UserSettings>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                UserSettings,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { account: 'Account' },
                            common: { actions: { copy: 'Copy' } },
                            preferences: {
                                language: {
                                    heading: 'Language',
                                    description: 'Choose dashboard language.',
                                    defaultLabel: 'Default language',
                                    defaultPlaceholder: 'Select language',
                                    defaultHint: 'Used across the dashboard.',
                                    rtlBadge: 'RTL'
                                },
                                actions: { saveChanges: 'Save changes' },
                                status: { saved: 'Preferences saved', saveFailed: 'Could not save preferences' }
                            },
                            settings: {
                                security: { title: 'Security' },
                                user: {
                                    breadcrumb: 'User settings',
                                    profile: {
                                        title: 'Profile',
                                        description: 'Update your personal details.',
                                        emailLabel: 'Email address',
                                        emailPlaceholder: 'you@example.com',
                                        givenNameLabel: 'Given name',
                                        givenNamePlaceholder: 'Ada',
                                        lastNameLabel: 'Last name',
                                        lastNamePlaceholder: 'Lovelace',
                                        userIdLabel: 'User ID',
                                        saveAction: 'Save profile',
                                        status: { saved: 'Profile saved' },
                                        validation: {
                                            emailRequired: 'Email is required.',
                                            emailInvalid: 'Please enter a valid email address.',
                                            emailTooLong: 'Email must be 320 characters or fewer.',
                                            nameTooLong: 'Names must be 120 characters or fewer.'
                                        },
                                        errors: {
                                            invalidInput: 'Please check the highlighted fields and try again.',
                                            emailTaken: 'That email is already in use.',
                                            notFound: 'Your account could not be found.',
                                            updateFailed: 'Failed to update profile. Please try again.'
                                        }
                                    },
                                    exportShortcut: {
                                        title: 'Analytics export',
                                        description: 'Export all accessible sites or individual site takeouts from the Import & Export hub.',
                                        action: 'Open Import & Export'
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
                provideRouter([]),
                {
                    provide: UserPreferencesService,
                    useValue: {
                        preferences: signal({ default_locale: 'en' }),
                        isLoading: signal(false),
                        isSaving: signal(false),
                        save: vi.fn()
                    }
                },
                {
                    provide: UserProfileService,
                    useValue: {
                        profile: signal({
                            id: 'user-1',
                            email: 'demo@example.com',
                            given_name: 'Demo',
                            last_name: 'User',
                            display_name: 'Demo User',
                            avatar_url: ''
                        }),
                        isLoading: signal(false),
                        isSaving: signal(false),
                        updateProfile: vi.fn()
                    }
                }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(UserSettings);
        fixture.detectChanges();
    });

    it('replaces the user export tab with an account shortcut to the Import & Export hub', () => {
        const text = fixture.nativeElement.textContent;
        const links = Array.from(fixture.nativeElement.querySelectorAll('a')) as HTMLAnchorElement[];

        expect(text).toContain('Analytics export');
        expect(text).toContain('Export all accessible sites or individual site takeouts from the Import & Export hub.');
        expect(links.some((link) => link.getAttribute('href') === '/import-export/export')).toBe(true);
        expect(fixture.nativeElement.querySelector('[role="tab"][value="export"]')).toBeNull();
        expect(fixture.nativeElement.querySelector('p-splitbutton')).toBeNull();
    });
});
