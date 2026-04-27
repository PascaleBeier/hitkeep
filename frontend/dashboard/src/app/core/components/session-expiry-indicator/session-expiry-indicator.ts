import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';
import { DialogModule } from 'primeng/dialog';
import { formatDurationInterval } from '@core/i18n/duration-format';
import { AuthService } from '@services/auth.service';

@Component({
    selector: 'app-session-expiry-indicator',
    standalone: true,
    changeDetection: ChangeDetectionStrategy.OnPush,
    imports: [ButtonModule, DialogModule, TranslocoPipe],
    templateUrl: './session-expiry-indicator.html',
    styleUrl: './session-expiry-indicator.css'
})
export class SessionExpiryIndicator {
    protected readonly auth = inject(AuthService);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });
    protected readonly extendError = signal(false);
    protected readonly remainingLabel = computed(() => formatDurationInterval(this.auth.sessionDisplayRemainingSeconds(), this.activeLanguage()));
    protected readonly policyDurationLabel = computed(() => formatDurationInterval(this.auth.session()?.duration_seconds ?? 0, this.activeLanguage()));
    protected readonly warningVisible = computed(() => this.auth.sessionWarningActive());

    extendSession() {
        this.extendError.set(false);
        this.auth.extendSession().subscribe({
            error: () => this.extendError.set(true)
        });
    }

    signOut() {
        this.auth.logout().subscribe();
    }
}
