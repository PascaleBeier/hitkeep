import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';

import { AuthCard } from '@core/components/auth-card/auth-card';
import { Brand } from '@components/brand/brand';

@Component({
    selector: 'app-application-state',
    imports: [AuthCard, Brand, ButtonModule, TranslocoPipe],
    template: `
        <main class="hk-auth-page application-state-page">
            <section class="hk-auth-shell application-state-shell" [attr.aria-labelledby]="titleId()" [attr.role]="danger() ? 'alert' : 'status'" [attr.aria-live]="danger() ? 'assertive' : 'polite'" aria-atomic="true">
                <app-auth-card>
                    <div class="application-state-body">
                        <app-brand size="large" />

                        @if (statusLabel()) {
                            <span class="application-state-status">{{ statusLabel() }}</span>
                        }

                        <span class="application-state-icon" [class.application-state-icon--danger]="danger()">
                            <i [class]="icon()" aria-hidden="true"></i>
                        </span>

                        <div class="application-state-copy">
                            <h1 [id]="titleId()">{{ titleKey() | transloco }}</h1>
                            <p>{{ messageKey() | transloco }}</p>
                        </div>

                        @if (primaryActionLabelKey()) {
                            <div class="application-state-actions">
                                <p-button class="application-state-action" [label]="primaryActionLabelKey()! | transloco" [icon]="primaryActionIcon()" [fluid]="true" (onClick)="primaryAction.emit()" />
                                @if (secondaryActionLabelKey()) {
                                    <p-button class="application-state-action" [label]="secondaryActionLabelKey()! | transloco" [icon]="secondaryActionIcon()" severity="secondary" [outlined]="true" [fluid]="true" (onClick)="secondaryAction.emit()" />
                                }
                            </div>
                        }
                    </div>
                </app-auth-card>
            </section>
        </main>
    `,
    styleUrl: './application-state.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ApplicationState {
    titleKey = input.required<string>();
    messageKey = input.required<string>();
    titleId = input('application-state-title');
    icon = input('pi pi-info-circle');
    statusLabel = input('');
    danger = input(false);
    primaryActionLabelKey = input<string>();
    primaryActionIcon = input('pi pi-refresh');
    secondaryActionLabelKey = input<string>();
    secondaryActionIcon = input('pi pi-home');
    primaryAction = output<void>();
    secondaryAction = output<void>();
}
