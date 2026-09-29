import { Component, signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { vi } from 'vitest';

import { AppTable, AppTableCell, AppTableColumn, csvCell } from './table';

interface Hook {
    id: string;
    name: string;
    status: string;
    events: string[];
    attempts: number;
    updated_at: string;
}

const ROWS: Hook[] = [
    { id: '1', name: 'Alpha', status: 'active', events: ['hit.created'], attempts: 3, updated_at: '2026-09-01T10:00:00Z' },
    { id: '2', name: 'Beta', status: 'paused', events: ['goal.converted', 'hit.created'], attempts: 1, updated_at: '2026-09-02T10:00:00Z' },
    { id: '3', name: '=Gamma', status: 'active', events: [], attempts: 7, updated_at: '2026-09-03T10:00:00Z' }
];

const COLUMNS: AppTableColumn<Hook>[] = [
    { field: 'name', headerKey: 'cols.name', frozen: true },
    {
        field: 'status',
        headerKey: 'cols.status',
        type: 'enum',
        groupable: true,
        options: [
            { value: 'active', labelKey: 'status.active' },
            { value: 'paused', labelKey: 'status.paused' }
        ]
    },
    { field: 'attempts', headerKey: 'cols.attempts', type: 'number', align: 'end', total: true },
    { field: 'events', headerKey: 'cols.events', type: 'enum', sortable: false, options: [{ value: 'hit.created', labelKey: 'status.hit' }] }
];

@Component({
    imports: [AppTable, AppTableCell],
    template: `
        <app-table [value]="rows()" [columns]="columns" stateKey="spec" [rowActions]="actions" [defaultSort]="{ field: 'name', order: 1 }" [loading]="loading()">
            <ng-template appTableCell="events" let-row>
                <span class="events-cell">{{ row.events.length }}</span>
            </ng-template>
        </app-table>
    `
})
class Host {
    readonly rows = signal<Hook[]>(ROWS.map((row) => ({ ...row })));
    readonly loading = signal(false);
    readonly columns = COLUMNS;
    readonly actions = () => [{ label: 'Edit' }];
}

describe('AppTable', () => {
    let fixture: ComponentFixture<Host>;

    beforeEach(async () => {
        localStorage.clear();
        await TestBed.configureTestingModule({
            imports: [
                Host,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            cols: { name: 'Name', status: 'Status', attempts: 'Attempts', events: 'Events' },
                            status: { active: 'Active', paused: 'Paused', hit: 'Hit' },
                            common: { searchPlaceholder: 'Search', columns: { actions: 'Actions' }, actions: { exportCsv: 'Export CSV', more: 'More' } },
                            table: { total: 'Total', searchChip: 'Search: {{value}}', groupCount: '{{count}} rows', noResults: 'No matches', empty: 'Nothing yet', clearFilters: 'Clear filters' }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ],
            providers: [provideTranslocoLocale({ langToLocaleMapping: { en: 'en-US' } })]
        }).compileComponents();

        fixture = TestBed.createComponent(Host);
        fixture.detectChanges();
        await fixture.whenStable();
    });

    afterEach(() => localStorage.clear());

    it('renders sorted rows, default cells, custom cells, and a frozen actions column', () => {
        expect(names()).toEqual(['=Gamma', 'Alpha', 'Beta']);
        expect(headers()).toEqual(['Name', 'Status', 'Attempts', 'Events', 'Actions']);
        expect(rowCells(1)).toContain('Active');
        expect(fixture.nativeElement.querySelector('.events-cell').textContent).toBe('0');
        expect(fixture.nativeElement.querySelectorAll('td.app-table__actions app-table-row-actions').length).toBe(3);
        expect(fixture.nativeElement.querySelector('td.p-datatable-frozen-column')?.textContent).toContain('=Gamma');
    });

    it('searches as a filter and shows a removable chip', async () => {
        await search('alp');

        expect(names()).toEqual(['Alpha']);
        expect(chips()).toEqual(['Search: alp']);

        (fixture.nativeElement.querySelector('app-filter-chip-row button') as HTMLButtonElement).click();
        await settle();

        expect(names().length).toBe(3);
        expect(chips()).toEqual([]);
    });

    it('filters enum columns, including array values, and shows the no-results state', async () => {
        const table = component()['table']();
        table.filters['events'] = [{ value: ['hit.created'], matchMode: 'hkIn', operator: 'and' }];
        table._filter();
        await settle();

        expect(names()).toEqual(['Alpha', 'Beta']);
        expect(chips()).toEqual(['Events: Hit']);

        await search('zzz');
        expect(fixture.nativeElement.querySelector('.app-table__empty')?.textContent).toContain('No matches');
    });

    it('groups rows with labelled, counted, collapsible group headers', async () => {
        component()['setGroupBy']('status');
        await settle();

        const labels = Array.from(fixture.nativeElement.querySelectorAll('.app-table__group-label') as NodeListOf<HTMLElement>).map((label) => label.textContent?.trim());
        const counts = Array.from(fixture.nativeElement.querySelectorAll('.app-table__group-count') as NodeListOf<HTMLElement>).map((count) => count.textContent?.trim());
        expect(labels).toEqual(['Active', 'Paused']);
        expect(counts).toEqual(['2 rows', '1 rows']);
        expect(names()).toEqual(['=Gamma', 'Alpha', 'Beta']);

        (fixture.nativeElement.querySelector('.app-table__group-toggle') as HTMLButtonElement).click();
        await settle();
        expect(names()).toEqual(['Beta']);
        expect(JSON.parse(localStorage.getItem('hitkeep.table.v1.spec.view')!).groupBy).toBe('status');
    });

    it('keeps rendered rows when callers rebuild row objects with the same keys', async () => {
        const before = fixture.nativeElement.querySelector('tr.app-table__row');
        fixture.componentInstance.rows.set(ROWS.map((row) => ({ ...row })));
        await settle();

        expect(fixture.nativeElement.querySelector('tr.app-table__row')).toBe(before);
    });

    it('sums total columns over the filtered rows', async () => {
        const footer = () => (fixture.nativeElement.querySelector('tr.app-table__total') as HTMLElement).textContent?.replace(/\s+/g, ' ').trim();
        expect(footer()).toBe('Total 11');

        await search('alp');
        expect(footer()).toBe('Total 3');
    });

    it('hides columns and persists the choice', async () => {
        component()['setShownColumns'](['status', 'attempts']);
        await settle();

        expect(headers()).toEqual(['Name', 'Status', 'Attempts', 'Actions']);
        expect(JSON.parse(localStorage.getItem('hitkeep.table.v1.spec.view')!).hidden).toEqual(['events']);
    });

    it('persists sort and filter state and resets the view', async () => {
        await search('beta');
        expect(JSON.parse(localStorage.getItem('hitkeep.table.v1.spec')!).filters.global.value).toBe('beta');

        component()['resetView']();
        await settle();

        expect(names().length).toBe(3);
        expect(localStorage.getItem('hitkeep.table.v1.spec')).toBeNull();
    });

    it('exports the current view as formula-safe CSV', async () => {
        let blob: Blob | undefined;
        vi.spyOn(URL, 'createObjectURL').mockImplementation((value) => {
            blob = value as Blob;
            return 'blob:table';
        });
        vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined);
        vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);

        await search('a');
        component()['exportCsv']();
        const text = await blob!.text();

        expect(text.replace('\uFEFF', '').split('\r\n')).toEqual(['"Name","Status","Attempts","Events"', `"'=Gamma","Active","7",""`, '"Alpha","Active","3","Hit"', '"Beta","Paused","1","goal.converted; Hit"']);
    });

    it('shows skeleton rows while the first load is in flight', async () => {
        fixture.componentInstance.rows.set([]);
        fixture.componentInstance.loading.set(true);
        await settle();

        expect(fixture.nativeElement.querySelectorAll('.app-table__skeleton').length).toBe(5);
    });

    it('quotes CSV cells', () => {
        expect(csvCell('a "b"')).toBe('"a ""b"""');
        expect(csvCell('+1')).toBe(`"'+1"`);
    });

    function component(): AppTable<Hook> {
        return fixture.debugElement.children[0].componentInstance as AppTable<Hook>;
    }

    async function search(value: string) {
        component()['applySearch'](value);
        await new Promise((resolve) => setTimeout(resolve, 300));
        await settle();
    }

    async function settle() {
        fixture.detectChanges();
        await fixture.whenStable();
        fixture.detectChanges();
    }

    function names(): string[] {
        return Array.from(fixture.nativeElement.querySelectorAll('tr.app-table__row') as NodeListOf<HTMLElement>).map((row) => row.querySelector('td')?.textContent?.trim() ?? '');
    }

    function rowCells(index: number): string[] {
        const row = fixture.nativeElement.querySelectorAll('tr.app-table__row')[index] as HTMLElement;
        return Array.from(row.querySelectorAll('td')).map((cell) => cell.textContent?.trim() ?? '');
    }

    function headers(): string[] {
        return Array.from(fixture.nativeElement.querySelectorAll('thead th') as NodeListOf<HTMLElement>).map((cell) => cell.querySelector('.app-table__heading > span')?.textContent?.trim() ?? cell.textContent?.trim() ?? '');
    }

    function chips(): string[] {
        return Array.from(fixture.nativeElement.querySelectorAll('app-filter-chip-row .font-medium') as NodeListOf<HTMLElement>).map((chip) => chip.textContent?.trim() ?? '');
    }
});
