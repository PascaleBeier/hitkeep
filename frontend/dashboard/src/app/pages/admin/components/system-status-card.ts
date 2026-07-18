import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { ButtonModule } from 'primeng/button';
import { TooltipModule } from 'primeng/tooltip';

type ButtonSeverity = 'secondary' | 'success' | 'info' | 'warn' | 'danger' | 'help' | 'contrast';

@Component({
    selector: 'app-system-status-card',
    imports: [ButtonModule, TooltipModule],
    templateUrl: './system-status-card.html',
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: {
        class: 'block min-w-0',
        '[class.col-span-full]': 'wide()'
    }
})
export class SystemStatusCard {
    title = input.required<string>();
    titleId = input.required<string>();
    description = input('');
    loading = input(false);
    wide = input(false);
    refreshable = input(true);
    refreshDisabled = input(false);
    refreshLabel = input.required<string>();
    actionLabel = input('');
    actionIcon = input('pi pi-bolt');
    actionSeverity = input<ButtonSeverity>('secondary');
    actionLoading = input(false);
    actionDisabled = input(false);

    refreshClicked = output<void>();
    actionClicked = output<void>();

    protected hasAction = computed(() => this.actionLabel().length > 0);
}
