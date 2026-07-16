import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';

import { SocialProviderIcon } from '@core/components/social-provider-icon/social-provider-icon';
import type { SocialProviderID } from '@services/auth.service';

export interface AuthMethodOption {
    readonly id: string;
    readonly labelKey: string;
    readonly icon?: string;
    readonly providerIcon?: SocialProviderID;
    readonly wide?: boolean;
    readonly loading?: boolean;
    readonly disabled?: boolean;
}

@Component({
    selector: 'app-auth-methods',
    imports: [ButtonModule, SocialProviderIcon, TranslocoPipe],
    templateUrl: './auth-methods.html',
    styleUrl: './auth-methods.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AuthMethods {
    readonly methods = input.required<readonly AuthMethodOption[]>();
    readonly ariaLabelKey = input('login.authenticationMethods');
    readonly methodSelected = output<string>();

    protected selectMethod(method: AuthMethodOption): void {
        if (method.disabled || method.loading) {
            return;
        }
        this.methodSelected.emit(method.id);
    }
}
