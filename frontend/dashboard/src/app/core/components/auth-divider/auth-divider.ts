import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { DividerModule } from '@openng/optimus-ui/divider';

import { AUTH_DIVIDER_DESIGN_TOKENS } from '@core/theme/hitkeep-preset';

@Component({
    selector: 'app-auth-divider',
    imports: [DividerModule, TranslocoPipe],
    template: `<p-divider align="center" [dt]="designTokens">{{ labelKey() | transloco }}</p-divider>`,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AuthDivider {
    readonly labelKey = input.required<string>();
    protected readonly designTokens = AUTH_DIVIDER_DESIGN_TOKENS;
}
