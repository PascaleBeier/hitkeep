import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { finalize } from 'rxjs';

import { AuditTableComponent } from '@components/audit-table/audit-table';
import { AuditTableExportStatus, AuditTableQuery } from '@components/audit-table/audit-table.types';
import { AdminSystemService, InstanceAuditEntry, InstanceAuditListResponse } from '@services/admin-system.service';
import { AuditPresentationService } from '@services/audit-presentation.service';

@Component({
    selector: 'app-system-audit',
    imports: [AuditTableComponent],
    templateUrl: './system-audit.html',
    styleUrl: './system-audit.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SystemAudit implements OnInit {
    private readonly system = inject(AdminSystemService);
    private readonly presentation = inject(AuditPresentationService);
    private auditRequestID = 0;

    protected readonly entries = signal<InstanceAuditEntry[]>([]);
    protected readonly total = signal(0);
    protected readonly query = signal<AuditTableQuery>({
        limit: 25,
        offset: 0
    });
    protected readonly isLoading = signal(false);
    protected readonly isExporting = signal(false);
    protected readonly hasError = signal(false);
    protected readonly exportStatus = signal<AuditTableExportStatus | null>(null);

    protected readonly actionOptions = computed(() => this.presentation.actionOptions('system'));
    protected readonly outcomeOptions = computed(() => this.presentation.outcomeOptions());
    protected readonly targetTypeOptions = computed(() => this.presentation.targetTypeOptions());

    ngOnInit() {
        this.loadEntries(this.query());
    }

    protected onQueryChange(query: AuditTableQuery) {
        const nextQuery = this.normalizeQuery(query);
        this.query.set(nextQuery);
        this.exportStatus.set(null);
        this.loadEntries(nextQuery);
    }

    protected refresh() {
        this.loadEntries(this.query());
    }

    protected exportAudit() {
        this.exportStatus.set(null);
        this.isExporting.set(true);
        this.system
            .exportAudit(this.query())
            .pipe(finalize(() => this.isExporting.set(false)))
            .subscribe({
                next: (blob) => {
                    this.downloadAuditBlob(blob);
                    this.exportStatus.set({ severity: 'success', key: 'admin.system.audit.exportSuccess' });
                },
                error: () => {
                    this.exportStatus.set({ severity: 'error', key: 'admin.system.audit.exportFailed' });
                }
            });
    }

    private loadEntries(query: AuditTableQuery) {
        const requestID = ++this.auditRequestID;
        this.isLoading.set(true);
        this.hasError.set(false);
        this.system
            .listAudit(query)
            .pipe(
                finalize(() => {
                    if (requestID === this.auditRequestID) {
                        this.isLoading.set(false);
                    }
                })
            )
            .subscribe({
                next: (resp) => {
                    if (requestID !== this.auditRequestID) {
                        return;
                    }
                    this.onData(resp);
                },
                error: () => {
                    if (requestID !== this.auditRequestID) {
                        return;
                    }
                    this.hasError.set(true);
                }
            });
    }

    private onData(resp: InstanceAuditListResponse) {
        this.entries.set(resp.entries);
        this.total.set(resp.total);
        this.query.update((current) => ({
            ...current,
            limit: resp.limit,
            offset: resp.offset
        }));
    }

    private downloadAuditBlob(blob: Blob) {
        const date = new Date().toISOString().slice(0, 10);
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `instance-audit-${date}.json`;
        a.click();
        window.URL.revokeObjectURL(url);
    }

    private normalizeQuery(query: AuditTableQuery): AuditTableQuery {
        return {
            ...query,
            limit: query.limit || 25,
            offset: query.offset || 0
        };
    }
}
