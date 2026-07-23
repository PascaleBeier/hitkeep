import { Component, input, ChangeDetectionStrategy, computed, effect, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { HttpErrorResponse } from '@angular/common/http';
import { TranslocoPipe } from '@jsverse/transloco';

import { ButtonModule } from '@openng/optimus-ui/button';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { CopyControl } from '@components/copy-control/copy-control';
import { Site } from '@models/analytics.types';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { SiteService } from '@features/sites/services/site.service';
import { domainValidator, sanitizeDomainInput } from '@features/sites/utils/domain-validator';

@Component({
    selector: 'app-site-general-settings',
    standalone: true,
    imports: [ReactiveFormsModule, ButtonModule, InputTextModule, RouterLink, CopyControl, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    templateUrl: './site-general-settings.html',
    styleUrl: './site-general-settings.css'
})
export class SiteGeneralSettings {
    site = input.required<Site | null>();
    private access = inject(AccessService);
    private siteService = inject(SiteService);

    protected readonly canRenameDomain = computed(() => {
        const site = this.site();
        return !!site && this.access.canSite(site.id, SITE_CAPABILITIES.manageData);
    });

    private readonly domainFormModel = signal({
        domain: new FormControl('', { nonNullable: true, validators: [Validators.required, domainValidator] })
    });
    protected readonly domainForm = compatForm(this.domainFormModel);
    protected readonly renaming = signal(false);
    protected readonly renameError = signal<string | null>(null);

    constructor() {
        effect(() => {
            const site = this.site();
            if (site) {
                this.domainForm.domain().control().setValue(site.domain, { emitEvent: false });
                this.renameError.set(null);
            }
        });
    }

    protected hasDomainChanged() {
        return this.domainForm.domain().value() !== (this.site()?.domain ?? '');
    }

    protected isDomainInvalid() {
        return this.domainForm.domain().invalid() && (this.domainForm.domain().dirty() || this.domainForm.domain().touched());
    }

    protected sanitizeDomain() {
        this.domainForm.domain().control().setValue(sanitizeDomainInput(this.domainForm.domain().value()));
    }

    protected saveDomain() {
        const site = this.site();
        if (!site || this.domainForm().invalid() || !this.hasDomainChanged()) return;

        this.renaming.set(true);
        this.renameError.set(null);
        this.siteService.renameSiteDomain(site.id, this.domainForm.domain().value()).subscribe({
            next: () => this.renaming.set(false),
            error: (error: HttpErrorResponse) => {
                this.renaming.set(false);
                this.renameError.set(error.status === 409 ? 'sites.settings.general.renameErrors.domainTaken' : 'sites.settings.general.renameErrors.renameFailed');
            }
        });
    }
}
