import { inject, signal, Service } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { finalize, tap } from 'rxjs';
import { ReportDefinition, ReportDefinitionInput, ReportPreview, ReportRecipientConfirmation, ReportRun } from '@models/analytics.types';

@Service()
export class ReportDefinitionsService {
    private readonly http = inject(HttpClient);

    readonly reports = signal<ReportDefinition[]>([]);
    readonly isLoading = signal(false);

    load() {
        this.isLoading.set(true);
        return this.http.get<ReportDefinition[]>('/api/reports').pipe(
            tap((reports) => this.reports.set(reports)),
            finalize(() => this.isLoading.set(false))
        );
    }

    create(definition: ReportDefinitionInput) {
        return this.http.post<ReportDefinition>('/api/reports', definition).pipe(tap((report) => this.reports.update((reports) => [report, ...reports])));
    }

    update(reportID: string, definition: Partial<Omit<ReportDefinitionInput, 'scope' | 'tenant_id'>>) {
        return this.http.patch<ReportDefinition>(`/api/reports/${encodeURIComponent(reportID)}`, definition).pipe(tap((report) => this.reports.update((reports) => reports.map((item) => (item.id === report.id ? report : item)))));
    }

    delete(reportID: string) {
        return this.http.delete<void>(`/api/reports/${encodeURIComponent(reportID)}`).pipe(tap(() => this.reports.update((reports) => reports.filter((report) => report.id !== reportID))));
    }

    preview(definition: ReportDefinitionInput, reportID?: string) {
        return this.http.post<ReportPreview>('/api/reports/preview', { definition, report_id: reportID });
    }

    testSend(reportID: string) {
        return this.http.post<{ status: string; message_id: string; sent_at: string }>(`/api/reports/${encodeURIComponent(reportID)}/test-send`, {});
    }

    runs(reportID: string) {
        return this.http.get<ReportRun[]>(`/api/reports/${encodeURIComponent(reportID)}/runs`);
    }

    retry(runID: string) {
        return this.http.post<void>(`/api/report-runs/${encodeURIComponent(runID)}/retry`, {});
    }

    resubscribe(reportID: string) {
        return this.http.post<void>(`/api/reports/${encodeURIComponent(reportID)}/resubscribe`, {}).pipe(
            tap(() => {
                this.reports.update((reports) => reports.map((report) => (report.id === reportID ? { ...report, recipients: report.recipients.map((recipient) => ({ ...recipient, opted_out_at: undefined })) } : report)));
            })
        );
    }

    resendConfirmation(reportID: string, recipientID: string) {
        return this.http
            .post<void>(`/api/reports/${encodeURIComponent(reportID)}/recipients/${encodeURIComponent(recipientID)}/confirmation`, {})
            .pipe(
                tap(() =>
                    this.reports.update((reports) =>
                        reports.map((report) => (report.id === reportID ? { ...report, recipients: report.recipients.map((recipient) => (recipient.id === recipientID ? { ...recipient, invitation_state: 'sent' as const } : recipient)) } : report))
                    )
                )
            );
    }

    confirmation(token: string) {
        return this.http.get<ReportRecipientConfirmation>(`/api/report-recipient-confirmations/${encodeURIComponent(token)}`);
    }

    decideConfirmation(token: string, action: 'confirm' | 'decline') {
        return this.http.post<void>(`/api/report-recipient-confirmations/${encodeURIComponent(token)}`, { action });
    }
}
