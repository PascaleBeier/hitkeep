import { Component, input, output, ChangeDetectionStrategy, effect, signal } from '@angular/core';

import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { TranslocoPipe } from '@jsverse/transloco';
import { MenuItem } from '@openng/optimus-ui/api';
import { ButtonModule } from '@openng/optimus-ui/button';
import { Select, SelectModule } from '@openng/optimus-ui/select';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { Site } from '@models/analytics.types';
import { SiteSelectOption } from '@features/sites/components/site-select-option';
@Component({
    selector: 'app-site-selector',
    standalone: true,
    imports: [ButtonModule, ReactiveFormsModule, SelectModule, SkeletonModule, SiteSelectOption, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    styleUrl: './site-selector.css',
    template: `
        <div class="flex w-full flex-col gap-2" role="region" [attr.aria-label]="'sites.selector.regionAria' | transloco">
            @if (loading()) {
                <p-skeleton height="40px" class="rounded-md" />
            } @else {
                <p-select
                    #siteSelect
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
                    <ng-template pTemplate="footer">
                        @if (actions().length) {
                            <div class="border-t border-surface p-1">
                                @for (action of actions(); track action.label) {
                                    <p-button
                                        [label]="action.label"
                                        [icon]="action.icon"
                                        [ariaLabel]="action.label"
                                        [disabled]="disabled() || action.disabled"
                                        [text]="true"
                                        severity="secondary"
                                        size="small"
                                        [style]="{ width: '100%', justifyContent: 'flex-start' }"
                                        (onClick)="onAction(action, $event, siteSelect)"
                                    />
                                }
                            </div>
                        }
                    </ng-template>
                </p-select>
            }
        </div>
    `
})
export class SiteSelector {
    private readonly siteFormModel = signal({
        selectedSite: new FormControl<Site | null>(null)
    });
    protected readonly siteForm = compatForm(this.siteFormModel);

    sites = input.required<Site[]>();
    current = input<Site | null>(null);
    loading = input<boolean>(false);
    readonly disabled = input(false);
    readonly actions = input<MenuItem[]>([]);
    siteSelected = output<Site>();

    constructor() {
        effect(() => {
            const control = this.siteForm.selectedSite().control();
            if (this.disabled()) control.disable({ emitEvent: false });
            else control.enable({ emitEvent: false });
        });
        effect(() => {
            this.siteForm.selectedSite().control().setValue(this.current(), { emitEvent: false });
        });
    }

    protected onAction(item: MenuItem, event: Event, select: Select): void {
        if (this.disabled() || item.disabled || !this.actions().includes(item)) return;
        select.hide();
        item.command?.({ originalEvent: event, item });
    }

    protected onSiteChange(site: Site | null): void {
        if (!site) {
            return;
        }
        this.siteSelected.emit(site);
    }
}
