import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';

import { PageFrame } from '@components/page-frame/page-frame';
import { TEAM_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';

@Component({
    selector: 'app-assistants',
    imports: [PageFrame, RouterLink, TranslocoPipe, ButtonModule, InputTextModule, MessageModule],
    templateUrl: './assistants.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AssistantsPage {
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly access = inject(AccessService);

    protected readonly endpoint = computed(() => {
        const connection = this.bootstrap.assistants();
        return connection.enabled ? (connection.endpoint ?? '') : '';
    });
    protected readonly canManageClients = computed(() => this.access.canActiveTeam(TEAM_CAPABILITIES.manageApiClients));
    protected readonly copying = signal(false);
    protected readonly copyFeedback = signal('');

    protected async copyEndpoint(): Promise<void> {
        const endpoint = this.endpoint();
        if (!endpoint || this.copying()) {
            return;
        }
        this.copying.set(true);
        this.copyFeedback.set('');
        try {
            await navigator.clipboard.writeText(endpoint);
            this.copyFeedback.set('assistants.copied');
        } catch {
            this.copyFeedback.set('assistants.copyFailed');
        } finally {
            this.copying.set(false);
        }
    }
}
