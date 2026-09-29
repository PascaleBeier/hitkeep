export type AppTableColumnType = 'text' | 'enum' | 'number' | 'date' | 'boolean';

export interface AppTableOption {
    value: unknown;
    /** Transloco key for the label. */
    labelKey?: string;
    /** Literal label for runtime values (event names, hostnames). */
    label?: string;
}

export interface AppTableColumn<T = unknown> {
    /** Row value used for sort, filter, group, and export. Dotted paths resolve nested values. */
    field: string;
    /** Transloco key for the header. */
    headerKey?: string;
    /** Literal header for runtime or untranslated labels (metric acronyms, dimension names). */
    header?: string;
    /** Picks the filter element, default cell, group label, and export format. Defaults to `text`. */
    type?: AppTableColumnType;
    /** Enum labels for filter options, cells, group headers, and export. Omit to offer the distinct row values. */
    options?: readonly AppTableOption[];
    /** Translates enum values without listed options as `labelKeyPrefix + value`. */
    labelKeyPrefix?: string;
    /** Enum filters pick one value instead of many (for APIs that accept a single value). */
    singleSelect?: boolean;
    sortable?: boolean;
    filterable?: boolean;
    /** Included in the global search. Defaults to true for text columns. */
    searchable?: boolean;
    /** Offered in "Group by". */
    groupable?: boolean;
    /** Identity column, frozen left. */
    frozen?: boolean;
    /** Offered in the column toggle. Defaults to true, never for frozen columns. */
    hideable?: boolean;
    /** Hidden until the viewer shows it. */
    hidden?: boolean;
    /** Initial width; user resizing persists with the table state. */
    width?: string;
    align?: 'start' | 'end';
    /** Formatted CSV value. */
    exportValue?: (row: T) => string;
}

export interface AppTableSort {
    field: string;
    order: 1 | -1;
}
