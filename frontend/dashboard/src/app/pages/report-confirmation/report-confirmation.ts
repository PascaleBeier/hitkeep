import { ChangeDetectionStrategy, Component, effect, inject, input, signal } from '@angular/core';
import { finalize, Subscription } from 'rxjs';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageModule } from '@openng/optimus-ui/message';

import { Brand } from '@components/brand/brand';
import { AuthCard } from '@core/components/auth-card/auth-card';
import { ReportRecipientConfirmation } from '@models/analytics.types';
import { ReportDefinitionsService } from '@services/report-definitions.service';

type ConfirmationState = 'loading' | 'ready' | 'submitting' | 'confirmed' | 'declined' | 'invalid';

@Component({
    selector: 'app-report-confirmation',
    imports: [AuthCard, Brand, ButtonModule, MessageModule, TranslocoPipe],
    templateUrl: './report-confirmation.html',
    styleUrl: './report-confirmation.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ReportConfirmation {
    private readonly service = inject(ReportDefinitionsService);
    private decisionRequest: Subscription | null = null;
    protected readonly token = input<string>();

    protected readonly state = signal<ConfirmationState>('loading');
    protected readonly confirmation = signal<ReportRecipientConfirmation | null>(null);

    constructor() {
        effect((onCleanup) => {
            const token = this.token()?.trim();
            this.decisionRequest?.unsubscribe();
            this.decisionRequest = null;
            this.confirmation.set(null);
            if (!token) {
                this.state.set('invalid');
                return;
            }

            this.state.set('loading');
            const subscription = this.service.confirmation(token).subscribe({
                next: (confirmation) => {
                    this.confirmation.set(confirmation);
                    this.state.set('ready');
                },
                error: () => this.state.set('invalid')
            });
            onCleanup(() => {
                subscription.unsubscribe();
                this.decisionRequest?.unsubscribe();
                this.decisionRequest = null;
            });
        });
    }

    protected decide(action: 'confirm' | 'decline'): void {
        if (this.state() !== 'ready') return;
        const token = this.token()?.trim();
        if (!token) return;
        this.state.set('submitting');
        this.decisionRequest = this.service
            .decideConfirmation(token, action)
            .pipe(finalize(() => this.state.update((state) => (state === 'submitting' ? 'invalid' : state))))
            .subscribe({
                next: () => this.state.set(action === 'confirm' ? 'confirmed' : 'declined'),
                error: () => this.state.set('invalid')
            });
    }
}
