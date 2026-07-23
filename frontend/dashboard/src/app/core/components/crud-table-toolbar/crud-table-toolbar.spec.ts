import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { CrudTableToolbar } from './crud-table-toolbar';

@Component({
    imports: [CrudTableToolbar],
    template: `
        <app-crud-table-toolbar>
            <input class="hk-crud-search" aria-label="Search" />
            <button type="button">Refresh</button>
        </app-crud-table-toolbar>
    `
})
class CrudTableToolbarTestHost {}

describe('CrudTableToolbar', () => {
    it('projects the standard CRUD search and action controls', () => {
        const fixture = TestBed.createComponent(CrudTableToolbarTestHost);
        fixture.detectChanges();

        const toolbar = fixture.nativeElement.querySelector('app-crud-table-toolbar') as HTMLElement;
        expect(toolbar.querySelector('.hk-crud-search')).not.toBeNull();
        expect(toolbar.querySelector('button')?.textContent).toContain('Refresh');
    });
});
