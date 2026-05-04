import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { SiteExclusionSettings } from './site-exclusion-settings';
import { ExclusionsService } from '@services/exclusions.service';
import { Site } from '@models/analytics.types';

describe('SiteExclusionSettings', () => {
    let fixture: ComponentFixture<SiteExclusionSettings>;

    const exclusionsService = {
        getCurrentIP: vi.fn(() => of({ ip: '203.0.113.10', cidr: '203.0.113.10/32' })),
        listSiteExclusions: vi.fn(() => of([])),
        createSiteExclusion: vi.fn(),
        deleteSiteExclusion: vi.fn()
    };

    const site: Site = {
        id: 'site-1',
        user_id: 'user-1',
        domain: 'example.com',
        created_at: '2026-05-01T00:00:00Z'
    };

    beforeEach(async () => {
        vi.clearAllMocks();

        await TestBed.configureTestingModule({
            imports: [
                SiteExclusionSettings,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                columns: { actions: 'Actions' },
                                actions: { cancel: 'Cancel' },
                                searchPlaceholder: 'Search',
                                copyControl: {
                                    copy: 'Copy',
                                    copied: 'Copied',
                                    failed: 'Copy failed',
                                    ariaLabel: 'Copy to clipboard'
                                }
                            },
                            share: { dialog: { deleteAction: 'Delete' } },
                            sites: {
                                settings: { tabs: { filtering: 'Filtering' } },
                                exclusions: {
                                    suggestionsTitle: 'Suggestions',
                                    suggestionsHint: 'Use your current IP.',
                                    currentIpLoading: 'Loading current IP',
                                    currentIpUnavailable: 'Current IP unavailable',
                                    addAction: 'Add exclusion',
                                    description: 'Exclude traffic.',
                                    cidrLabel: 'CIDR',
                                    cidrPlaceholder: '203.0.113.10/32',
                                    descriptionLabel: 'Description',
                                    descriptionPlaceholder: 'Office',
                                    loading: 'Loading',
                                    empty: 'No exclusions',
                                    confirmDelete: 'Delete {{cidr}}?',
                                    columns: {
                                        cidr: 'CIDR',
                                        description: 'Description',
                                        created: 'Created'
                                    },
                                    errors: {
                                        invalidCidr: 'Invalid CIDR',
                                        descriptionTooLong: 'Too long',
                                        loadFailed: 'Load failed',
                                        createFailed: 'Create failed',
                                        deleteFailed: 'Delete failed'
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
            providers: [{ provide: ExclusionsService, useValue: exclusionsService }]
        }).compileComponents();

        fixture = TestBed.createComponent(SiteExclusionSettings);
        fixture.componentRef.setInput('site', site);
        fixture.detectChanges();
    });

    it('renders the current IP with the shared copy control', () => {
        const text = fixture.nativeElement.textContent as string;
        const copyButton = fixture.nativeElement.querySelector('app-copy-control button') as HTMLButtonElement | null;

        expect(text).toContain('203.0.113.10/32');
        expect(copyButton).not.toBeNull();
        expect(copyButton?.textContent).toContain('Copy');
        expect(copyButton?.disabled).toBe(false);
    });
});
