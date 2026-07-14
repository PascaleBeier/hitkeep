import { ChangeDetectionStrategy, Component } from '@angular/core';
import { CardModule } from 'primeng/card';

import { AUTH_CARD_DESIGN_TOKENS } from '@core/theme/hitkeep-preset';

@Component({
    selector: 'app-auth-card',
    imports: [CardModule],
    template: `<p-card class="hk-auth-card" [dt]="designTokens"><ng-content /></p-card>`,
    styles: `
        :host {
            display: block;
        }
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AuthCard {
    protected readonly designTokens = AUTH_CARD_DESIGN_TOKENS;
}
