import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { ActivatedRoute } from '@angular/router';
import { finalize } from 'rxjs';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';
import { MessageModule } from 'primeng/message';

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
    private readonly token = inject(ActivatedRoute).snapshot.queryParamMap.get('token')?.trim() ?? '';

    protected readonly state = signal<ConfirmationState>('loading');
    protected readonly confirmation = signal<ReportRecipientConfirmation | null>(null);

    constructor() {
        if (!this.token) {
            this.state.set('invalid');
            return;
        }
        this.service.confirmation(this.token).subscribe({
            next: (confirmation) => {
                this.confirmation.set(confirmation);
                this.state.set('ready');
            },
            error: () => this.state.set('invalid')
        });
    }

    protected decide(action: 'confirm' | 'decline'): void {
        if (this.state() !== 'ready') return;
        this.state.set('submitting');
        this.service
            .decideConfirmation(this.token, action)
            .pipe(finalize(() => this.state.update((state) => (state === 'submitting' ? 'invalid' : state))))
            .subscribe({
                next: () => this.state.set(action === 'confirm' ? 'confirmed' : 'declined'),
                error: () => this.state.set('invalid')
            });
    }
}
