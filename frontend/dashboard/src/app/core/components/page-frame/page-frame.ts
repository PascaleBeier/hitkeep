import { ChangeDetectionStrategy, Component, input } from '@angular/core';

@Component({
    selector: 'app-page-frame',
    template: `
        <div class="page-frame">
            @if (heading()) {
                <h1 class="page-frame__heading">{{ heading() }}</h1>
            }
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
    heading = input('');
}
