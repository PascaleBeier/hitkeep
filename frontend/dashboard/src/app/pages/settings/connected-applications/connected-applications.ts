import { ChangeDetectionStrategy, Component, DestroyRef, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageModule } from '@openng/optimus-ui/message';
import { finalize } from 'rxjs';

import { OAuthGrant } from '@models/oauth.types';
import { OAuthService } from '@services/oauth.service';
import { formatOAuthTime, oauthScopeLabelKey } from '@core/utils/oauth-display';
import { injectActiveLang } from '@core/i18n/active-lang';
import { PageFrame } from '@components/page-frame/page-frame';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { AppTable, AppTableCell, AppTableColumn } from '@components/table/table';
import { TableRowActionItem } from '@components/table-row-actions/table-row-actions';

@Component({
    selector: 'app-connected-applications',
    imports: [TranslocoPipe, ButtonModule, MessageModule, PageFrame, DialogShell, AppTable, AppTableCell],
    templateUrl: './connected-applications.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ConnectedApplicationsPage {
    protected readonly scopeLabelKey = oauthScopeLabelKey;
    private readonly oauth = inject(OAuthService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly transloco = inject(TranslocoService);
    private readonly language = injectActiveLang();
    protected readonly grants = signal<OAuthGrant[]>([]);
    protected readonly loading = signal(false);
    protected readonly loadError = signal<string | null>(null);
    protected readonly pending = signal(new Set<string>());
    protected readonly failures = signal(new Set<string>());
    protected readonly revoked = signal(false);
    protected readonly selectedGrant = signal<OAuthGrant | null>(null);
    protected readonly columns: AppTableColumn<OAuthGrant>[] = [
        { field: 'client.name', headerKey: 'oauth.application', frozen: true },
        { field: 'team.name', headerKey: 'oauth.team' },
        { field: 'created_at', headerKey: 'oauth.createdAt', type: 'date' },
        { field: 'last_used_at', headerKey: 'oauth.lastUsedAt', type: 'date' }
    ];
    protected readonly actions = (grant: OAuthGrant): TableRowActionItem[] => [
        { label: this.transloco.translate('oauth.connectionDetails'), icon: 'pi pi-info-circle', command: () => this.openDetails(grant) },
        { label: this.transloco.translate(this.pending().has(grant.id) ? 'oauth.revoking' : 'oauth.revoke'), icon: 'pi pi-ban', danger: true, disabled: this.pending().has(grant.id), command: () => this.revoke(grant) }
    ];

    protected openDetails(grant: OAuthGrant): void {
        this.selectedGrant.set(grant);
    }

    constructor() {
        this.load();
    }

    protected load(): void {
        if (this.loading()) return;
        this.loading.set(true);
        this.loadError.set(null);
        this.oauth
            .grants()
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.loading.set(false))
            )
            .subscribe({
                next: (grants) => this.grants.set(grants),
                error: (error: { status?: number }) => this.loadError.set(error.status === 401 || error.status === 403 ? 'oauth.permissionDenied' : 'oauth.loadFailed')
            });
    }

    protected revoke(grant: OAuthGrant): void {
        if (this.pending().has(grant.id)) return;
        this.pending.update((ids) => new Set(ids).add(grant.id));
        this.failures.update((ids) => {
            const next = new Set(ids);
            next.delete(grant.id);
            return next;
        });
        this.revoked.set(false);
        this.oauth
            .revoke(grant.id)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() =>
                    this.pending.update((ids) => {
                        const next = new Set(ids);
                        next.delete(grant.id);
                        return next;
                    })
                )
            )
            .subscribe({
                next: () => {
                    this.grants.update((grants) => grants.filter((item) => item.id !== grant.id));
                    if (this.selectedGrant()?.id === grant.id) this.selectedGrant.set(null);
                    this.revoked.set(true);
                },
                error: () => this.failures.update((ids) => new Set(ids).add(grant.id))
            });
    }

    protected formatTime(value: string | null | undefined): string {
        const language = this.language();
        if (!value) return this.transloco.translate('oauth.neverUsed');
        return formatOAuthTime(value, language) ?? this.transloco.translate('oauth.unavailable');
    }
}
