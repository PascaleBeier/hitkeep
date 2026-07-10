import { ChangeDetectionStrategy, Component, DestroyRef, OnInit, computed, inject, signal } from '@angular/core';
import { ReactiveFormsModule, NonNullableFormBuilder, Validators } from '@angular/forms';
import { takeUntilDestroyed, toObservable, toSignal } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ConfirmationService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { ConfirmDialogModule } from 'primeng/confirmdialog';
import { DialogModule } from 'primeng/dialog';
import { IconFieldModule } from 'primeng/iconfield';
import { InputIconModule } from 'primeng/inputicon';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';
import { TableModule } from 'primeng/table';
import { TagModule } from 'primeng/tag';
import { TextareaModule } from 'primeng/textarea';
import { catchError, distinctUntilChanged, finalize, forkJoin, map, Observable, of, switchMap, tap } from 'rxjs';

import { OneTimeCredential } from '@components/one-time-credential/one-time-credential';
import { dialogCancelButton, dialogDangerButton, dialogPrimaryButton } from '@components/dialog-actions/dialog-actions';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';
import { PageState } from '@components/page-state/page-state';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';
import { INSTANCE_CAPABILITIES, SITE_CAPABILITIES } from '@core/access/capabilities';
import { SiteService } from '@features/sites/services/site.service';
import { AccessService } from '@services/access.service';
import { Webhook, WebhookDelivery, WebhookEventDescriptor, WebhookInput, WebhookSecretResponse, WebhookScope, WebhooksService } from '@services/webhooks.service';

@Component({
    selector: 'app-webhooks-page',
    imports: [
        ReactiveFormsModule,
        TranslocoPipe,
        ButtonModule,
        ConfirmDialogModule,
        DialogModule,
        IconFieldModule,
        InputIconModule,
        InputTextModule,
        MessageModule,
        TableModule,
        TagModule,
        TextareaModule,
        PageHeader,
        PageHeaderLeft,
        PageBreadcrumb,
        PageState,
        OneTimeCredential,
        RelativeDateTime,
        TableRowActions
    ],
    providers: [ConfirmationService],
    templateUrl: './webhooks.html',
    styleUrl: './webhooks.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class WebhooksPage implements OnInit {
    private readonly service = inject(WebhooksService);
    private readonly sites = inject(SiteService);
    private readonly access = inject(AccessService);
    private readonly fb = inject(NonNullableFormBuilder);
    private readonly confirmation = inject(ConfirmationService);
    private readonly transloco = inject(TranslocoService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly language = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    protected readonly activeSite = this.sites.activeSite;
    protected readonly canManageInstance = computed(() => this.access.hasInstance(INSTANCE_CAPABILITIES.manageWebhooks));
    protected readonly canManageSite = computed(() => !!this.activeSite() && this.access.canActiveSite(SITE_CAPABILITIES.manageWebhooks));
    protected readonly canManageCurrentScope = computed(() => (this.scope() === 'instance' ? this.canManageInstance() : this.canManageSite()));
    protected readonly scope = signal<WebhookScope>('site');
    protected readonly webhooks = signal<Webhook[]>([]);
    protected readonly catalog = signal<WebhookEventDescriptor[]>([]);
    protected readonly deliveries = signal<WebhookDelivery[]>([]);
    protected readonly selectedWebhook = signal<Webhook | null>(null);
    protected readonly loading = signal(false);
    protected readonly saving = signal(false);
    protected readonly deliveryLoading = signal(false);
    protected readonly deliveryError = signal<string | null>(null);
    protected readonly dialogVisible = signal(false);
    protected readonly editing = signal<Webhook | null>(null);
    protected readonly selectedEvents = signal<string[]>([]);
    protected readonly revealedSecret = signal('');
    protected readonly feedback = signal<{ severity: 'success' | 'error'; key: string } | null>(null);
    private readonly reloadSequence = signal(0);
    private readonly loadedContext = signal<{ scope: WebhookScope; siteID?: string } | null>(null);
    private readonly requestContext = computed(() => {
        const refresh = this.reloadSequence();
        const scope = this.scope();
        return { scope, siteID: scope === 'site' ? this.activeSite()?.id : undefined, refresh };
    });
    private readonly requestContext$ = toObservable(this.requestContext);

    protected readonly form = this.fb.group({
        name: ['', [Validators.required, Validators.maxLength(120)]],
        description: ['', Validators.maxLength(500)],
        url: ['', [Validators.required]],
        enabled: [true]
    });

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.language();
        return [
            { label: this.transloco.translate('nav.integration'), routerLink: '/integration/webhooks' },
            { label: this.transloco.translate('nav.webhooks'), isCurrent: true }
        ];
    });

    ngOnInit(): void {
        if (!this.canManageSite() && this.canManageInstance()) this.scope.set('instance');
        this.requestContext$
            .pipe(
                distinctUntilChanged((previous, current) => previous.scope === current.scope && previous.siteID === current.siteID && previous.refresh === current.refresh),
                tap(({ scope, siteID }) => {
                    const loaded = this.loadedContext();
                    const contextChanged = !loaded || loaded.scope !== scope || loaded.siteID !== siteID;
                    if (loaded && contextChanged) this.revealedSecret.set('');
                    this.loading.set(true);
                    this.feedback.set(null);
                    this.loadedContext.set(null);
                    if (contextChanged) {
                        this.webhooks.set([]);
                        this.catalog.set([]);
                        this.selectedWebhook.set(null);
                        this.deliveries.set([]);
                        this.deliveryError.set(null);
                    }
                }),
                switchMap(({ scope, siteID }) => {
                    if (scope === 'site' && !siteID) return of({ scope, siteID, webhooks: [], catalog: [], failed: false });
                    return forkJoin({ webhooks: this.service.list(scope, siteID), catalog: this.service.catalog(scope, siteID) }).pipe(
                        map(({ webhooks, catalog }) => ({ scope, siteID, webhooks, catalog, failed: false })),
                        catchError(() => of({ scope, siteID, webhooks: [], catalog: [], failed: true }))
                    );
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe(({ scope, siteID, webhooks, catalog, failed }) => {
                this.loading.set(false);
                this.webhooks.set(webhooks);
                this.catalog.set(catalog);
                if (failed) {
                    this.feedback.set({ severity: 'error', key: 'integration.webhooks.feedback.loadError' });
                    return;
                }
                this.loadedContext.set({ scope, siteID });
            });
    }

    protected switchScope(scope: WebhookScope): void {
        if (scope === this.scope()) return;
        this.scope.set(scope);
        this.revealedSecret.set('');
        this.selectedWebhook.set(null);
        this.deliveries.set([]);
        this.deliveryError.set(null);
    }

    protected load(): void {
        this.reloadSequence.update((value) => value + 1);
    }

    protected openCreate(): void {
        this.editing.set(null);
        this.form.reset({ name: '', description: '', url: '', enabled: true });
        this.selectedEvents.set([]);
        this.dialogVisible.set(true);
    }

    protected openEdit(webhook: Webhook): void {
        if (!this.loadedActionContext()) return;
        this.editing.set(webhook);
        this.form.reset({ name: webhook.name, description: webhook.description, url: webhook.url, enabled: webhook.enabled });
        this.selectedEvents.set([...webhook.events]);
        this.dialogVisible.set(true);
    }

    protected toggleEvent(eventType: string, checked: boolean): void {
        this.selectedEvents.update((events) => (checked ? [...events, eventType] : events.filter((item) => item !== eventType)));
    }

    protected eventSelected(eventType: string): boolean {
        return this.selectedEvents().includes(eventType);
    }

    protected webhookActions(webhook: Webhook): TableRowActionItem[] {
        this.language();
        return [
            {
                label: this.transloco.translate('integration.webhooks.actions.deliveries'),
                icon: 'pi pi-list',
                command: () => this.showDeliveries(webhook)
            },
            {
                label: this.transloco.translate('integration.webhooks.actions.test'),
                icon: 'pi pi-play',
                disabled: !webhook.enabled,
                command: () => this.sendTest(webhook)
            },
            {
                label: this.transloco.translate('integration.webhooks.actions.edit'),
                icon: 'pi pi-pencil',
                command: () => this.openEdit(webhook)
            },
            {
                label: this.transloco.translate('integration.webhooks.actions.rotate'),
                icon: 'pi pi-refresh',
                command: () => this.confirmRotate(webhook)
            },
            { separator: true },
            {
                label: this.transloco.translate('integration.webhooks.actions.delete'),
                icon: 'pi pi-trash',
                danger: true,
                command: () => this.confirmDelete(webhook)
            }
        ];
    }

    protected save(): void {
        this.form.markAllAsTouched();
        if (this.form.invalid || this.selectedEvents().length === 0) return;
        const input: WebhookInput = { ...this.form.getRawValue(), events: this.selectedEvents() };
        const editing = this.editing();
        const context = editing ? this.loadedActionContext() : this.currentContext();
        if (!context) return;
        const request: Observable<Webhook | WebhookSecretResponse> = editing ? this.service.update(editing.id, input, context.scope, context.siteID) : this.service.create(input, context.scope, context.siteID);
        this.saving.set(true);
        request.pipe(finalize(() => this.saving.set(false))).subscribe({
            next: (result) => {
                const webhook = 'webhook' in result ? result.webhook : result;
                if ('secret' in result) this.revealedSecret.set(result.secret);
                this.webhooks.update((items) => (editing ? items.map((item) => (item.id === webhook.id ? webhook : item)) : [webhook, ...items]));
                this.dialogVisible.set(false);
                this.feedback.set({ severity: 'success', key: editing ? 'integration.webhooks.feedback.updated' : 'integration.webhooks.feedback.created' });
            },
            error: () => this.feedback.set({ severity: 'error', key: 'integration.webhooks.feedback.saveError' })
        });
    }

    protected confirmRotate(webhook: Webhook): void {
        this.confirmation.confirm({
            message: this.transloco.translate('integration.webhooks.confirm.rotateMessage', { name: webhook.name }),
            icon: 'pi pi-refresh',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('integration.webhooks.actions.cancel')),
            acceptButtonProps: dialogPrimaryButton(this.transloco.translate('integration.webhooks.actions.rotate')),
            accept: () => this.rotate(webhook)
        });
    }

    private rotate(webhook: Webhook): void {
        const context = this.loadedActionContext();
        if (!context) return;
        this.service.rotate(webhook.id, context.scope, context.siteID).subscribe({
            next: (result) => {
                this.revealedSecret.set(result.secret);
                this.webhooks.update((items) => items.map((item) => (item.id === webhook.id ? result.webhook : item)));
                this.feedback.set({ severity: 'success', key: 'integration.webhooks.feedback.rotated' });
            },
            error: () => this.feedback.set({ severity: 'error', key: 'integration.webhooks.feedback.rotateError' })
        });
    }

    protected sendTest(webhook: Webhook): void {
        const context = this.loadedActionContext();
        if (!context) return;
        this.service.test(webhook.id, context.scope, context.siteID).subscribe({
            next: () => {
                this.feedback.set({ severity: 'success', key: 'integration.webhooks.feedback.testQueued' });
                this.showDeliveries(webhook);
            },
            error: () => this.feedback.set({ severity: 'error', key: 'integration.webhooks.feedback.testError' })
        });
    }

    protected showDeliveries(webhook: Webhook): void {
        const context = this.loadedActionContext();
        if (!context) return;
        this.selectedWebhook.set(webhook);
        this.deliveryError.set(null);
        this.deliveryLoading.set(true);
        this.service
            .deliveries(webhook.id, context.scope, context.siteID)
            .pipe(finalize(() => this.deliveryLoading.set(false)))
            .subscribe({
                next: (items) => this.deliveries.set(items),
                error: () => this.deliveryError.set('integration.webhooks.feedback.deliveryError')
            });
    }

    protected onDeliveryDialogVisibleChange(visible: boolean): void {
        if (visible) return;
        this.selectedWebhook.set(null);
        this.deliveries.set([]);
        this.deliveryError.set(null);
    }

    protected confirmDelete(webhook: Webhook): void {
        this.confirmation.confirm({
            message: this.transloco.translate('integration.webhooks.confirm.deleteMessage', { name: webhook.name }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('integration.webhooks.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('integration.webhooks.actions.delete')),
            accept: () => this.delete(webhook)
        });
    }

    protected statusSeverity(status: string): 'success' | 'warn' | 'danger' | 'secondary' {
        if (status === 'succeeded') return 'success';
        if (status === 'pending' || status === 'retrying' || status === 'processing') return 'warn';
        if (status === 'failed') return 'danger';
        return 'secondary';
    }

    private delete(webhook: Webhook): void {
        const context = this.loadedActionContext();
        if (!context) return;
        this.service.delete(webhook.id, context.scope, context.siteID).subscribe({
            next: () => {
                this.webhooks.update((items) => items.filter((item) => item.id !== webhook.id));
                if (this.selectedWebhook()?.id === webhook.id) {
                    this.selectedWebhook.set(null);
                    this.deliveries.set([]);
                }
                this.feedback.set({ severity: 'success', key: 'integration.webhooks.feedback.deleted' });
            },
            error: () => this.feedback.set({ severity: 'error', key: 'integration.webhooks.feedback.deleteError' })
        });
    }

    private siteID(): string | undefined {
        return this.scope() === 'site' ? this.activeSite()?.id : undefined;
    }

    private currentContext(): { scope: WebhookScope; siteID?: string } {
        return { scope: this.scope(), siteID: this.siteID() };
    }

    private loadedActionContext(): { scope: WebhookScope; siteID?: string } | null {
        const loaded = this.loadedContext();
        const current = this.currentContext();
        if (loaded?.scope === current.scope && loaded.siteID === current.siteID) return current;
        this.load();
        return null;
    }
}
