import { Component, input, output, ChangeDetectionStrategy, computed, inject, effect, signal, viewChild } from '@angular/core';

import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { TranslocoPipe } from '@jsverse/transloco';
import { SelectModule } from '@openng/optimus-ui/select';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { Site } from '@models/analytics.types';
import { SiteSelectOption } from '@features/sites/components/site-select-option';
import { ShareDashboardLink } from '@features/share/components/share-dashboard-link';
import { AccessService } from '@services/access.service';
import { ShareService } from '@services/share.service';
@Component({
    selector: 'app-site-selector',
    standalone: true,
    imports: [ReactiveFormsModule, SelectModule, SkeletonModule, SiteSelectOption, ShareDashboardLink, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    styleUrl: './site-selector.css',
    template: `
        <div class="flex flex-col gap-2 w-full" role="region" [attr.aria-label]="'sites.selector.regionAria' | transloco">
            <div class="flex items-center justify-between">
                <label for="site-dropdown" class="text-xs font-semibold text-[var(--p-text-muted-color)] uppercase"> {{ 'sites.selector.sitesLabel' | transloco }} </label>
                @if (!shareService.isShareMode()) {
                    <button
                        type="button"
                        (click)="addClicked.emit()"
                        class="cursor-pointer flex items-center justify-center size-6 rounded-md border border-surface-200 dark:border-surface-700 text-muted-color hover:text-[var(--p-text-color)] hover:bg-surface-100 dark:hover:bg-surface-800 transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500"
                        [attr.aria-label]="'sites.selector.addSiteAria' | transloco"
                    >
                        <i class="pi pi-plus text-xs" aria-hidden="true"></i>
                    </button>
                }
            </div>

            @if (loading()) {
                <p-skeleton height="40px" class="rounded-md" />
            } @else {
                <div class="flex flex-col gap-2">
                    @if (sites().length > 0) {
                        <p-select
                            inputId="site-dropdown"
                            [options]="sites()"
                            [formControl]="siteForm.selectedSite().control()"
                            [filter]="true"
                            filterBy="domain"
                            dataKey="id"
                            (onChange)="onSiteChange($event.value)"
                            optionLabel="domain"
                            [placeholder]="'sites.selector.selectPlaceholder' | transloco"
                            class="site-selector__select w-full text-sm"
                            [attr.aria-label]="'sites.selector.selectSiteAria' | transloco"
                        >
                            <ng-template pTemplate="selectedItem" let-selected>
                                <app-site-select-option [site]="selected" [selected]="true" />
                            </ng-template>

                            <ng-template pTemplate="item" let-site>
                                <app-site-select-option [site]="site" />
                            </ng-template>
                        </p-select>
                    }

                    @if (sites().length > 0) {
                        @if (!shareService.isShareMode()) {
                            <div class="mt-1 flex items-center gap-1 px-1">
                                <button
                                    type="button"
                                    (click)="settingsClicked.emit()"
                                    class="cursor-pointer flex h-8 flex-1 items-center justify-center rounded-md text-muted-color transition-colors hover:bg-surface-100 hover:text-[var(--p-text-color)] focus:outline-none focus:ring-2 focus:ring-primary-500 dark:hover:bg-surface-800"
                                    [attr.aria-label]="'sites.selector.siteSettingsAria' | transloco"
                                    [title]="'sites.selector.siteSettingsAria' | transloco"
                                >
                                    <i class="pi pi-cog text-sm" aria-hidden="true"></i>
                                </button>
                                <button
                                    type="button"
                                    (click)="trackingClicked.emit()"
                                    class="cursor-pointer flex h-8 flex-1 items-center justify-center rounded-md text-muted-color transition-colors hover:bg-surface-100 hover:text-[var(--p-text-color)] focus:outline-none focus:ring-2 focus:ring-primary-500 dark:hover:bg-surface-800"
                                    [attr.aria-label]="'sites.selector.trackingCodeAria' | transloco"
                                    [title]="'sites.selector.trackingCodeAria' | transloco"
                                >
                                    <i class="pi pi-code text-sm" aria-hidden="true"></i>
                                </button>
                                <button
                                    type="button"
                                    (click)="openShareDialog()"
                                    class="cursor-pointer flex h-8 flex-1 items-center justify-center rounded-md text-muted-color transition-colors hover:bg-surface-100 hover:text-[var(--p-text-color)] focus:outline-none focus:ring-2 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-surface-800"
                                    [attr.aria-label]="'sites.selector.shareDashboardAria' | transloco"
                                    [title]="'sites.selector.shareDashboardAria' | transloco"
                                    [disabled]="!canShareCurrentSite()"
                                >
                                    <i class="pi pi-share-alt text-sm" aria-hidden="true"></i>
                                </button>
                            </div>
                            @defer (when shareDialogLoaded()) {
                                <app-share-dashboard-link #shareDialog />
                            }
                        }
                    }
                </div>
            }
        </div>
    `
})
export class SiteSelector {
    protected shareService = inject(ShareService);
    private access = inject(AccessService);
    private readonly shareDialog = viewChild<ShareDashboardLink>('shareDialog');
    private readonly siteFormModel = signal({
        selectedSite: new FormControl<Site | null>(null)
    });
    protected readonly siteForm = compatForm(this.siteFormModel);
    protected readonly shareDialogLoaded = signal(false);
    private readonly pendingShareDialogOpen = signal(false);

    sites = input.required<Site[]>();
    current = input<Site | null>(null);
    loading = input<boolean>(false);
    siteSelected = output<Site>();
    addClicked = output<void>();
    settingsClicked = output<void>();
    trackingClicked = output<void>();
    protected readonly canShareCurrentSite = computed(() => {
        const site = this.current();
        return !!site && this.access.canSite(site.id, SITE_CAPABILITIES.manageTeam);
    });

    constructor() {
        effect(() => {
            this.siteForm.selectedSite().control().setValue(this.current(), { emitEvent: false });
        });

        effect(() => {
            if (!this.pendingShareDialogOpen()) {
                return;
            }

            const dialog = this.shareDialog();
            if (!dialog) {
                return;
            }

            dialog.open();
            this.pendingShareDialogOpen.set(false);
        });
    }

    protected onSiteChange(site: Site | null): void {
        if (!site) {
            return;
        }
        this.siteSelected.emit(site);
    }

    protected openShareDialog() {
        if (!this.canShareCurrentSite()) {
            return;
        }

        this.shareDialogLoaded.set(true);
        this.pendingShareDialogOpen.set(true);
    }
}
