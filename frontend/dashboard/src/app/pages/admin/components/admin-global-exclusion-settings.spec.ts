import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of, Subject, throwError } from 'rxjs';
import { vi } from 'vitest';

import { AdminGlobalExclusionSettings } from './admin-global-exclusion-settings';
import { ExclusionsService } from '@services/exclusions.service';
import { IPExclusion } from '@models/analytics.types';

describe('AdminGlobalExclusionSettings', () => {
    let fixture: ComponentFixture<AdminGlobalExclusionSettings>;

    const exclusionsService = {
        getCurrentIP: vi.fn(() => of({ ip: '203.0.113.10', cidr: '203.0.113.10/32' })),
        listInstanceExclusions: vi.fn(() => of([])),
        createInstanceExclusion: vi.fn(() =>
            of<IPExclusion>({
                id: 'rule-1',
                type: 'cidr' as const,
                cidr: '203.0.113.10/32',
                description: 'Office',
                created_at: '2026-05-01T00:00:00Z'
            })
        ),
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
                                    typeLabel: 'Rule type',
                                    cidrLabel: 'CIDR',
                                    cidrPlaceholder: '203.0.113.10/32',
                                    countryLabel: 'Country',
                                    countryPlaceholder: 'Select a country',
                                    descriptionLabel: 'Description',
                                    descriptionPlaceholder: 'Office',
                                    loading: 'Loading',
                                    empty: 'No filters',
                                    confirmDelete: 'Delete {{value}}?',
                                    ruleTypes: { cidr: 'IP/CIDR', country: 'Country' },
                                    columns: {
                                        type: 'Type',
                                        value: 'Value',
                                        description: 'Description',
                                        created: 'Created'
                                    },
                                    status: {
                                        createSuccess: 'Created {{value}}',
                                        deleteSuccess: 'Deleted {{value}}'
                                    },
                                    errors: {
                                        invalidCidr: 'Invalid CIDR',
                                        invalidCountry: 'Select a country',
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
            providers: [
                provideTranslocoLocale({
                    langToLocaleMapping: {
                        en: 'en-US'
                    }
                }),
                { provide: ExclusionsService, useValue: exclusionsService }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(AdminGlobalExclusionSettings);
        fixture.detectChanges();
    });

    afterEach(() => {
        document.querySelectorAll('.p-dialog-mask, .p-confirm-dialog, .table-row-actions-menu').forEach((element) => element.remove());
    });

    it('renders the current IP with the shared copy control', () => {
        const text = fixture.nativeElement.textContent as string;
        const copyButton = fixture.nativeElement.querySelector('app-copy-control button') as HTMLButtonElement | null;

        expect(text).toContain('203.0.113.10/32');
        expect(copyButton).not.toBeNull();
        expect(copyButton?.textContent).toContain('Copy');
        expect(copyButton?.disabled).toBe(false);
    });

    it('shows the exclusions table surface first and opens add form in a dialog', async () => {
        expect(fixture.nativeElement.querySelector('.admin-exclusions__form')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Add filter');

        clickButton('Add filter');
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.body.querySelector('.admin-exclusions__form')).not.toBeNull();
        expect(document.body.textContent).toContain('CIDR');
    });

    it('creates an exclusion from the dialog and closes it', async () => {
        clickButton('Add filter');
        fixture.detectChanges();
        await fixture.whenStable();

        (document.body.querySelector('#instance-exclusion-cidr') as HTMLInputElement).value = '203.0.113.10/32';
        (document.body.querySelector('#instance-exclusion-cidr') as HTMLInputElement).dispatchEvent(new Event('input'));
        (document.body.querySelector('#instance-exclusion-description') as HTMLInputElement).value = 'Office';
        (document.body.querySelector('#instance-exclusion-description') as HTMLInputElement).dispatchEvent(new Event('input'));

        fixture.componentInstance['addRule']();
        fixture.detectChanges();

        const createCalls = (exclusionsService.createInstanceExclusion as unknown as { mock: { calls: unknown[][] } }).mock.calls;
        expect(createCalls[0]?.[0]).toEqual({
            type: 'cidr',
            cidr: '203.0.113.10/32',
            country_code: undefined,
            description: 'Office'
        });
        expect(fixture.componentInstance['isAddDialogVisible']()).toBe(false);
        expect(fixture.nativeElement.textContent).toContain('203.0.113.10/32');
        expect(fixture.nativeElement.textContent).toContain('Created 203.0.113.10/32');
    });

    it('shows row loading and success feedback while deleting an exclusion', () => {
        const pending = new Subject<void>();
        exclusionsService.deleteInstanceExclusion.mockReturnValueOnce(pending.asObservable());
        fixture.componentInstance['exclusions'].set([
            {
                id: 'rule-1',
                type: 'cidr',
                cidr: '203.0.113.10/32',
                description: 'Office',
                created_at: '2026-05-01T00:00:00Z'
            }
        ]);
        fixture.detectChanges();

        fixture.componentInstance['deleteRule']({
            id: 'rule-1',
            type: 'cidr',
            cidr: '203.0.113.10/32',
            description: 'Office',
            created_at: '2026-05-01T00:00:00Z'
        });
        fixture.detectChanges();

        expect(fixture.componentInstance['deletingRuleID']()).toBe('rule-1');

        pending.next();
        pending.complete();
        fixture.detectChanges();

        expect(fixture.componentInstance['deletingRuleID']()).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('Deleted 203.0.113.10/32');
    });

    it('ignores duplicate create submits while the request is in flight', async () => {
        const pending = new Subject<{ id: string; type: 'cidr'; cidr: string; description: string; created_at: string }>();
        exclusionsService.createInstanceExclusion.mockReturnValueOnce(pending.asObservable());
        clickButton('Add filter');
        fixture.detectChanges();
        await fixture.whenStable();

        (document.body.querySelector('#instance-exclusion-cidr') as HTMLInputElement).value = '203.0.113.10/32';
        (document.body.querySelector('#instance-exclusion-cidr') as HTMLInputElement).dispatchEvent(new Event('input'));

        fixture.componentInstance['addRule']();
        fixture.componentInstance['addRule']();

        expect(exclusionsService.createInstanceExclusion).toHaveBeenCalledTimes(1);
        pending.complete();
    });

    it('creates a country exclusion from the dialog', async () => {
        exclusionsService.createInstanceExclusion.mockReturnValueOnce(
            of<IPExclusion>({
                id: 'rule-country',
                type: 'country' as const,
                country_code: 'DE',
                description: 'Germany',
                created_at: '2026-05-01T00:00:00Z'
            })
        );
        clickButton('Add filter');
        fixture.detectChanges();
        await fixture.whenStable();

        fixture.componentInstance['form'].controls.type.setValue('country');
        fixture.componentInstance['form'].controls.countryCode.setValue('DE');
        fixture.componentInstance['form'].controls.description.setValue('Germany');

        fixture.componentInstance['addRule']();
        fixture.detectChanges();

        const createCalls = (exclusionsService.createInstanceExclusion as unknown as { mock: { calls: unknown[][] } }).mock.calls;
        expect(createCalls.at(-1)?.[0]).toEqual({
            type: 'country',
            cidr: undefined,
            country_code: 'DE',
            description: 'Germany'
        });
        expect(fixture.nativeElement.textContent).toContain('Germany (DE)');
    });

    it('keeps create errors inside the add dialog', async () => {
        exclusionsService.createInstanceExclusion.mockReturnValueOnce(throwError(() => new Error('nope')));
        clickButton('Add filter');
        fixture.detectChanges();
        await fixture.whenStable();

        (document.body.querySelector('#instance-exclusion-cidr') as HTMLInputElement).value = '203.0.113.10/32';
        (document.body.querySelector('#instance-exclusion-cidr') as HTMLInputElement).dispatchEvent(new Event('input'));

        fixture.componentInstance['addRule']();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.body.querySelector('.p-dialog')?.textContent).toContain('Create failed');
        expect(fixture.nativeElement.textContent).not.toContain('Create failed');
    });

    function clickButton(label: string): void {
        const button = Array.from(fixture.nativeElement.querySelectorAll('button') as NodeListOf<HTMLButtonElement>).find((entry) => entry.textContent?.includes(label));
        button?.click();
    }
});
