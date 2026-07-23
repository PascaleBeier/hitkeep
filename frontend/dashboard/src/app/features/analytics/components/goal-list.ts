import { Component, input, output, signal, ChangeDetectionStrategy } from '@angular/core';

import { TranslocoPipe } from '@jsverse/transloco';
import { TranslocoDecimalPipe } from '@jsverse/transloco-locale';
import { CardModule } from '@openng/optimus-ui/card';
import { ButtonModule } from '@openng/optimus-ui/button';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { TooltipModule } from '@openng/optimus-ui/tooltip';
import { GoalStats } from '@models/analytics.types';

// Components
import { EmptyState } from '@components/molecules/empty-state';
import { GoalManager } from '@features/goals/components/goal-manager';

@Component({
    selector: 'app-goal-list',
    standalone: true,
    imports: [CardModule, SkeletonModule, TranslocoPipe, TranslocoDecimalPipe, ButtonModule, TooltipModule, EmptyState, GoalManager],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <p-card class="shadow-sm h-full border border-surface-200 dark:border-surface-700 surface-card">
            <!-- Header with Quick Add Button -->
            <div class="flex items-center justify-between mb-4">
                <div class="flex items-center gap-2">
                    <i class="pi pi-flag text-[var(--p-primary-color)]" aria-hidden="true"></i>
                    <h3 class="font-semibold text-lg">{{ 'goals.list.title' | transloco }}</h3>
                </div>
                <!-- Only show header add button if we have data (otherwise EmptyState handles it) -->
                @if (!readOnly() && !isLoading() && data() && data().length > 0) {
                    <p-button icon="pi pi-plus" (onClick)="showManager.set(true)" [rounded]="true" [text]="true" [pTooltip]="'goals.list.manageTooltip' | transloco" styleClass="w-8 h-8" />
                }
            </div>

            @if (isLoading()) {
                <div class="flex flex-col gap-3">
                    @for (i of [1, 2, 3]; track i) {
                        <p-skeleton height="3rem" styleClass="w-full" />
                    }
                </div>
            } @else if (!data() || data().length === 0) {
                <!-- Reusable Empty State -->
                <app-empty-state
                    icon="pi-flag"
                    [title]="'goals.list.emptyTitle' | transloco"
                    [description]="'goals.list.emptyDescription' | transloco"
                    [actionLabel]="readOnly() ? '' : ('goals.list.createAction' | transloco)"
                    (actionClicked)="showManager.set(true)"
                />
            } @else {
                <!-- List Data -->
                <ul class="flex flex-col gap-3 m-0 p-0 list-none">
                    @for (item of data(); track item.goal_id) {
                        <li class="flex items-center justify-between text-sm p-3 border border-surface-100 dark:border-surface-800 rounded hover:bg-surface-50 dark:hover:bg-surface-800 transition-colors">
                            <div class="flex flex-col gap-1">
                                <span class="font-medium truncate" [title]="item.name">{{ item.name }}</span>
                                <span class="text-xs text-muted-color">{{ item.conversions | translocoDecimal }} {{ 'goals.list.conversionsSuffix' | transloco }}</span>
                            </div>
                            <div class="flex flex-col items-end">
                                <span class="font-bold text-[var(--p-primary-color)]">{{ item.conversion_rate | translocoDecimal: { minimumFractionDigits: 1, maximumFractionDigits: 2 } }}%</span>
                                <span class="text-xs text-muted-color">{{ 'goals.list.conversionRateShort' | transloco }}</span>
                            </div>
                        </li>
                    }
                </ul>
            }

            <!-- Embedded Manager Dialog -->
            @if (!readOnly()) {
                <app-goal-manager [(visible)]="showManager" [siteId]="siteId()" (goalsChanged)="refresh.emit()" />
            }
        </p-card>
    `
})
export class GoalList {
    data = input.required<GoalStats[]>();
    siteId = input.required<string | null>(); // Required for the manager
    isLoading = input<boolean>(false);
    readOnly = input<boolean>(false);

    // Emit when data changes so parent can reload stats
    refresh = output<void>();

    // Internal state for the dialog
    protected showManager = signal(false);
}
