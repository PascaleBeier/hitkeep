import { Component, computed, input, output, ChangeDetectionStrategy } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';

import { CardModule } from '@openng/optimus-ui/card';
import { ButtonModule } from '@openng/optimus-ui/button';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { TooltipModule } from '@openng/optimus-ui/tooltip';
import { Funnel } from '@models/analytics.types';
import { EmptyState } from '@components/molecules/empty-state';
import { injectSkeletonGate } from '@services/report-subject.service';

@Component({
    selector: 'app-funnel-list',
    standalone: true,
    imports: [CardModule, ButtonModule, SkeletonModule, TooltipModule, EmptyState, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <p-card class="shadow-sm h-full border border-surface-200 dark:border-surface-700 surface-card">
            <div class="flex items-center justify-between mb-4">
                <div class="flex items-center gap-2">
                    <i class="pi pi-filter text-[var(--p-primary-color)]" aria-hidden="true"></i>
                    <h3 class="font-semibold text-lg">{{ 'funnels.list.title' | transloco }}</h3>
                </div>
                @if (!readOnly() && !showSkeleton() && hasFunnels()) {
                    <p-button icon="pi pi-plus" (onClick)="manageClicked.emit()" [rounded]="true" [text]="true" [pTooltip]="'funnels.list.manageTooltip' | transloco" styleClass="w-8 h-8" />
                }
            </div>

            @if (showSkeleton()) {
                <div class="flex flex-col gap-3">
                    @for (i of [1, 2]; track i) {
                        <p-skeleton height="3rem" styleClass="w-full" />
                    }
                </div>
            } @else if (!hasFunnels()) {
                <app-empty-state
                    icon="pi-filter"
                    [title]="'funnels.list.emptyTitle' | transloco"
                    [description]="'funnels.list.emptyDescription' | transloco"
                    [actionLabel]="readOnly() ? '' : ('funnels.list.createAction' | transloco)"
                    (actionClicked)="manageClicked.emit()"
                />
            } @else {
                <ul class="flex flex-col gap-3 m-0 p-0 list-none">
                    @for (funnel of funnels(); track funnel.id) {
                        <li>
                            <button
                                type="button"
                                class="w-full cursor-pointer flex items-center justify-between text-sm p-3 border border-surface-100 dark:border-surface-800 rounded hover:bg-surface-50 dark:hover:bg-surface-800 transition-colors text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
                                (click)="funnelClicked.emit(funnel)"
                            >
                                <div class="flex flex-col gap-1 min-w-0">
                                    <span class="font-medium truncate">{{ funnel.name }}</span>
                                    <div class="flex items-center gap-1 text-xs text-muted-color">
                                        <span>{{ 'funnels.list.stepsCount' | transloco: { count: funnel.steps.length } }}</span>
                                    </div>
                                </div>
                                <i class="pi pi-chevron-right text-muted-color" aria-hidden="true"></i>
                            </button>
                        </li>
                    }
                </ul>
            }
        </p-card>
    `
})
export class FunnelList {
    funnels = input.required<Funnel[]>();
    isLoading = input<boolean>(false);
    readOnly = input<boolean>(false);

    protected readonly hasFunnels = computed(() => this.funnels().length > 0);
    protected readonly showSkeleton = injectSkeletonGate(this.isLoading, this.hasFunnels);
    manageClicked = output<void>();
    funnelClicked = output<Funnel>();
}
