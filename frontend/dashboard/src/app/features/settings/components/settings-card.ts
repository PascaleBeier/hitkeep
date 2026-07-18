import { ChangeDetectionStrategy, Component, input } from '@angular/core';

@Component({
    selector: 'app-settings-card',
    templateUrl: './settings-card.html',
    styleUrl: './settings-card.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SettingsCard {
    readonly title = input.required<string>();
    readonly subtitle = input('');
    readonly icon = input('');
    readonly titleId = input('');
    readonly tone = input<'default' | 'danger'>('default');
}
