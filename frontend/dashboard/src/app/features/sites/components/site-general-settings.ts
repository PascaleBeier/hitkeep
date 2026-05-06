import { Component, input, ChangeDetectionStrategy } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';

import { ButtonModule } from 'primeng/button';
import { CopyControl } from '@components/copy-control/copy-control';
import { Site } from '@models/analytics.types';

@Component({
    selector: 'app-site-general-settings',
    standalone: true,
    imports: [ButtonModule, RouterLink, CopyControl, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    templateUrl: './site-general-settings.html',
    styleUrl: './site-general-settings.css'
})
export class SiteGeneralSettings {
    site = input.required<Site | null>();
}
