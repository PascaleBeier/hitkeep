import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormField, form, maxLength, pattern, required } from '@angular/forms/signals';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';

import { CopyControl } from '@components/copy-control/copy-control';
import { TeamSSOConfig, UpdateTeamSSORequest } from '@models/analytics.types';
import { SettingsCard } from '@features/settings/components/settings-card';
import { TeamService } from '@services/team.service';

interface TeamSSOFormModel {
    issuerURL: string;
    clientID: string;
    clientSecret: string;
    allowedDomains: string;
    emailClaim: string;
    displayNameClaim: string;
    enabled: boolean;
}

const EMPTY_SSO_FORM: TeamSSOFormModel = {
    issuerURL: '',
    clientID: '',
    clientSecret: '',
    allowedDomains: '',
    emailClaim: 'email',
    displayNameClaim: 'name',
    enabled: false
};

@Component({
    selector: 'app-team-sso',
    imports: [TranslocoPipe, FormField, InputTextModule, ButtonModule, MessageModule, SettingsCard, CopyControl],
    templateUrl: './team-sso.html',
    styleUrl: './team-sso.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamSSOPage {
    private readonly destroyRef = inject(DestroyRef);
    protected readonly teamService = inject(TeamService);
    protected readonly team = this.teamService.activeTeam;

    protected readonly model = signal<TeamSSOFormModel>({ ...EMPTY_SSO_FORM });
    protected readonly ssoForm = form(this.model, (schema) => {
        required(schema.issuerURL);
        pattern(schema.issuerURL, /^https:\/\/[^\s]+$/);
        maxLength(schema.issuerURL, 2048);
        required(schema.clientID);
        maxLength(schema.clientID, 512);
        required(schema.allowedDomains);
        required(schema.emailClaim);
        pattern(schema.emailClaim, /^[A-Za-z_][A-Za-z0-9_.-]{0,127}$/);
        required(schema.displayNameClaim);
        pattern(schema.displayNameClaim, /^[A-Za-z_][A-Za-z0-9_.-]{0,127}$/);
    });
    protected readonly callbackURL = signal('');
    protected readonly clientSecretConfigured = signal(false);
    protected readonly persistedEnabled = signal(false);
    protected readonly isLoading = signal(false);
    protected readonly isSaving = signal(false);
    protected readonly isTesting = signal(false);
    protected readonly successKey = signal('');
    protected readonly errorKey = signal('');
    protected readonly canSave = computed(() => !this.isLoading() && !this.isSaving() && !this.isTesting() && !this.ssoForm().invalid() && (this.clientSecretConfigured() || this.model().clientSecret.trim().length > 0));

    constructor() {
        effect((onCleanup) => {
            const teamID = this.team()?.id;
            if (!teamID) {
                this.applyConfig(null);
                return;
            }
            this.isLoading.set(true);
            this.successKey.set('');
            this.errorKey.set('');
            const subscription = this.teamService
                .getTeamSSO(teamID)
                .pipe(takeUntilDestroyed(this.destroyRef))
                .subscribe({
                    next: (config) => {
                        this.applyConfig(config);
                        this.isLoading.set(false);
                    },
                    error: () => {
                        this.isLoading.set(false);
                        this.errorKey.set('admin.team.sso.errors.loadFailed');
                    }
                });
            onCleanup(() => subscription.unsubscribe());
        });
    }

    protected saveSettings(): void {
        if (this.isSaving()) {
            return;
        }
        if (this.ssoForm().invalid()) {
            this.touchRequiredFields();
            return;
        }
        const teamID = this.team()?.id;
        if (!teamID) {
            return;
        }

        this.successKey.set('');
        this.errorKey.set('');
        this.isSaving.set(true);
        this.teamService.updateTeamSSO(teamID, this.requestPayload()).subscribe({
            next: (config) => {
                this.applyConfig(config);
                this.isSaving.set(false);
                this.successKey.set('admin.team.sso.saveSuccess');
            },
            error: (err) => {
                this.isSaving.set(false);
                this.errorKey.set(err?.error?.code === 'domain_conflict' ? 'admin.team.sso.errors.domainConflict' : 'admin.team.sso.errors.saveFailed');
            }
        });
    }

    protected testConnection(): void {
        const teamID = this.team()?.id;
        if (!teamID || !this.clientSecretConfigured() || this.isTesting()) {
            return;
        }
        this.successKey.set('');
        this.errorKey.set('');
        this.isTesting.set(true);
        this.teamService.testTeamSSO(teamID).subscribe({
            next: () => {
                this.isTesting.set(false);
                this.successKey.set('admin.team.sso.testSuccess');
            },
            error: () => {
                this.isTesting.set(false);
                this.errorKey.set('admin.team.sso.errors.testFailed');
            }
        });
    }

    private requestPayload(): UpdateTeamSSORequest {
        const value = this.model();
        return {
            provider_type: 'oidc',
            issuer_url: value.issuerURL.trim(),
            client_id: value.clientID.trim(),
            client_secret: value.clientSecret,
            allowed_domains: value.allowedDomains
                .split(/[\s,]+/)
                .map((domain) => domain.trim())
                .filter(Boolean),
            email_claim: value.emailClaim.trim(),
            display_name_claim: value.displayNameClaim.trim(),
            enabled: value.enabled
        };
    }

    private applyConfig(config: TeamSSOConfig | null): void {
        this.model.set(
            config
                ? {
                      issuerURL: config.issuer_url,
                      clientID: config.client_id,
                      clientSecret: '',
                      allowedDomains: config.allowed_domains.join('\n'),
                      emailClaim: config.email_claim || 'email',
                      displayNameClaim: config.display_name_claim || 'name',
                      enabled: config.enabled
                  }
                : { ...EMPTY_SSO_FORM }
        );
        this.callbackURL.set(config?.callback_url ?? '');
        this.clientSecretConfigured.set(config?.client_secret_configured ?? false);
        this.persistedEnabled.set(config?.enabled ?? false);
    }

    private touchRequiredFields(): void {
        this.ssoForm.issuerURL().markAsTouched();
        this.ssoForm.clientID().markAsTouched();
        this.ssoForm.allowedDomains().markAsTouched();
        this.ssoForm.emailClaim().markAsTouched();
        this.ssoForm.displayNameClaim().markAsTouched();
    }
}
