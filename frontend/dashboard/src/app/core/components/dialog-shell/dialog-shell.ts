import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { DialogModule } from '@openng/optimus-ui/dialog';
import type { ButtonSeverity } from '@openng/optimus-ui/types/button';

@Component({
    selector: 'app-dialog-shell',
    imports: [ButtonModule, DialogModule],
    templateUrl: './dialog-shell.html',
    styleUrl: './dialog-shell.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class DialogShell {
    readonly title = input('');
    readonly visible = input(false);
    readonly width = input('42rem');
    readonly role = input('dialog');
    readonly busy = input(false);
    readonly closable = input(true);
    readonly closeOnEscape = input(true);

    readonly secondaryLabel = input('');
    readonly primaryLabel = input('');
    readonly primaryIcon = input('');
    readonly primarySeverity = input<ButtonSeverity | undefined>(undefined);
    readonly primaryDisabled = input(false);
    readonly primaryLoading = input(false);
    readonly showSecondary = input(true);
    readonly showPrimary = input(true);

    readonly visibleChange = output<boolean>();
    readonly secondaryAction = output<void>();
    readonly primaryAction = output<void>();

    protected readonly breakpoints = {
        '768px': '96vw'
    };

    protected readonly dialogStyle = () => ({
        width: this.width(),
        maxWidth: '96vw'
    });

    protected onVisibleChange(visible: boolean): void {
        if (!visible && this.busy()) {
            return;
        }
        this.visibleChange.emit(visible);
    }

    protected onSecondaryAction(): void {
        if (this.busy()) {
            return;
        }
        this.secondaryAction.emit();
        this.visibleChange.emit(false);
    }

    protected hasFooter(): boolean {
        return (this.showSecondary() && this.secondaryLabel().length > 0) || (this.showPrimary() && this.primaryLabel().length > 0);
    }
}
