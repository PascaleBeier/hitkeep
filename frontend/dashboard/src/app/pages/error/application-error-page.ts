import { Location } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { Router } from '@angular/router';
import { TranslocoService } from '@jsverse/transloco';

import { ApplicationState } from '@core/components/application-state/application-state';
import { injectActiveLang } from '@core/i18n/active-lang';
import { ApplicationErrorKind, ApplicationErrorState, readApplicationErrorState } from '@services/application-error-navigation.service';

interface ErrorPresentation {
    titleKey: string;
    messageKey: string;
    icon: string;
}

@Component({
    selector: 'app-application-error-page',
    imports: [ApplicationState],
    template: `
        <app-application-state
            [statusLabel]="statusLabel()"
            [titleKey]="presentation().titleKey"
            [messageKey]="presentation().messageKey"
            [icon]="presentation().icon"
            [danger]="true"
            [primaryActionLabelKey]="primaryActionLabelKey()"
            [primaryActionIcon]="primaryActionIcon()"
            [secondaryActionLabelKey]="secondaryActionLabelKey()"
            [secondaryActionIcon]="secondaryActionIcon()"
            (primaryAction)="onPrimaryAction()"
            (secondaryAction)="onSecondaryAction()"
        />
    `,
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ApplicationErrorPage {
    private readonly router = inject(Router);
    private readonly location = inject(Location);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = injectActiveLang();
    private readonly state = readApplicationErrorState(this.location.getState());
    protected readonly applicationErrorKind = input<ApplicationErrorKind | 'not-found'>();
    private readonly notFound = computed(() => this.applicationErrorKind() === 'not-found');

    protected readonly kind = computed<ApplicationErrorKind | 'not-found'>(() => (this.notFound() ? 'not-found' : (this.state?.kind ?? 'generic')));
    protected readonly presentation = computed<ErrorPresentation>(() => this.resolvePresentation());
    protected readonly statusLabel = computed(() => {
        this.activeLanguage();
        if (this.notFound()) return this.transloco.translate('applicationError.status.http', { status: 404 });
        if (this.state?.status) return this.transloco.translate('applicationError.status.http', { status: this.state.status });
        if (this.kind() === 'offline') return this.transloco.translate('applicationError.status.offline');
        if (this.kind() === 'navigation') return this.transloco.translate('applicationError.status.navigation');
        return '';
    });
    protected readonly primaryActionLabelKey = computed(() => {
        if (this.kind() === 'not-found' || this.kind() === 'generic') return 'applicationError.actions.dashboard';
        if (this.kind() === 'navigation') return 'applicationError.actions.reloadPage';
        return 'applicationError.actions.tryAgain';
    });
    protected readonly primaryActionIcon = computed(() => (this.kind() === 'not-found' || this.kind() === 'generic' ? 'pi pi-home' : 'pi pi-refresh'));
    protected readonly secondaryActionLabelKey = computed(() => (this.kind() === 'not-found' ? 'applicationError.actions.goBack' : this.kind() === 'generic' ? undefined : 'applicationError.actions.dashboard'));
    protected readonly secondaryActionIcon = computed(() => (this.kind() === 'not-found' ? 'pi pi-arrow-left' : 'pi pi-home'));

    protected onPrimaryAction(): void {
        const kind = this.kind();
        if (kind === 'not-found' || kind === 'generic') {
            void this.goToDashboard();
            return;
        }
        if (kind === 'navigation') {
            this.hardReloadTarget();
            return;
        }
        void this.router.navigateByUrl(this.state?.returnUrl ?? '/dashboard', { replaceUrl: true });
    }

    protected onSecondaryAction(): void {
        if (this.kind() === 'not-found') {
            this.location.back();
            return;
        }
        void this.goToDashboard();
    }

    private resolvePresentation(): ErrorPresentation {
        const kind = this.kind();
        if (kind === 'not-found') {
            return { titleKey: 'applicationError.notFound.title', messageKey: 'applicationError.notFound.message', icon: 'pi pi-compass' };
        }

        const contextMessage = this.contextMessageKey(this.state);
        const presentations: Record<ApplicationErrorKind, ErrorPresentation> = {
            offline: { titleKey: 'applicationError.offline.title', messageKey: contextMessage ?? 'applicationError.offline.message', icon: 'pi pi-wifi' },
            client: { titleKey: 'applicationError.client.title', messageKey: contextMessage ?? 'applicationError.client.message', icon: 'pi pi-exclamation-circle' },
            server: { titleKey: 'applicationError.server.title', messageKey: contextMessage ?? 'applicationError.server.message', icon: 'pi pi-server' },
            navigation: { titleKey: 'applicationError.navigation.title', messageKey: 'applicationError.navigation.message', icon: 'pi pi-file' },
            generic: { titleKey: 'applicationError.generic.title', messageKey: contextMessage ?? 'applicationError.generic.message', icon: 'pi pi-exclamation-triangle' }
        };
        return presentations[kind];
    }

    private contextMessageKey(state: ApplicationErrorState | null): string | null {
        if (!state || state.context === 'navigation') return null;
        const keys: Record<Exclude<ApplicationErrorState['context'], 'navigation'>, string> = {
            'setup-status': 'applicationError.context.setupStatus',
            'dashboard-bootstrap': 'applicationError.context.dashboardBootstrap',
            'cloud-signup-status': 'applicationError.context.cloudSignupStatus',
            'cloud-signup-profile': 'applicationError.context.cloudSignupProfile'
        };
        return keys[state.context];
    }

    private hardReloadTarget(): void {
        this.location.replaceState(this.state?.returnUrl ?? '/dashboard');
        this.location.historyGo(0);
    }

    private goToDashboard(): Promise<boolean> {
        return this.router.navigateByUrl('/dashboard', { replaceUrl: true });
    }
}
