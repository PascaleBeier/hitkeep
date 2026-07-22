import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { DialogShell } from '@components/dialog-shell/dialog-shell';

@Component({
    selector: 'app-crud-dialog',
    imports: [DialogShell],
    templateUrl: './crud-dialog.html',
    styleUrl: './crud-dialog.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class CrudDialog {
    readonly title = input('');
    readonly visible = input(false);
    readonly submitLabel = input('');
    readonly cancelLabel = input('');
    readonly submitIcon = input('pi pi-check');
    readonly submitDisabled = input(false);
    readonly saving = input(false);
    readonly width = input('42rem');

    readonly visibleChange = output<boolean>();
    readonly submitted = output<void>();
    readonly cancelled = output<void>();

    protected onVisibleChange(visible: boolean): void {
        if (!visible && this.saving()) {
            return;
        }
        this.visibleChange.emit(visible);
    }

    protected onSecondaryAction(): void {
        this.cancelled.emit();
    }
}
