import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';

import { CopyControl } from '@components/copy-control/copy-control';

@Component({
    selector: 'app-one-time-credential',
    imports: [CopyControl, TranslocoPipe],
    templateUrl: './one-time-credential.html',
    styleUrl: './one-time-credential.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class OneTimeCredential {
    readonly value = input.required<string>();
    readonly titleKey = input.required<string>();
    readonly descriptionKey = input.required<string>();
    readonly copyLabelKey = input('common.copyControl.copy');
}
