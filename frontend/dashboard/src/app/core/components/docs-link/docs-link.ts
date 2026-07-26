import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';

/** Outlined for the primary guide of a surface, text for the supporting ones. */
export type DocsLinkVariant = 'outlined' | 'text';

/**
 * Button-styled link out to an external guide. Centralizes the `target`/`rel` pairing
 * and the accessible name, which every hand-rolled copy of this anchor has to repeat.
 */
@Component({
    selector: 'app-docs-link',
    standalone: true,
    imports: [ButtonModule, TranslocoPipe],
    template: `
        @let label = labelKey() | transloco;
        <a
            pButton
            [href]="href()"
            target="_blank"
            rel="noreferrer"
            size="small"
            severity="secondary"
            [outlined]="variant() === 'outlined'"
            [text]="variant() === 'text'"
            icon="pi pi-external-link"
            iconPos="right"
            [label]="label"
            [attr.aria-label]="label"
            [attr.data-testid]="testId() || null"
        ></a>
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class DocsLink {
    readonly href = input.required<string>();
    /** Translation key for the visible label, which also becomes the accessible name. */
    readonly labelKey = input.required<string>();
    readonly variant = input<DocsLinkVariant>('outlined');
    readonly testId = input('');
}
