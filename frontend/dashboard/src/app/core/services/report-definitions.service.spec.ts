import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { ReportDefinition, ReportDefinitionInput } from '@models/analytics.types';
import { ReportDefinitionsService } from './report-definitions.service';

describe('ReportDefinitionsService', () => {
    let service: ReportDefinitionsService;
    let httpMock: HttpTestingController;

    const definition: ReportDefinitionInput = {
        name: 'Morning summary',
        scope: 'personal',
        preset: 'site_summary',
        site_mode: 'selected',
        site_ids: ['site-1'],
        recipient_user_ids: ['user-1'],
        external_recipient_emails: [],
        schedule: { frequency: 'daily', timezone: 'Europe/Berlin', local_time: '08:15' },
        status: 'active'
    };

    const report: ReportDefinition = {
        id: 'report-1',
        owner_user_id: 'user-1',
        name: definition.name,
        scope: definition.scope,
        preset: definition.preset,
        site_mode: definition.site_mode,
        sites: [{ id: 'site-1', domain: 'example.test' }],
        recipients: [{ id: 'recipient-1', kind: 'member', user_id: 'user-1', email: 'owner@example.test', status: 'confirmed' }],
        schedule: definition.schedule,
        status: definition.status,
        source: 'v2',
        consent_version: 1,
        next_run_at: '2026-07-19T06:15:00Z',
        created_at: '2026-07-18T04:00:00Z',
        updated_at: '2026-07-18T04:00:00Z'
    };

    beforeEach(() => {
        TestBed.configureTestingModule({ providers: [ReportDefinitionsService, provideHttpClient(), provideHttpClientTesting()] });
        service = TestBed.inject(ReportDefinitionsService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => httpMock.verify());

    it('loads named reports and tracks loading state', () => {
        service.load().subscribe();
        expect(service.isLoading()).toBe(true);
        const req = httpMock.expectOne('/api/reports');
        expect(req.request.method).toBe('GET');
        req.flush([report]);
        expect(service.reports()).toEqual([report]);
        expect(service.isLoading()).toBe(false);
    });

    it('creates and updates a scheduled report', () => {
        service.create(definition).subscribe();
        const create = httpMock.expectOne('/api/reports');
        expect(create.request.method).toBe('POST');
        expect(create.request.body.schedule.local_time).toBe('08:15');
        create.flush(report);

        service.update(report.id, { schedule: { ...report.schedule, local_time: '09:30' } }).subscribe();
        const update = httpMock.expectOne('/api/reports/report-1');
        expect(update.request.method).toBe('PATCH');
        expect(update.request.body.schedule.local_time).toBe('09:30');
        update.flush({ ...report, schedule: { ...report.schedule, local_time: '09:30' } });
        expect(service.reports()[0]?.schedule.local_time).toBe('09:30');
    });

    it('uses preview, test-send, history, retry, and resubscribe endpoints', () => {
        service.preview(definition).subscribe();
        const preview = httpMock.expectOne('/api/reports/preview');
        expect(preview.request.body).toEqual({ definition, report_id: undefined });
        preview.flush({});

        service.testSend(report.id).subscribe();
        httpMock.expectOne('/api/reports/report-1/test-send').flush({ status: 'accepted' });

        service.runs(report.id).subscribe();
        httpMock.expectOne('/api/reports/report-1/runs').flush([]);

        service.retry('run-1').subscribe();
        httpMock.expectOne('/api/report-runs/run-1/retry').flush(null);

        service.reports.set([{ ...report, recipients: [{ ...report.recipients[0]!, opted_out_at: '2026-07-18T04:00:00Z' }] }]);
        service.resubscribe(report.id).subscribe();
        httpMock.expectOne('/api/reports/report-1/resubscribe').flush(null);
        expect(service.reports()[0]?.recipients[0]?.opted_out_at).toBeUndefined();

        service.resendConfirmation(report.id, 'recipient-1').subscribe();
        httpMock.expectOne('/api/reports/report-1/recipients/recipient-1/confirmation').flush(null);
        expect(service.reports()[0]?.recipients[0]?.invitation_state).toBe('sent');
    });
});
