import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, input, output, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { TranslocoPipe } from '@jsverse/transloco';
import { debounceTime, Subject } from 'rxjs';

import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { AuditPresentationService } from '@services/audit-presentation.service';
import { ButtonModule } from 'primeng/button';
import { DatePickerModule } from 'primeng/datepicker';
import { IconFieldModule } from 'primeng/iconfield';
import { InputIconModule } from 'primeng/inputicon';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';
import { PaginatorModule } from 'primeng/paginator';
import { SelectModule } from 'primeng/select';
import { TableModule } from 'primeng/table';
import { TagModule } from 'primeng/tag';
import { TooltipModule } from 'primeng/tooltip';

import { AuditTableExportStatus, AuditTableFilterConfig, AuditTableOption, AuditTableQuery, AuditTableRow } from './audit-table.types';

interface AuditEvidenceField {
    key: string;
    labelKey: string;
    value: string;
    mono: boolean;
}

interface AuditPaginatorEvent {
    first?: number;
    rows?: number;
}

const DEFAULT_QUERY: AuditTableQuery = {
    limit: 25,
    offset: 0
};

const DEFAULT_FILTER_CONFIG: AuditTableFilterConfig = {
    action: true,
    outcome: true,
    targetType: true,
    dateRange: true,
    search: true
};

let nextAuditTableID = 0;

@Component({
    selector: 'app-audit-table',
    imports: [FormsModule, TableModule, PaginatorModule, ButtonModule, SelectModule, DatePickerModule, IconFieldModule, InputIconModule, InputTextModule, TagModule, MessageModule, TooltipModule, RelativeDateTime, TranslocoPipe],
    templateUrl: './audit-table.html',
    styleUrl: './audit-table.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AuditTableComponent {
    private readonly destroyRef = inject(DestroyRef);
    private readonly presentation = inject(AuditPresentationService);
    private readonly searchSubject = new Subject<string>();

    readonly rows = input<AuditTableRow[]>([]);
    readonly total = input<number>(0);
    readonly loading = input<boolean>(false);
    readonly emptyTextKey = input<string>('auditTable.empty');
    readonly errorKey = input<string | null>(null);
    readonly filterConfig = input<Partial<AuditTableFilterConfig>>({});
    readonly actionOptions = input<AuditTableOption[]>([]);
    readonly outcomeOptions = input<AuditTableOption[]>([]);
    readonly targetTypeOptions = input<AuditTableOption[]>([]);
    readonly pageSizeOptions = input<number[]>([25, 50, 100]);
    readonly query = input<AuditTableQuery>(DEFAULT_QUERY);
    readonly exportEnabled = input<boolean>(false);
    readonly exportLoading = input<boolean>(false);
    readonly exportStatus = input<AuditTableExportStatus | null>(null);

    readonly queryChange = output<AuditTableQuery>();
    readonly refresh = output<void>();
    readonly exportRequest = output<void>();
    readonly rowExpansionChange = output<{ row: AuditTableRow; expanded: boolean }>();

    protected readonly idPrefix = `audit-table-${nextAuditTableID++}`;
    protected readonly searchText = signal('');
    protected readonly dateRange = signal<Date[]>([]);
    protected readonly expandedRowIDs = signal<ReadonlySet<string>>(new Set());
    protected readonly resolvedFilterConfig = computed<AuditTableFilterConfig>(() => ({
        ...DEFAULT_FILTER_CONFIG,
        ...this.filterConfig()
    }));
    protected readonly pageSize = computed(() => this.query().limit || this.pageSizeOptions()[0] || DEFAULT_QUERY.limit);
    protected readonly firstRow = computed(() => this.query().offset || 0);
    protected readonly visibleRangeLabelKey = computed(() => (this.total() > 0 ? 'auditTable.pagination.summary' : 'auditTable.pagination.empty'));
    protected readonly hasActiveFilters = computed(() => {
        const query = this.query();
        return Boolean(query.action || query.outcome || query.target_type || query.query || query.from || query.to);
    });

    constructor() {
        this.searchSubject.pipe(debounceTime(300), takeUntilDestroyed(this.destroyRef)).subscribe((value) => {
            this.updateQuery({
                query: value.trim() || undefined,
                offset: 0
            });
        });

        effect(() => {
            const query = this.query();
            this.searchText.set(query.query ?? '');
            this.dateRange.set(this.dateRangeFromQuery(query));
        });
    }

    protected updateFilter(field: 'action' | 'outcome' | 'target_type', value: string | null | undefined) {
        this.updateQuery({
            [field]: value || undefined,
            offset: 0
        });
    }

    protected onSearchInput(value: string) {
        this.searchText.set(value);
        this.searchSubject.next(value);
    }

    protected onDateRangeChange(value: Date[] | Date | null) {
        const range = Array.isArray(value) ? value.filter(Boolean) : value ? [value] : [];
        this.dateRange.set(range);
        if (range.length === 0) {
            this.updateQuery({ from: undefined, to: undefined, offset: 0 });
            return;
        }
        if (range.length === 2 && range[0] && range[1]) {
            this.updateQuery({
                from: range[0].toISOString(),
                to: range[1].toISOString(),
                offset: 0
            });
        }
    }

    protected clearFilters() {
        this.searchText.set('');
        this.dateRange.set([]);
        this.queryChange.emit({
            limit: this.pageSize(),
            offset: 0
        });
    }

    protected onPageChange(event: AuditPaginatorEvent) {
        this.updateQuery({
            limit: event.rows ?? this.pageSize(),
            offset: event.first ?? 0
        });
    }

    protected toggleRow(row: AuditTableRow) {
        const next = new Set(this.expandedRowIDs());
        const expanded = !next.has(row.id);
        if (expanded) {
            next.add(row.id);
        } else {
            next.delete(row.id);
        }
        this.expandedRowIDs.set(next);
        this.rowExpansionChange.emit({ row, expanded });
    }

    protected isExpanded(row: AuditTableRow): boolean {
        return this.expandedRowIDs().has(row.id);
    }

    protected expandIcon(row: AuditTableRow): string {
        return this.isExpanded(row) ? 'pi pi-chevron-down' : 'pi pi-chevron-right';
    }

    protected expandLabel(row: AuditTableRow): string {
        return this.isExpanded(row) ? 'auditTable.actions.collapseRow' : 'auditTable.actions.expandRow';
    }

    protected actionLabel(row: AuditTableRow): string {
        return this.presentation.actionLabel(row.action);
    }

    protected targetTypeLabel(row: AuditTableRow): string {
        return this.presentation.targetTypeLabel(row.target_type);
    }

    protected targetLabel(row: AuditTableRow): string {
        return this.presentation.targetLabel(row);
    }

    protected actorLabel(row: AuditTableRow): string {
        return this.presentation.actorLabel(row);
    }

    protected roleLabel(role: string | null | undefined): string {
        return this.presentation.roleLabel(role);
    }

    protected outcomeLabel(row: AuditTableRow): string {
        return this.presentation.outcomeLabel(row.outcome);
    }

    protected actionSeverity(row: AuditTableRow) {
        return this.presentation.actionSeverity(row.action);
    }

    protected outcomeSeverity(row: AuditTableRow) {
        return this.presentation.outcomeSeverity(row.outcome);
    }

    protected evidenceFields(row: AuditTableRow): AuditEvidenceField[] {
        return [
            { key: 'actor_id', labelKey: 'auditTable.evidence.actorId', value: row.actor_id || row.actor_user_id || '', mono: true },
            { key: 'actor_role', labelKey: 'auditTable.evidence.actorRole', value: row.actor_role_snapshot ? this.roleLabel(row.actor_role_snapshot) : '', mono: false },
            { key: 'team_id', labelKey: 'auditTable.evidence.teamId', value: row.team_id || '', mono: true },
            { key: 'target_type', labelKey: 'auditTable.evidence.targetType', value: this.presentation.targetTypeLabel(row.target_type), mono: false },
            { key: 'target_id', labelKey: 'auditTable.evidence.targetId', value: row.target_id || '', mono: true },
            { key: 'target_user_id', labelKey: 'auditTable.evidence.targetUserId', value: row.target_user_id || '', mono: true },
            { key: 'request_id', labelKey: 'auditTable.evidence.requestId', value: row.request_id || '', mono: true },
            { key: 'user_agent', labelKey: 'auditTable.evidence.userAgent', value: row.user_agent || '', mono: false }
        ].filter((field) => field.value);
    }

    protected hasEvidence(row: AuditTableRow): boolean {
        return this.evidenceFields(row).length > 0 || Boolean(row.details);
    }

    protected pageSummaryParams() {
        const total = this.total();
        const start = total === 0 ? 0 : this.firstRow() + 1;
        const end = Math.min(this.firstRow() + this.rows().length, total);
        return { start, end, total };
    }

    protected trackEvidenceField(_index: number, field: AuditEvidenceField): string {
        return field.key;
    }

    private updateQuery(patch: Partial<AuditTableQuery>) {
        const current = this.query();
        this.queryChange.emit(
            this.cleanQuery({
                ...current,
                ...patch
            })
        );
    }

    private cleanQuery(query: AuditTableQuery): AuditTableQuery {
        const cleaned: AuditTableQuery = {
            limit: query.limit || this.pageSizeOptions()[0] || DEFAULT_QUERY.limit,
            offset: query.offset || 0
        };
        if (query.action) cleaned.action = query.action;
        if (query.outcome) cleaned.outcome = query.outcome;
        if (query.target_type) cleaned.target_type = query.target_type;
        if (query.query?.trim()) cleaned.query = query.query.trim();
        if (query.from) cleaned.from = query.from;
        if (query.to) cleaned.to = query.to;
        return cleaned;
    }

    private dateRangeFromQuery(query: AuditTableQuery): Date[] {
        if (!query.from || !query.to) {
            return [];
        }
        const from = new Date(query.from);
        const to = new Date(query.to);
        if (Number.isNaN(from.getTime()) || Number.isNaN(to.getTime())) {
            return [];
        }
        return [from, to];
    }
}
