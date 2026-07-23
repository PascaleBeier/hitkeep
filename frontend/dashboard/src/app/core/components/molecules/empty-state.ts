import { Component, input, output, ChangeDetectionStrategy } from '@angular/core';

import { ButtonModule } from '@openng/optimus-ui/button';

@Component({
    selector: 'app-empty-state',
    standalone: true,
    imports: [ButtonModule],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <div class="flex flex-col items-center justify-center py-12 text-center h-full w-full">
            <!-- Icon Circle -->
            <div class="w-16 h-16 rounded-full bg-[var(--p-surface-100)] dark:bg-[var(--p-surface-800)] flex items-center justify-center mb-4 text-[var(--p-primary-color)]">
                <i [class]="'pi ' + icon() + ' text-2xl'" aria-hidden="true"></i>
            </div>

            <!-- Content -->
            <h3 class="text-lg font-semibold text-[var(--p-text-color)] mb-2">
                {{ title() }}
            </h3>

            @if (description()) {
                <p class="text-sm text-[var(--p-text-muted-color)] max-w-xs mx-auto mb-6 leading-relaxed">
                    {{ description() }}
                </p>
            }

            <!-- CTA Button -->
            @if (actionLabel()) {
                <p-button [label]="actionLabel()" [icon]="actionIcon()" (onClick)="actionClicked.emit()" [outlined]="true" size="small" />
            }
        </div>
    `
})
export class EmptyState {
    // Configuration Inputs
    icon = input.required<string>(); // e.g., 'pi-flag'
    title = input.required<string>(); // e.g., 'No goals yet'
    description = input<string>(''); // e.g., 'Track specific events...'

    // CTA Configuration
    actionLabel = input<string>(''); // e.g., 'Create Goal'
    actionIcon = input<string>('pi pi-plus');

    // Outputs
    actionClicked = output<void>();
}
