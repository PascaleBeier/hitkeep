import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { IPExclusion, Team } from '@models/analytics.types';
import { CreateExclusionPayload, ExclusionsService } from '@services/exclusions.service';
import { TeamService } from '@services/team.service';
import { TeamExclusionsPage } from './team-exclusions';

describe('TeamExclusionsPage', () => {
    let fixture: ComponentFixture<TeamExclusionsPage>;
    const activeTeam = signal<Team>({ id: 'team-1', name: 'Acme', logo_url: '', role: 'owner', created_at: '2026-01-01T00:00:00Z' });
    const inheritedRule: IPExclusion = {
        id: 'instance-rule',
        scope: 'instance',
        type: 'user_agent',
        user_agent: 'monitoring-bot',
        inherited: true,
        created_at: '2026-05-02T00:00:00Z'
    };
    const exclusionsService = {
        getCurrentIP: vi.fn(() => of({ ip: '203.0.113.10', cidr: '203.0.113.10/32' })),
        listTeamExclusions: vi.fn((teamID: string, effective: boolean) => {
            void teamID;
            void effective;
            return of([inheritedRule]);
        }),
        createTeamExclusion: vi.fn((_: string, payload: CreateExclusionPayload) =>
            of<IPExclusion>({
                id: 'team-rule',
                scope: 'team',
                type: payload.type ?? 'cidr',
                user_agent: payload.user_agent,
                path: payload.path,
                inherited: false,
                created_at: '2026-05-03T00:00:00Z'
            })
        ),
        deleteTeamExclusion: vi.fn(() => of(undefined))
    };

    beforeEach(async () => {
        vi.clearAllMocks();
        await TestBed.configureTestingModule({
            imports: [
                TeamExclusionsPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                columns: { actions: 'Actions' },
                                actions: { cancel: 'Cancel' },
                                searchPlaceholder: 'Search',
                                copyControl: { copy: 'Copy', copied: 'Copied', failed: 'Copy failed', ariaLabel: 'Copy to clipboard' }
                            },
                            share: { dialog: { deleteAction: 'Delete' } },
                            settings: { apiClients: { actions: { refresh: 'Refresh' } } },
                            admin: {
                                team: {
                                    exclusions: {
                                        title: 'Team traffic filters',
                                        description: 'Filter team traffic.',
                                        addAction: 'Add team filter',
                                        typeLabel: 'Rule type',
                                        cidrLabel: 'IP or CIDR',
                                        cidrPlaceholder: '203.0.113.10/32',
                                        countryLabel: 'Country',
                                        countryPlaceholder: 'Select a country',
                                        userAgentLabel: 'User-agent contains',
                                        userAgentPlaceholder: 'monitoring-bot',
                                        pathLabel: 'Path',
                                        pathPlaceholder: '/admin',
                                        descriptionLabel: 'Description',
                                        descriptionPlaceholder: 'Office',
                                        suggestionsTitle: 'Your current IP',
                                        currentIpLoading: 'Loading current IP',
                                        currentIpUnavailable: 'Current IP unavailable',
                                        inheritedHint: 'Instance filters also apply.',
                                        inheritedReadOnly: 'Inherited',
                                        loading: 'Loading',
                                        empty: 'No filters',
                                        confirmDelete: 'Delete {{value}}?',
                                        ruleTypes: { cidr: 'IP/CIDR', country: 'Country', userAgent: 'User agent', path: 'Path' },
                                        scopes: { instance: 'Instance', team: 'Team', site: 'Site' },
                                        columns: { type: 'Type', value: 'Value', description: 'Description', created: 'Created' },
                                        errors: {
                                            invalidCidr: 'Invalid CIDR',
                                            invalidCountry: 'Select a country',
                                            invalidUserAgent: 'Enter a user agent',
                                            invalidPath: 'Enter a path',
                                            descriptionTooLong: 'Too long',
                                            loadFailed: 'Load failed',
                                            createFailed: 'Create failed',
                                            deleteFailed: 'Delete failed'
                                        },
                                        status: { createSuccess: 'Created {{value}}', deleteSuccess: 'Deleted {{value}}' }
                                    }
                                }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ],
            providers: [provideTranslocoLocale({ langToLocaleMapping: { en: 'en-US' } }), { provide: TeamService, useValue: { activeTeam } }, { provide: ExclusionsService, useValue: exclusionsService }]
        }).compileComponents();

        fixture = TestBed.createComponent(TeamExclusionsPage);
        fixture.detectChanges();
    });

    afterEach(() => {
        document.querySelectorAll('.p-dialog-mask, .p-confirm-dialog, .table-row-actions-menu').forEach((element) => element.remove());
    });

    it('loads effective team filters and renders inherited rules without actions', () => {
        expect(exclusionsService.listTeamExclusions).toHaveBeenCalledWith('team-1', true);
        expect(fixture.nativeElement.textContent).toContain('monitoring-bot');
        expect(fixture.nativeElement.textContent).toContain('Instance');
        expect(fixture.nativeElement.textContent).toContain('Inherited');
        expect(fixture.nativeElement.querySelector('app-table-row-actions')).toBeNull();
        expect(fixture.componentInstance['ruleActions'](inheritedRule)).toEqual([]);
    });

    it('validates and creates user-agent and path filters with type-specific values', () => {
        const form = fixture.componentInstance['form'];

        form.controls.type.setValue('user_agent');
        form.controls.userAgent.setValue('   ');
        fixture.componentInstance['addRule']();
        expect(exclusionsService.createTeamExclusion).not.toHaveBeenCalled();
        expect(form.controls.userAgent.hasError('required')).toBe(true);

        form.controls.userAgent.setValue('HealthCheck');
        fixture.componentInstance['addRule']();
        expect(exclusionsService.createTeamExclusion.mock.calls.at(-1)).toEqual([
            'team-1',
            {
                type: 'user_agent',
                cidr: undefined,
                country_code: undefined,
                user_agent: 'HealthCheck',
                path: undefined,
                description: ''
            }
        ]);

        form.controls.type.setValue('path');
        form.controls.path.setValue('/admin//users/');
        fixture.componentInstance['addRule']();
        expect(exclusionsService.createTeamExclusion.mock.calls.at(-1)).toEqual([
            'team-1',
            {
                type: 'path',
                cidr: undefined,
                country_code: undefined,
                user_agent: undefined,
                path: '/admin//users/',
                description: ''
            }
        ]);
    });
});
