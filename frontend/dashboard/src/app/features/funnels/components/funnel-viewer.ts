import { ChangeDetectionStrategy, Component, OnChanges, inject, input, model, output, signal } from '@angular/core';

import { TranslocoPipe } from '@jsverse/transloco';
import { TranslocoDecimalPipe } from '@jsverse/transloco-locale';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { AnalyticsService } from '@services/analytics.service';
import { FunnelStats } from '@models/analytics.types';

@Component({
    selector: 'app-funnel-viewer',
    standalone: true,
    imports: [DialogShell, SkeletonModule, TranslocoPipe, TranslocoDecimalPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <app-dialog-shell
            [title]="stats()?.name || ('funnels.viewer.dialogTitle' | transloco)"
            [visible]="visible()"
            (visibleChange)="onVisibleChange($event)"
            width="900px"
            [secondaryLabel]="'common.actions.close' | transloco"
            [primaryLabel]="'funnels.viewer.editAction' | transloco"
            primaryIcon="pi pi-pencil"
            [showPrimary]="canEdit()"
            (primaryAction)="editClicked.emit()"
        >
            @if (loading()) {
                <div class="flex flex-col gap-4 p-4">
                    <p-skeleton height="200px" styleClass="w-full" />
                    <div class="flex justify-between">
                        <p-skeleton width="100px" height="2rem" />
                        <p-skeleton width="100px" height="2rem" />
                    </div>
                </div>
            } @else if (stats()) {
                <div class="flex flex-col gap-6 p-2">
                    <!-- Top Summary Cards -->
                    <div class="grid grid-cols-3 gap-4">
                        <div class="p-4 bg-surface-50 dark:bg-surface-900 rounded border border-surface-200 dark:border-surface-700 text-center">
                            <div class="text-sm text-muted-color mb-1">{{ 'funnels.kpis.entries' | transloco }}</div>
                            <div class="text-2xl font-bold">{{ stats()!.total_entries | translocoDecimal }}</div>
                        </div>
                        <div class="p-4 bg-surface-50 dark:bg-surface-900 rounded border border-surface-200 dark:border-surface-700 text-center">
                            <div class="text-sm text-muted-color mb-1">{{ 'funnels.kpis.completions' | transloco }}</div>
                            <div class="text-2xl font-bold">{{ stats()!.total_completions | translocoDecimal }}</div>
                        </div>
                        <div class="p-4 bg-surface-50 dark:bg-surface-900 rounded border border-surface-200 dark:border-surface-700 text-center">
                            <div class="text-sm text-muted-color mb-1">{{ 'common.kpis.conversionRate' | transloco }}</div>
                            <div class="text-2xl font-bold text-primary">{{ stats()!.overall_conversion_rate | translocoDecimal: { minimumFractionDigits: 1, maximumFractionDigits: 2 } }}%</div>
                        </div>
                    </div>

                    <!-- Visual Steps -->
                    <div class="flex flex-col gap-0">
                        @for (step of stats()!.steps; track step.step_index; let first = $first) {
                            <!-- Connector Line -->
                            @if (!first) {
                                <div class="h-8 ml-8 border-l-2 border-dashed border-surface-300 dark:border-surface-600 relative">
                                    <!-- Dropoff Pill -->
                                    <div class="absolute top-1/2 left-4 -translate-y-1/2 text-xs font-medium text-red-500 bg-red-50 dark:bg-red-900/20 px-2 py-0.5 rounded-full border border-red-200 dark:border-red-900/50">
                                        -{{ step.dropoff | translocoDecimal }} {{ 'funnels.viewer.droppedSuffix' | transloco }}
                                    </div>
                                </div>
                            }

                            <div class="flex items-center gap-4 p-4 border border-surface-200 dark:border-surface-700 rounded-lg bg-surface-0 dark:bg-surface-800 shadow-sm relative z-10">
                                <!-- Step Index Circle -->
                                <div class="flex items-center justify-center w-8 h-8 rounded-full bg-primary text-primary-contrast font-bold text-sm shrink-0">
                                    {{ step.step_index + 1 }}
                                </div>

                                <!-- Info -->
                                <div class="flex-1 min-w-0">
                                    <div class="font-semibold truncate" [title]="step.name">{{ step.name }}</div>
                                    <div class="text-sm text-muted-color">{{ step.visitors | translocoDecimal }} {{ 'common.visitors' | transloco }}</div>
                                </div>

                                <!-- Step Conversion -->
                                @if (!first) {
                                    <div class="flex flex-col items-end shrink-0">
                                        <div [class]="conversionClass(step.conversion_rate)">{{ step.conversion_rate | translocoDecimal: { minimumFractionDigits: 1, maximumFractionDigits: 1 } }}%</div>
                                        <div class="text-xs text-muted-color">{{ 'funnels.viewer.retention' | transloco }}</div>
                                    </div>
                                } @else {
                                    <div class="text-xs font-medium text-muted-color uppercase tracking-wider px-2">{{ 'funnels.viewer.entry' | transloco }}</div>
                                }
                            </div>
                        }
                    </div>
                </div>
            }
        </app-dialog-shell>
    `
})
export class FunnelViewer implements OnChanges {
    visible = model(false);
    readonly siteId = input<string | null>(null);
    readonly funnelId = input<string | null>(null);
    readonly dateRange = input<{
        from: string;
        to: string;
    } | null>(null);
    readonly canEdit = input(true);
    readonly editClicked = output<void>();

    private analyticsService = inject(AnalyticsService);

    stats = signal<FunnelStats | null>(null);
    loading = signal(false);
    private lastLoadKey: string | null = null;

    ngOnChanges() {
        if (this.visible() && this.siteId() && this.funnelId() && this.dateRange()) {
            this.loadStats();
        }
    }

    loadStats() {
        const siteId = this.siteId();
        const funnelId = this.funnelId();
        const dateRange = this.dateRange();
        if (!siteId || !funnelId || !dateRange) return;
        const loadKey = `${siteId}:${funnelId}:${dateRange.from}:${dateRange.to}`;
        if (this.lastLoadKey === loadKey) return;
        this.lastLoadKey = loadKey;

        this.loading.set(true);
        this.analyticsService.getFunnelStats(siteId, funnelId, dateRange.from, dateRange.to).subscribe({
            next: (data) => {
                this.stats.set(data);
                this.loading.set(false);
            },
            error: () => this.loading.set(false)
        });
    }

    conversionClass(rate: number): string {
        const base = 'text-sm font-bold';
        if (rate >= 70) return `${base} text-green-600 dark:text-green-400`;
        if (rate >= 40) return `${base} text-yellow-600 dark:text-yellow-400`;
        return `${base} text-red-600 dark:text-red-400`;
    }

    onHide() {
        this.visible.set(false);
        this.stats.set(null);
        this.lastLoadKey = null;
    }

    protected onVisibleChange(visible: boolean) {
        if (visible) {
            this.visible.set(true);
            return;
        }
        this.onHide();
    }
}
