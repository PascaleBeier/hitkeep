import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';

@Component({
    selector: 'app-application-shell-state',
    imports: [TranslocoPipe],
    templateUrl: './application-shell-state.html',
    styleUrl: './application-shell-state.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ApplicationShellState {
    readonly titleKey = input.required<string>();
    readonly messageKey = input.required<string>();
    readonly titleId = input('application-shell-state-title');
    readonly icon = input('pi pi-info-circle');
    readonly danger = input(false);
    readonly primaryActionLabelKey = input<string>();
    readonly primaryActionIcon = input('pi pi-refresh');
    readonly primaryAction = output<void>();
}
