import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { CardModule } from '@openng/optimus-ui/card';

/**
 * Centred card shown when an analytics page has no data because the underlying
 * feature was never set up. Each page supplies its own icon, copy and guide.
 */
@Component({
    selector: 'app-setup-callout',
    imports: [TranslocoPipe, ButtonModule, CardModule],
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: { '[attr.data-testid]': 'testId()' },
    template: `
        <p-card class="shadow-sm border border-surface-200 dark:border-surface-700 surface-card">
            <div class="flex flex-col items-center gap-3 py-8 text-center">
                <i [class]="iconClass()" aria-hidden="true"></i>
                <h3 class="text-lg font-semibold">{{ titleKey() | transloco }}</h3>
                <p class="max-w-xl text-sm text-[var(--p-text-muted-color)]">{{ descriptionKey() | transloco }}</p>
                <a pButton [href]="docsUrl()" target="_blank" rel="noreferrer" size="small" icon="pi pi-external-link" iconPos="right" [label]="docsActionKey() | transloco" [attr.aria-label]="docsActionKey() | transloco" class="p-button-rounded"></a>
            </div>
        </p-card>
    `
})
export class SetupCallout {
    /** Icon class, e.g. `pi pi-cloud-upload`. */
    icon = input.required<string>();
    /** Translation key for the headline. */
    titleKey = input.required<string>();
    /** Translation key for the explanatory line. */
    descriptionKey = input.required<string>();
    /** Absolute URL of the setup guide. */
    docsUrl = input.required<string>();
    /** Translation key for the guide button label. */
    docsActionKey = input.required<string>();
    /** Rendered as `data-testid` on the host so page and e2e specs keep their selectors. */
    testId = input.required<string>();

    protected readonly iconClass = computed(() => `${this.icon()} text-5xl text-primary opacity-60`);
}
