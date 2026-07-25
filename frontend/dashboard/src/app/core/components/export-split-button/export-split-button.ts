import { ChangeDetectionStrategy, Component, computed, inject, input, output } from '@angular/core';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { MenuItem } from '@openng/optimus-ui/api';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { injectActiveLang } from '@core/i18n/active-lang';
import { buildTakeoutExportMenuItems, DEFAULT_HITS_EXPORT_FORMAT, TakeoutExportFormat } from '@core/export/export-formats';

export type ExportStatusState = 'idle' | 'success' | 'error';

/**
 * Takeout export split button: the primary action exports the default format,
 * the dropdown offers every supported takeout format.
 */
@Component({
    selector: 'app-export-split-button',
    imports: [SplitButtonModule, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: { class: 'flex items-center gap-2' },
    template: `
        <p-splitButton
            icon="pi pi-download"
            [label]="isExporting() ? ('common.preparing' | transloco) : ('common.actions.exportCsv' | transloco)"
            [model]="menuItems()"
            [disabled]="disabled()"
            styleClass="p-button-outlined p-button-sm"
            (onClick)="requestDefaultFormat()"
        />
    `
})
export class ExportSplitButton {
    isExporting = input<boolean>(false);
    disabled = input<boolean>(false);
    export = output<TakeoutExportFormat>();

    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = injectActiveLang();

    protected readonly menuItems = computed<MenuItem[]>(() => {
        this.activeLanguage();
        return buildTakeoutExportMenuItems(this.transloco, (format) => this.export.emit(format));
    });

    protected requestDefaultFormat(): void {
        this.export.emit(DEFAULT_HITS_EXPORT_FORMAT);
    }
}

/** Success/error banner for the last takeout export attempt. */
@Component({
    selector: 'app-export-status-banner',
    imports: [TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        @if (state() === 'success') {
            <div class="mb-6 rounded-md border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700">{{ successKey() | transloco }}</div>
        }
        @if (state() === 'error') {
            <div class="mb-6 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{{ errorKey() | transloco }}</div>
        }
    `
})
export class ExportStatusBanner {
    state = input<ExportStatusState>('idle');
    successKey = input.required<string>();
    errorKey = input.required<string>();
}
