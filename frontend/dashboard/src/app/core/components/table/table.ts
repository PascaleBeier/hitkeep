import { NgTemplateOutlet } from '@angular/common';
import { ChangeDetectionStrategy, Component, Directive, OnInit, TemplateRef, computed, contentChildren, inject, input, output, signal, viewChild } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { FilterMetadata, FilterService, SortEvent } from '@openng/optimus-ui/api';
import { ButtonModule } from '@openng/optimus-ui/button';
import { DatePickerModule } from '@openng/optimus-ui/datepicker';
import { IconFieldModule } from '@openng/optimus-ui/iconfield';
import { InputIconModule } from '@openng/optimus-ui/inputicon';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MultiSelectModule } from '@openng/optimus-ui/multiselect';
import { SelectModule } from '@openng/optimus-ui/select';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { Table, TableLazyLoadEvent, TableModule } from '@openng/optimus-ui/table';
import { TooltipModule } from '@openng/optimus-ui/tooltip';

import { FilterChipItem, FilterChipRow } from '@components/filter-chip-row/filter-chip-row';
import { PageState } from '@components/page-state/page-state';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';
import { localeForLanguage } from '@core/i18n/duration-format';

import { AppTableColumn, AppTableOption, AppTableSort } from './table.types';

export type { AppTableColumn, AppTableColumnType, AppTableOption, AppTableSort } from './table.types';

/** Custom cell rendering for one column: `<ng-template appTableCell="name" let-row>`. */
@Directive({ selector: 'ng-template[appTableCell]' })
export class AppTableCell {
    readonly field = input.required<string>({ alias: 'appTableCell' });
    readonly template = inject<TemplateRef<{ $implicit: unknown }>>(TemplateRef);
}

export type AppTableSlotName = 'toolbar' | 'actions' | 'expanded' | 'mobileRow';

/** Named table slot: toolbar actions, a custom actions cell, an expanded row, or a mobile card. */
@Directive({ selector: 'ng-template[appTableSlot]' })
export class AppTableSlot {
    readonly name = input.required<AppTableSlotName>({ alias: 'appTableSlot' });
    readonly template = inject<TemplateRef<{ $implicit: unknown }>>(TemplateRef);
}

const STORAGE_PREFIX = 'hitkeep.table.v1.';
const MATCH_IN = 'hkIn';
const MATCH_DATE_RANGE = 'hkDateRange';
const SKELETON_ROWS = [0, 1, 2, 3, 4].map((index) => ({ __skeleton: index }));
const NUMBER_SYMBOLS: Record<string, string> = { equals: '=', notEquals: '≠', lt: '<', lte: '≤', gt: '>', gte: '≥' };

interface ViewState {
    hidden?: string[];
    groupBy?: string | null;
}

type Row = Record<string, unknown>;

@Component({
    selector: 'app-table',
    imports: [
        NgTemplateOutlet,
        FormsModule,
        TranslocoPipe,
        TableModule,
        ButtonModule,
        DatePickerModule,
        IconFieldModule,
        InputIconModule,
        InputTextModule,
        MultiSelectModule,
        SelectModule,
        SkeletonModule,
        TooltipModule,
        FilterChipRow,
        PageState,
        RelativeDateTime,
        TableRowActions
    ],
    templateUrl: './table.html',
    styleUrl: './table.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AppTable<T = Row> implements OnInit {
    private readonly transloco = inject(TranslocoService);

    readonly value = input.required<readonly T[]>();
    readonly columns = input.required<readonly AppTableColumn<T>[]>();
    readonly stateKey = input.required<string>();
    readonly dataKey = input('id');
    readonly loading = input(false);
    readonly ariaLabelKey = input<string>();
    readonly defaultSort = input<AppTableSort>();
    readonly defaultGroupBy = input<string | null>(null);
    readonly rowActions = input<(row: T) => readonly TableRowActionItem[]>();
    readonly actionsDisabled = input(false);
    readonly rowActionsLoading = input<(row: T) => boolean>(() => false);
    /** Rows are clickable and keyboard-selectable; the row matching `selectedKey` is highlighted. */
    readonly selectable = input(false);
    readonly selectedKey = input<unknown>(null);
    /** Highlights every matching row, for pages where several rows can be active at once. */
    readonly rowSelected = input<(row: T) => boolean>();
    readonly searchable = input(true);
    /** Row fields searched in addition to searchable columns. */
    readonly searchFields = input<readonly string[]>([]);
    readonly exportable = input(true);
    readonly exportFilename = input<string>();
    readonly emptyTitleKey = input<string>();
    readonly emptyTextKey = input('table.empty');
    readonly emptyIcon = input('pi pi-inbox');
    readonly density = input<'compact' | 'comfortable'>('compact');
    readonly mobile = input<'scroll' | 'cards'>('scroll');
    readonly rows = input(10);
    readonly rowsPerPageOptions = input<number[]>([10, 25, 50]);
    readonly rowExpandable = input<(row: T) => boolean>(() => true);
    readonly lazy = input(false);
    readonly totalRecords = input(0);
    readonly first = input(0);

    readonly lazyLoad = output<TableLazyLoadEvent>();
    readonly rowExpansionChange = output<{ row: T; expanded: boolean }>();
    readonly rowSelect = output<T>();

    protected readonly tableRef = viewChild<Table>('table');
    private readonly cellTemplates = contentChildren(AppTableCell);
    private readonly slots = contentChildren(AppTableSlot);

    protected readonly toolbarTemplate = computed(() => this.slot('toolbar'));
    protected readonly actionsTemplate = computed(() => this.slot('actions'));
    protected readonly expandedTemplate = computed(() => this.slot('expanded'));
    protected readonly mobileRowTemplate = computed(() => this.slot('mobileRow'));
    protected readonly cells = computed(() => new Map(this.cellTemplates().map((cell) => [cell.field(), cell.template])));

    protected readonly search = signal('');
    protected readonly groupBy = signal<string | null>(null);
    private readonly hidden = signal<ReadonlySet<string>>(new Set());
    private readonly collapsedGroups = signal<ReadonlySet<string>>(new Set());
    private readonly expandedKeys = signal<ReadonlySet<unknown>>(new Set());
    private readonly filterTick = signal(0);
    private readonly translation = toSignal(this.transloco.selectTranslation());

    /** Placeholder rows render as skeletons until the first load completes. */
    protected readonly skeleton = computed(() => this.loading() && this.value().length === 0);
    protected readonly tableValue = computed(() => {
        if (this.skeleton()) return SKELETON_ROWS as unknown as T[];
        const groupBy = this.groupColumn();
        if (!this.lazy() || !groupBy) return [...this.value()];
        // Server-paged tables group within the current page and keep the server's order of groups.
        const groups = new Map<string, T[]>();
        for (const row of this.value()) {
            const key = this.groupKey(row, groupBy);
            groups.set(key, [...(groups.get(key) ?? []), row]);
        }
        return [...groups.values()].flat();
    });
    protected readonly visibleColumns = computed(() => this.columns().filter((column) => !this.hidden().has(column.field)));
    protected readonly hasActions = computed(() => Boolean(this.rowActions() || this.actionsTemplate()));
    protected readonly colspan = computed(() => this.visibleColumns().length + (this.hasActions() ? 1 : 0) + (this.expandedTemplate() ? 1 : 0));
    protected readonly skeletonCells = computed(() => Array.from({ length: this.colspan() }, (_, index) => index));
    protected readonly globalFilterFields = computed(() => [
        ...this.columns()
            .filter((column) => column.searchable ?? (column.type ?? 'text') === 'text')
            .map((column) => column.field),
        ...this.searchFields()
    ]);
    protected readonly groupColumn = computed(() => this.columns().find((column) => column.field === this.groupBy()) ?? null);
    protected readonly groupOptions = computed(() => {
        this.translation();
        return this.columns()
            .filter((column) => column.groupable)
            .map((column) => ({ label: this.headerLabel(column), value: column.field }));
    });
    protected readonly toggleOptions = computed(() => {
        this.translation();
        return this.columns()
            .filter((column) => this.isHideable(column))
            .map((column) => ({ label: this.headerLabel(column), value: column.field }));
    });
    protected readonly shownToggleFields = computed(() =>
        this.toggleOptions()
            .map((option) => option.value)
            .filter((field) => !this.hidden().has(field))
    );
    protected readonly initialSort = computed(() => [this.defaultSort() ?? { field: this.columns().find((column) => column.sortable !== false)?.field ?? this.dataKey(), order: 1 }]);
    protected readonly pageReport = computed(() => {
        this.translation();
        return this.transloco.translate('table.pageReport');
    });
    protected readonly chips = computed<FilterChipItem[]>(() => {
        this.filterTick();
        this.translation();
        const filters = this.tableRef()?.filters;
        if (!filters) return [];
        const chips: FilterChipItem[] = [];
        const global = filters['global'] as FilterMetadata | undefined;
        if (global?.value) {
            chips.push({ key: 'global', label: this.transloco.translate('table.searchChip', { value: global.value }), remove: () => this.clearSearch() });
        }
        for (const column of this.columns()) {
            const values = this.constraints(column.field).filter((constraint) => !this.isBlank(constraint.value));
            if (values.length === 0) {
                continue;
            }
            chips.push({
                key: column.field,
                label: `${this.headerLabel(column)}: ${values.map((constraint) => this.describe(column, constraint)).join(', ')}`,
                remove: () => this.clearColumnFilter(column.field)
            });
        }
        return chips;
    });

    constructor() {
        const filters = inject(FilterService);
        filters.register(MATCH_IN, (value: unknown, filter: unknown[] | null) => {
            if (!filter?.length) return true;
            return Array.isArray(value) ? value.some((item) => filter.includes(item)) : filter.includes(value);
        });
        filters.register(MATCH_DATE_RANGE, (value: unknown, filter: (Date | null)[] | null) => {
            const [from, to] = filter ?? [];
            if (!from) return true;
            if (value == null || value === '') return false;
            const time = new Date(value as string).getTime();
            const end = new Date(to ?? from);
            end.setHours(23, 59, 59, 999);
            return time >= new Date(from).setHours(0, 0, 0, 0) && time <= end.getTime();
        });
    }

    ngOnInit(): void {
        const view = this.readView();
        this.hidden.set(
            new Set(
                view?.hidden ??
                    this.columns()
                        .filter((column) => column.hidden)
                        .map((column) => column.field)
            )
        );
        this.groupBy.set(view && 'groupBy' in view ? (view.groupBy ?? null) : this.defaultGroupBy());
    }

    protected table(): Table {
        return this.tableRef()!;
    }

    protected get storageKey(): string {
        return STORAGE_PREFIX + this.stateKey();
    }

    protected onFilter(): void {
        this.search.set(String((this.table().filters['global'] as FilterMetadata | undefined)?.value ?? ''));
        this.filterTick.update((tick) => tick + 1);
    }

    protected applySearch(value: string): void {
        this.search.set(value);
        this.table().filterGlobal(value, 'contains');
    }

    protected clearSearch(): void {
        this.search.set('');
        this.table().filters['global'] = { value: null, matchMode: 'contains' };
        this.table()._filter();
    }

    protected clearFilters(): void {
        this.search.set('');
        this.table().clearFilterValues();
        this.table()._filter();
    }

    protected setGroupBy(field: string | null): void {
        this.groupBy.set(field);
        this.collapsedGroups.set(new Set());
        this.writeView();
        const table = this.table();
        if (!this.lazy()) {
            table.multiSortMeta = table.multiSortMeta?.length ? [...table.multiSortMeta] : this.initialSort();
        }
    }

    protected setShownColumns(fields: string[]): void {
        const shown = new Set(fields);
        this.hidden.set(
            new Set(
                this.toggleOptions()
                    .map((option) => option.value)
                    .filter((field) => !shown.has(field))
            )
        );
        this.writeView();
    }

    protected resetView(): void {
        const table = this.table();
        this.removeView();
        this.hidden.set(
            new Set(
                this.columns()
                    .filter((column) => column.hidden)
                    .map((column) => column.field)
            )
        );
        this.groupBy.set(this.defaultGroupBy());
        this.collapsedGroups.set(new Set());
        this.search.set('');
        table.clearFilterValues();
        table.first = 0;
        table.multiSortMeta = this.initialSort();
        table._filter();
        table.clearState();
    }

    /** Sorts by the group column first, then the active sort. */
    protected sortRows(event: SortEvent): void {
        const group = this.groupColumn();
        const meta = event.multiSortMeta ?? [];
        event.data?.sort((a: T, b: T) => {
            if (group) {
                const order = meta.find((entry) => entry.field === group.field)?.order ?? (group.type === 'date' ? -1 : 1);
                const result = group.type === 'date' ? this.compare(this.resolve(a, group.field), this.resolve(b, group.field)) : this.compare(this.groupKey(a, group), this.groupKey(b, group));
                if (result !== 0) return result * order;
            }
            for (const entry of meta) {
                const result = this.compare(this.resolve(a, entry.field), this.resolve(b, entry.field));
                if (result !== 0) return result * entry.order;
            }
            return 0;
        });
    }

    protected exportCsv(): void {
        const table = this.table();
        const rows = ((this.lazy() ? this.tableValue() : (table.filteredValue ?? table.value)) ?? []) as T[];
        const columns = this.visibleColumns();
        const lines = [columns.map((column) => this.headerLabel(column)), ...rows.map((row) => columns.map((column) => this.exportValue(row, column)))];
        const csv = lines.map((line) => line.map(csvCell).join(',')).join('\r\n');
        const url = URL.createObjectURL(new Blob(['﻿' + csv], { type: 'text/csv;charset=utf-8' }));
        const link = document.createElement('a');
        link.href = url;
        link.download = `${this.exportFilename() ?? this.stateKey()}.csv`;
        link.click();
        URL.revokeObjectURL(url);
    }

    protected groupStart(rowIndex: number): string | null {
        const column = this.groupColumn();
        if (!column || this.skeleton()) return null;
        const table = this.table();
        const rows = (table.filteredValue ?? table.value ?? []) as T[];
        const index = this.lazy() ? rowIndex - table.first! : rowIndex;
        const key = this.groupKey(rows[index], column);
        const pageStart = this.lazy() ? index === 0 : index === table.first;
        return pageStart || this.groupKey(rows[index - 1], column) !== key ? key : null;
    }

    protected isGroupCollapsed(row: T): boolean {
        const column = this.groupColumn();
        return Boolean(column) && this.collapsedGroups().has(this.groupKey(row, column!));
    }

    protected isKeyCollapsed(key: string): boolean {
        return this.collapsedGroups().has(key);
    }

    protected toggleGroup(key: string): void {
        const next = new Set(this.collapsedGroups());
        if (!next.delete(key)) next.add(key);
        this.collapsedGroups.set(next);
    }

    protected groupCount(key: string): number {
        const column = this.groupColumn();
        const table = this.table();
        const rows = (table.filteredValue ?? table.value ?? []) as T[];
        return column ? rows.filter((row) => this.groupKey(row, column) === key).length : 0;
    }

    protected groupLabel(key: string): string {
        const column = this.groupColumn();
        if (!column || key === '') return this.transloco.translate('table.noValue');
        if (column.type === 'date') return this.formatDate(new Date(`${key}T00:00:00`), 'full');
        return this.label(column, column.type === 'boolean' ? key === 'true' : key);
    }

    protected isSelected(row: T): boolean {
        const predicate = this.rowSelected();
        if (predicate) return predicate(row);
        return this.selectedKey() != null && this.resolve(row, this.dataKey()) === this.selectedKey();
    }

    protected selectRow(row: T, event?: Event): void {
        if (!this.selectable()) return;
        event?.preventDefault();
        this.rowSelect.emit(row);
    }

    protected isExpanded(row: T): boolean {
        return this.expandedKeys().has(this.resolve(row, this.dataKey()));
    }

    protected toggleExpanded(row: T): void {
        const key = this.resolve(row, this.dataKey());
        const next = new Set(this.expandedKeys());
        const expanded = !next.delete(key);
        if (expanded) next.add(key);
        this.expandedKeys.set(next);
        this.rowExpansionChange.emit({ row, expanded });
    }

    protected resolve(row: T | undefined, field: string): unknown {
        return field.split('.').reduce<unknown>((value, key) => (value == null ? value : (value as Row)[key]), row);
    }

    protected headerLabel(column: AppTableColumn<T>): string {
        this.translation();
        return column.header ?? this.transloco.translate(column.headerKey ?? column.field);
    }

    protected label(column: AppTableColumn<T>, value: unknown): string {
        if (column.type === 'boolean') return this.transloco.translate(value ? 'common.yes' : 'common.no');
        const option = column.options?.find((candidate) => candidate.value === value || String(candidate.value) === String(value));
        if (option) return this.optionLabel(option);
        return column.labelKeyPrefix && value != null && value !== '' ? this.transloco.translate(column.labelKeyPrefix + String(value)) : String(value ?? '');
    }

    protected enumOptions(column: AppTableColumn<T>) {
        this.translation();
        if (column.options) return column.options.map((option) => ({ label: this.optionLabel(option), value: option.value }));
        const values = new Set(this.value().flatMap((row) => this.resolve(row, column.field) ?? []));
        return [...values]
            .filter((value) => value !== '')
            .sort((a, b) => this.compare(a, b))
            .map((value) => ({ label: this.label(column, value), value }));
    }

    private optionLabel(option: AppTableOption): string {
        return option.label ?? this.transloco.translate(option.labelKey ?? String(option.value));
    }

    protected formatNumber(value: unknown): string {
        return typeof value === 'number' ? new Intl.NumberFormat(this.locale()).format(value) : String(value ?? '');
    }

    private formatDate(date: Date, dateStyle: 'full' | 'medium'): string {
        return new Intl.DateTimeFormat(this.locale(), { dateStyle }).format(date);
    }

    private locale(): string {
        return localeForLanguage(this.transloco.getActiveLang());
    }

    private slot(name: AppTableSlotName): TemplateRef<{ $implicit: unknown }> | null {
        return this.slots().find((slot) => slot.name() === name)?.template ?? null;
    }

    private isHideable(column: AppTableColumn<T>): boolean {
        return !column.frozen && column.hideable !== false;
    }

    private groupKey(row: T | undefined, column: AppTableColumn<T>): string {
        const value = this.resolve(row, column.field);
        if (value == null || value === '') return '';
        if (column.type === 'date') {
            const date = new Date(value as string);
            return Number.isNaN(date.getTime()) ? '' : `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
        }
        return Array.isArray(value) ? value.join(', ') : String(value);
    }

    private compare(a: unknown, b: unknown): number {
        if (a == null || a === '') return b == null || b === '' ? 0 : -1;
        if (b == null || b === '') return 1;
        if (typeof a === 'number' && typeof b === 'number') return a - b;
        if (typeof a === 'boolean' && typeof b === 'boolean') return Number(a) - Number(b);
        return String(a).localeCompare(String(b), this.transloco.getActiveLang(), { numeric: true, sensitivity: 'base' });
    }

    private constraints(field: string): FilterMetadata[] {
        const meta = this.table().filters[field];
        return Array.isArray(meta) ? meta : meta ? [meta] : [];
    }

    private clearColumnFilter(field: string): void {
        const table = this.table();
        const [first] = this.constraints(field);
        table.filters[field] = [{ ...first, value: null }];
        table._filter();
    }

    private describe(column: AppTableColumn<T>, constraint: FilterMetadata): string {
        const value = constraint.value;
        switch (column.type) {
            case 'enum':
                return (Array.isArray(value) ? value : [value]).map((item) => this.label(column, item)).join(' / ');
            case 'boolean':
                return this.label(column, value);
            case 'date':
                return (Array.isArray(value) ? value : [value])
                    .filter(Boolean)
                    .map((date: Date) => this.formatDate(new Date(date), 'medium'))
                    .join(' – ');
            case 'number':
                return `${NUMBER_SYMBOLS[constraint.matchMode ?? 'equals'] ?? ''} ${this.formatNumber(value)}`.trim();
            default:
                return constraint.matchMode === 'equals' ? `= ${value}` : `"${value}"`;
        }
    }

    private exportValue(row: T, column: AppTableColumn<T>): string {
        if (column.exportValue) return column.exportValue(row);
        const value = this.resolve(row, column.field);
        if (value == null) return '';
        const format = (item: unknown) => (column.type === 'enum' || column.type === 'boolean' ? this.label(column, item) : String(item ?? ''));
        return Array.isArray(value) ? value.map(format).join('; ') : format(value);
    }

    private isBlank(value: unknown): boolean {
        return value == null || value === '' || (Array.isArray(value) && value.every((item) => item == null));
    }

    private readView(): ViewState | null {
        try {
            const raw = localStorage.getItem(`${this.storageKey}.view`);
            return raw ? (JSON.parse(raw) as ViewState) : null;
        } catch {
            return null;
        }
    }

    private writeView(): void {
        try {
            localStorage.setItem(`${this.storageKey}.view`, JSON.stringify({ hidden: [...this.hidden()], groupBy: this.groupBy() } satisfies ViewState));
        } catch {
            // View state is a convenience; the table works without storage.
        }
    }

    private removeView(): void {
        try {
            localStorage.removeItem(`${this.storageKey}.view`);
        } catch {
            // See writeView.
        }
    }
}

function pad(value: number): string {
    return String(value).padStart(2, '0');
}

/** Quotes a CSV cell and neutralises spreadsheet formulas. */
export function csvCell(value: string): string {
    const safe = /^[=+\-@\t\r]/.test(value) ? `'${value}` : value;
    return `"${safe.replace(/"/g, '""')}"`;
}
