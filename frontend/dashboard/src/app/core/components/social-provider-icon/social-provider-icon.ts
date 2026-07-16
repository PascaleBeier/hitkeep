import { ChangeDetectionStrategy, Component, input } from '@angular/core';

import type { SocialProviderID } from '@services/auth.service';

@Component({
    selector: 'app-social-provider-icon',
    templateUrl: './social-provider-icon.html',
    styleUrl: './social-provider-icon.css',
    host: {
        'aria-hidden': 'true',
        '[attr.data-provider-icon]': 'provider()'
    },
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SocialProviderIcon {
    readonly provider = input.required<SocialProviderID>();
}
