import { ChangeDetectionStrategy, Component, computed, effect, inject, linkedSignal, untracked } from '@angular/core';
import { Clipboard } from '@angular/cdk/clipboard';
import { toSignal } from '@angular/core/rxjs-interop';

import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { finalize, Subscription } from 'rxjs';
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { ShareLink, ShareService } from '@services/share.service';
import { SiteService } from '@features/sites/services/site.service';
import { AppTable, AppTableCell, AppTableColumn } from '@components/table/table';
import { TableRowActionItem } from '@components/table-row-actions/table-row-actions';

interface ShareNotice {
    kind: 'success' | 'error';
    key: string;
}

@Component({
    selector: 'app-site-share-settings-page',
    imports: [ButtonModule, InputTextModule, ConfirmDialogModule, AppTable, AppTableCell, TranslocoPipe],
    providers: [ConfirmationService],
    templateUrl: './site-share-settings-page.html',
    styleUrl: './site-share-settings-page.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class SiteShareSettingsPage {
    private shareService = inject(ShareService);
    private siteService = inject(SiteService);
    private access = inject(AccessService);
    private confirmation = inject(ConfirmationService);
    private transloco = inject(TranslocoService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });
    private clipboard = inject(Clipboard);

    protected isShareMode = computed(() => this.shareService.isShareMode());
    protected readonly canManageShares = computed(() => {
        const site = this.siteService.activeSite();
        return !this.isShareMode() && !!site && this.access.canSite(site.id, SITE_CAPABILITIES.manageTeam);
    });
    private readonly shareSiteId = computed(() => (this.canManageShares() ? (this.siteService.activeSite()?.id ?? null) : null));
    protected shareLinks = linkedSignal({ source: this.shareSiteId, computation: (): ShareLink[] => [] });
    protected linksLoading = linkedSignal({ source: this.shareSiteId, computation: (siteId) => !!siteId });
    protected createLoading = linkedSignal({ source: this.shareSiteId, computation: () => false });
    protected deletingShareId = linkedSignal({ source: this.shareSiteId, computation: (): string | null => null });
    protected readonly shareLinkColumns: AppTableColumn<ShareLink>[] = [
        { field: 'token_hint', headerKey: 'share.dialog.tokenHintLabel', frozen: true },
        { field: 'created_at', headerKey: 'common.columns.created', type: 'date' },
        { field: 'url', headerKey: 'share.dialog.shareUrlLabel' }
    ];
    protected readonly shareLinkActionLoading = (link: ShareLink) => this.deletingShareId() === link.id;
    protected notice = linkedSignal({ source: this.shareSiteId, computation: (): ShareNotice | null => null });
    private requests: Subscription | null = null;

    constructor() {
        effect((onCleanup) => {
            const siteId = this.shareSiteId();
            const requests = new Subscription();
            this.requests = requests;
            onCleanup(() => {
                requests.unsubscribe();
                this.confirmation.close();
            });
            if (siteId) untracked(() => this.loadShareLinks(siteId, requests));
        });
    }

    protected generateShareLink() {
        const siteId = this.shareSiteId();
        const requests = this.requests;
        if (!siteId || !requests || !this.isCurrentScope(siteId, requests) || this.createLoading()) {
            return;
        }

        this.createLoading.set(true);
        this.notice.set(null);

        requests.add(
            this.shareService
                .createShareLink(siteId)
                .pipe(
                    finalize(() => {
                        if (this.isCurrentScope(siteId, requests)) this.createLoading.set(false);
                    })
                )
                .subscribe({
                    next: (link) => {
                        if (!this.isCurrentScope(siteId, requests)) return;
                        this.shareLinks.update((links) => [link, ...links.filter((existing) => existing.id !== link.id)]);
                        this.notice.set({ kind: 'success', key: 'share.dialog.createSuccess' });
                    },
                    error: () => {
                        if (!this.isCurrentScope(siteId, requests)) return;
                        this.notice.set({ kind: 'error', key: 'share.dialog.generateFailed' });
                    }
                })
        );
    }

    protected confirmDeleteShareLink(link: ShareLink) {
        const siteId = this.shareSiteId();
        const requests = this.requests;
        if (!siteId || !requests || !this.isCurrentScope(siteId, requests) || !this.shareLinks().some((current) => current.id === link.id && current.site_id === siteId) || this.deletingShareId() !== null) {
            return;
        }

        this.confirmation.confirm({
            message: this.transloco.translate('share.dialog.deleteConfirmMessage'),
            header: this.transloco.translate('share.dialog.deleteConfirmTitle'),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('share.dialog.deleteAction')),
            accept: () => this.deleteShareLink(link, siteId, requests)
        });
    }

    protected readonly shareLinkActions = (link: ShareLink): TableRowActionItem[] => {
        this.activeLanguage();
        return [
            {
                label: this.transloco.translate('common.copyControl.copy'),
                icon: 'pi pi-copy',
                disabled: !link.url,
                command: () => this.copyShareLink(link)
            },
            { separator: true },
            {
                label: this.transloco.translate('share.dialog.deleteAction'),
                icon: 'pi pi-trash',
                danger: true,
                disabled: this.deletingShareId() !== null,
                command: () => this.confirmDeleteShareLink(link)
            }
        ];
    };

    private loadShareLinks(siteId: string, requests: Subscription) {
        requests.add(
            this.shareService
                .listShareLinks(siteId)
                .pipe(
                    finalize(() => {
                        if (this.isCurrentScope(siteId, requests)) this.linksLoading.set(false);
                    })
                )
                .subscribe({
                    next: (links) => {
                        if (!this.isCurrentScope(siteId, requests)) return;
                        this.shareLinks.set(this.mergeKnownURLs(links));
                    },
                    error: () => {
                        if (!this.isCurrentScope(siteId, requests)) return;
                        this.notice.set({ kind: 'error', key: 'share.dialog.loadFailed' });
                    }
                })
        );
    }

    private deleteShareLink(link: ShareLink, siteId: string, requests: Subscription) {
        if (!this.isCurrentScope(siteId, requests) || !this.shareLinks().some((current) => current.id === link.id && current.site_id === siteId)) {
            return;
        }

        this.deletingShareId.set(link.id);
        requests.add(
            this.shareService
                .deleteShareLink(siteId, link.id)
                .pipe(
                    finalize(() => {
                        if (this.isCurrentScope(siteId, requests)) this.deletingShareId.set(null);
                    })
                )
                .subscribe({
                    next: () => {
                        if (!this.isCurrentScope(siteId, requests)) return;
                        this.shareLinks.update((links) => links.filter((existing) => existing.id !== link.id));
                        this.notice.set({ kind: 'success', key: 'share.dialog.deleteSuccess' });
                    },
                    error: () => {
                        if (!this.isCurrentScope(siteId, requests)) return;
                        this.notice.set({ kind: 'error', key: 'share.dialog.deleteFailed' });
                    }
                })
        );
    }

    private copyShareLink(link: ShareLink) {
        const current = this.shareLinks().find((row) => row.id === link.id && row.site_id === this.shareSiteId());
        if (!current?.url) {
            return;
        }
        const copied = this.clipboard.copy(current.url);
        this.notice.set({ kind: copied ? 'success' : 'error', key: copied ? 'common.copyControl.copied' : 'common.copyControl.failed' });
    }

    private mergeKnownURLs(links: ShareLink[]): ShareLink[] {
        const knownByID = new Map<string, string>();
        for (const link of this.shareLinks()) {
            if (!link.url) {
                continue;
            }
            knownByID.set(link.id, link.url);
        }

        return links.map((link) => ({
            ...link,
            url: knownByID.get(link.id) ?? link.url
        }));
    }

    private isCurrentScope(siteId: string, requests: Subscription): boolean {
        return requests === this.requests && !requests.closed && siteId === this.shareSiteId();
    }
}
