import { ChangeDetectionStrategy, Component, input } from '@angular/core';

@Component({
    selector: 'app-page-frame',
    template: `
        <div class="page-frame">
            @if (subtitle()) {
                <p class="page-frame__subtitle page-frame__subtitle--intro">{{ subtitle() }}</p>
            }
            <div class="page-frame__body">
                <ng-content />
            </div>
        </div>
    `,
    styleUrl: './page-frame.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class PageFrame {
    subtitle = input('');
}
