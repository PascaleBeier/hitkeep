import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { AdminGlobalExclusionSettings } from './admin-global-exclusion-settings';
import { ExclusionsService } from '@services/exclusions.service';

describe('AdminGlobalExclusionSettings', () => {
    let fixture: ComponentFixture<AdminGlobalExclusionSettings>;

    const exclusionsService = {
        getCurrentIP: vi.fn(() => of({ ip: '203.0.113.10', cidr: '203.0.113.10/32' })),
        listInstanceExclusions: vi.fn(() => of([])),
        createInstanceExclusion: vi.fn(),
        deleteInstanceExclusion: vi.fn()
    };

    beforeEach(async () => {
        vi.clearAllMocks();

        await TestBed.configureTestingModule({
            imports: [
                AdminGlobalExclusionSettings,
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
                            settings: { apiClients: { actions: { refresh: 'Refresh' } } },
                            admin: {
                                exclusions: {
                                    suggestionsTitle: 'Suggestions',
                                    currentIpLoading: 'Loading current IP',
                                    currentIpUnavailable: 'Current IP unavailable',
                                    addAction: 'Add filter',
                                    cidrLabel: 'CIDR',
                                    cidrPlaceholder: '203.0.113.10/32',
                                    descriptionLabel: 'Description',
                                    descriptionPlaceholder: 'Office',
                                    loading: 'Loading',
                                    empty: 'No filters',
                                    confirmDelete: 'Delete {{cidr}}?',
                                    columns: {
                                        cidr: 'CIDR',
                                        description: 'Description',
                                        created: 'Created'
                                    },
                                    status: {
                                        deleteSuccess: 'Deleted {{cidr}}'
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

        fixture = TestBed.createComponent(AdminGlobalExclusionSettings);
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
