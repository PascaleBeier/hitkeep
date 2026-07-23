import { ChangeDetectionStrategy, Component } from '@angular/core';

@Component({
    selector: 'app-crud-table-toolbar',
    template: '<ng-content />',
    styleUrl: './crud-table-toolbar.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class CrudTableToolbar {}
