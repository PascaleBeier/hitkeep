import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { ReportConfirmation } from './report-confirmation';

describe('ReportConfirmation', () => {
    let fixture: ComponentFixture<ReportConfirmation>;
    let http: HttpTestingController;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                ReportConfirmation,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: { loading: 'Loading' },
                            reportConfirmation: {
                                title: 'Confirm scheduled report',
                                subtitle: 'Review consent',
                                team: 'Team',
                                report: 'Report',
                                cadence: 'Cadence',
                                sites: 'Sites',
                                disclaimer: 'No account access',
                                confirm: 'Confirm reports',
                                decline: 'Decline',
                                invalid: { title: 'Unavailable', description: 'Ask for a resend' },
                                confirmed: { title: 'Confirmed', description: 'Future reports only' },
                                declined: { title: 'Declined', description: 'No reports' }
                            },
                            settings: { reports: { frequency: { daily: { label: 'Daily' } } } }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }
                })
            ],
            providers: [provideHttpClient(), provideHttpClientTesting(), { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: convertToParamMap({ token: 'opaque-token' }) } } }]
        }).compileComponents();
        fixture = TestBed.createComponent(ReportConfirmation);
        http = TestBed.inject(HttpTestingController);
    });

    afterEach(() => http.verify());

    it('loads non-sensitive metadata without mutating consent, then confirms by POST', () => {
        fixture.detectChanges();
        const get = http.expectOne('/api/report-recipient-confirmations/opaque-token');
        expect(get.request.method).toBe('GET');
        get.flush({
            report_name: 'Client pulse',
            team_name: 'Agency',
            preset: 'site_summary',
            schedule: { frequency: 'daily', timezone: 'UTC', local_time: '08:00' },
            sites: [{ id: 'site-1', domain: 'client.test' }],
            expires_at: '2026-07-26T08:00:00Z'
        });
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Client pulse');
        expect(fixture.nativeElement.textContent).toContain('client.test');
        expect(http.match((request) => request.method === 'POST').length).toBe(0);

        (fixture.componentInstance as unknown as { decide(action: 'confirm'): void }).decide('confirm');
        const post = http.expectOne('/api/report-recipient-confirmations/opaque-token');
        expect(post.request.method).toBe('POST');
        expect(post.request.body).toEqual({ action: 'confirm' });
        post.flush(null);
        fixture.detectChanges();
        expect(fixture.nativeElement.textContent).toContain('Future reports only');
    });

    it('shows the same generic state for unavailable tokens', () => {
        fixture.detectChanges();
        http.expectOne('/api/report-recipient-confirmations/opaque-token').flush({ code: 'confirmation_expired' }, { status: 410, statusText: 'Gone' });
        fixture.detectChanges();
        expect(fixture.nativeElement.textContent).toContain('Ask for a resend');
    });
});
