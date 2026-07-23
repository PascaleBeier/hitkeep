import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { CustomTrackingDomain } from '@models/analytics.types';
import { TeamService } from '@services/team.service';
import { TeamTrackingDomains } from './team-tracking-domains';

describe('TeamTrackingDomains', () => {
    let fixture: ComponentFixture<TeamTrackingDomains>;

    const domain: CustomTrackingDomain = {
        id: 'domain-1',
        team_id: 'team-1',
        hostname: 'analytics.example.com',
        verification_status: 'pending',
        target_status: 'pending',
        tls_mode: 'external',
        tls_status: 'pending',
        enabled: true,
        active: false,
        dns_txt_name: '_hitkeep-tracking.analytics.example.com',
        dns_txt_value: 'hitkeep-domain-verification=hk-verify-token',
        dns_target: 'localhost',
        created_at: '2026-07-06T00:00:00Z',
        updated_at: '2026-07-06T00:00:00Z'
    };

    const teamServiceMock = {
        listTrackingDomains: vi.fn(() => of<CustomTrackingDomain[]>([])),
        createTrackingDomain: vi.fn((teamID: string, payload: { hostname: string }) => {
            void teamID;
            void payload;
            return of(domain);
        }),
        verifyTrackingDomain: vi.fn(),
        updateTrackingDomain: vi.fn(),
        deleteTrackingDomain: vi.fn()
    };

    beforeEach(async () => {
        vi.clearAllMocks();

        await TestBed.configureTestingModule({
            imports: [
                TeamTrackingDomains,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            admin: {
                                team: {
                                    settings: {
                                        trackingDomains: {
                                            title: 'Tracking domains',
                                            description: 'Use custom tracker hosts.',
                                            hostnamePlaceholder: 'analytics.example.com',
                                            hostnameRequired: 'Enter a hostname.',
                                            hostnameTooLong: 'Hostnames can be at most 253 characters.',
                                            hostnameInvalid: 'Use a valid hostname.',
                                            addAction: 'Add domain',
                                            empty: 'No tracking domains have been added yet.',
                                            added: 'Tracking domain added.',
                                            enabled: 'Enabled',
                                            disabled: 'Disabled',
                                            active: 'Active',
                                            notActive: 'Not active',
                                            checksLabel: 'Verification checks',
                                            status: {
                                                pending: 'Pending',
                                                verified: 'Verified',
                                                failed: 'Failed'
                                            },
                                            tlsMode: {
                                                external: 'External TLS',
                                                'caddy-on-demand': 'Caddy on-demand TLS'
                                            },
                                            dns: {
                                                txtName: 'TXT name',
                                                txtValue: 'TXT value',
                                                target: 'Target'
                                            },
                                            checks: {
                                                ownership: 'Ownership',
                                                target: 'Target',
                                                tls: 'TLS'
                                            }
                                        }
                                    }
                                }
                            },
                            common: {
                                actions: {
                                    refresh: 'Refresh',
                                    delete: 'Delete'
                                },
                                loading: 'Loading...'
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
            providers: [{ provide: TeamService, useValue: teamServiceMock }, ConfirmationService]
        }).compileComponents();

        fixture = TestBed.createComponent(TeamTrackingDomains);
        fixture.componentRef.setInput('teamId', 'team-1');
        fixture.detectChanges();
    });

    afterEach(() => {
        document.querySelectorAll('.p-dialog-mask, .p-dialog').forEach((element) => element.remove());
    });

    it('adds a tracking domain through the add dialog and opens the DNS setup popup', async () => {
        const addButton = Array.from(fixture.nativeElement.querySelectorAll('button') as NodeListOf<HTMLButtonElement>).find((button) => button.textContent?.includes('Add domain'));
        expect(addButton).toBeTruthy();
        addButton?.click();
        fixture.detectChanges();
        await fixture.whenStable();

        const input = document.body.querySelector('#team-tracking-domain-hostname') as HTMLInputElement;
        expect(input).toBeTruthy();
        input.value = 'analytics.example.com';
        input.dispatchEvent(new Event('input'));
        fixture.detectChanges();

        const form = input.closest('form') as HTMLFormElement;
        const event = new Event('submit', { bubbles: true, cancelable: true });
        form.dispatchEvent(event);
        fixture.detectChanges();
        await fixture.whenStable();

        expect(event.defaultPrevented).toBe(true);
        expect(teamServiceMock.createTrackingDomain).toHaveBeenCalledWith('team-1', {
            hostname: 'analytics.example.com'
        });
        expect(fixture.nativeElement.textContent).toContain('Tracking domain added.');
        expect(fixture.nativeElement.textContent).toContain('analytics.example.com');
        // Successful create opens the DNS setup dialog with the records to copy.
        expect(document.body.textContent).toContain('_hitkeep-tracking.analytics.example.com');
    });

    it('shows the domain in the table with validity icons', async () => {
        teamServiceMock.listTrackingDomains.mockReturnValue(of([domain]));
        fixture.componentInstance['loadDomains']('team-1');
        fixture.detectChanges();

        const hostnameCell = fixture.nativeElement.querySelector('td');
        expect(hostnameCell?.textContent).toContain('analytics.example.com');
        expect(fixture.nativeElement.querySelectorAll('i.hk-status-icon').length).toBeGreaterThanOrEqual(4);
    });
});
