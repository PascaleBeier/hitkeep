import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { toSignal } from '@angular/core/rxjs-interop';
import { AbstractControl, FormControl, FormGroup, ReactiveFormsModule, ValidationErrors, ValidatorFn, Validators } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';
import { InputTextModule } from 'primeng/inputtext';
import { SelectModule } from 'primeng/select';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { CopyControl } from '@components/copy-control/copy-control';
import { SiteService } from '@features/sites/services/site.service';
import { SiteFavicon } from '@features/sites/components/site-favicon';
import { Site } from '@models/analytics.types';

function urlValidator(): ValidatorFn {
    return (control: AbstractControl): ValidationErrors | null => {
        const value = (control.value ?? '').trim();
        if (!value) return null;
        try {
            const url = new URL(value);
            return url.protocol === 'http:' || url.protocol === 'https:' ? null : { urlInvalid: true };
        } catch {
            return { urlInvalid: true };
        }
    };
}

@Component({
    selector: 'app-utm-builder',
    templateUrl: './utm-builder.html',
    styleUrl: './utm-builder.css',
    changeDetection: ChangeDetectionStrategy.OnPush,
    imports: [FormsModule, ReactiveFormsModule, TranslocoPipe, ButtonModule, InputTextModule, SelectModule, SiteFavicon, PageHeader, PageHeaderLeft, PageBreadcrumb, CopyControl]
})
export class UtmBuilder {
    private transloco = inject(TranslocoService);
    protected siteService = inject(SiteService);

    protected selectedSite = signal<Site | null>(null);

    protected form = new FormGroup({
        url: new FormControl('', { nonNullable: true, validators: [Validators.required, urlValidator()] }),
        source: new FormControl('', { nonNullable: true, validators: [Validators.required] }),
        medium: new FormControl('', { nonNullable: true }),
        campaign: new FormControl('', { nonNullable: true }),
        term: new FormControl('', { nonNullable: true }),
        content: new FormControl('', { nonNullable: true })
    });

    private formValues = toSignal(this.form.valueChanges, { initialValue: this.form.getRawValue() });
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    protected generatedUrl = computed(() => {
        const v = this.formValues();
        const rawUrl = v.url?.trim() ?? '';
        const source = v.source?.trim() ?? '';
        if (!rawUrl || !source) return '';
        try {
            const url = new URL(rawUrl);
            url.searchParams.set('utm_source', source);
            if (v.medium?.trim()) url.searchParams.set('utm_medium', v.medium.trim());
            if (v.campaign?.trim()) url.searchParams.set('utm_campaign', v.campaign.trim());
            if (v.term?.trim()) url.searchParams.set('utm_term', v.term.trim());
            if (v.content?.trim()) url.searchParams.set('utm_content', v.content.trim());
            return url.toString();
        } catch {
            return '';
        }
    });

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        const site = this.siteService.activeSite();
        return [...(site ? [{ label: site.domain, favicon: site, routerLink: '/dashboard' }] : []), { label: this.transloco.translate('nav.utm'), routerLink: '/utm' }, { label: this.transloco.translate('utmBuilder.breadcrumb'), isCurrent: true }];
    });

    protected readonly utmParamKeys = ['source', 'medium', 'campaign', 'term', 'content'] as const;

    protected prefillFromSite(site: Site | null): void {
        if (!site) return;
        this.form.controls.url.setValue(`https://${site.domain}`);
        this.form.controls.url.markAsDirty();
        // Reset the selector back to empty after the binding cycle completes
        setTimeout(() => this.selectedSite.set(null), 0);
    }

    protected clearForm() {
        this.form.reset();
        this.selectedSite.set(null);
    }
}
