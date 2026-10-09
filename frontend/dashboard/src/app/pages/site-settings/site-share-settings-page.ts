import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import { Clipboard } from '@angular/cdk/clipboard';
import { toSignal } from '@angular/core/rxjs-interop';

import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { finalize } from 'rxjs';
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
    protected shareLinks = signal<ShareLink[]>([]);
    protected linksLoading = signal(false);
    protected createLoading = signal(false);
    protected deletingShareId = signal<string | null>(null);
    protected readonly shareLinkColumns: AppTableColumn<ShareLink>[] = [
        { field: 'token_hint', headerKey: 'share.dialog.tokenHintLabel', frozen: true },
        { field: 'created_at', headerKey: 'common.columns.created', type: 'date' },
        { field: 'url', headerKey: 'share.dialog.shareUrlLabel' }
    ];
    protected readonly shareLinkActionLoading = (link: ShareLink) => this.deletingShareId() === link.id;
    protected notice = signal<ShareNotice | null>(null);
    private shareSiteId = signal<string | null>(null);
    protected readonly canManageShares = computed(() => {
        const site = this.siteService.activeSite();
        return !!site && this.access.canSite(site.id, SITE_CAPABILITIES.manageTeam);
    });

    constructor() {
        effect(() => {
            const siteId = this.siteService.activeSite()?.id ?? null;
            if (this.shareSiteId() === siteId) {
                return;
            }

            this.shareSiteId.set(siteId);
            this.resetShareState();

            if (siteId && this.canManageShares()) {
                this.loadShareLinks(siteId);
            }
        });
    }

    protected generateShareLink() {
        const siteId = this.siteService.activeSite()?.id;
        if (!siteId || this.createLoading() || !this.canManageShares()) {
            return;
        }

        this.createLoading.set(true);
        this.notice.set(null);

        this.shareService
            .createShareLink(siteId)
            .pipe(finalize(() => this.createLoading.set(false)))
            .subscribe({
                next: (link) => {
                    this.shareLinks.update((links) => [link, ...links.filter((existing) => existing.id !== link.id)]);
                    this.notice.set({ kind: 'success', key: 'share.dialog.createSuccess' });
                },
                error: () => {
                    this.notice.set({ kind: 'error', key: 'share.dialog.generateFailed' });
                }
            });
    }

    protected confirmDeleteShareLink(link: ShareLink) {
        if (this.deletingShareId() !== null || !this.canManageShares()) {
            return;
        }

        this.confirmation.confirm({
            message: this.transloco.translate('share.dialog.deleteConfirmMessage'),
            header: this.transloco.translate('share.dialog.deleteConfirmTitle'),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('share.dialog.deleteAction')),
            accept: () => this.deleteShareLink(link)
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

    private loadShareLinks(siteId: string) {
        if (!this.canManageShares()) {
            return;
        }

        this.linksLoading.set(true);
        this.notice.set(null);

        this.shareService
            .listShareLinks(siteId)
            .pipe(finalize(() => this.linksLoading.set(false)))
            .subscribe({
                next: (links) => {
                    this.shareLinks.set(this.mergeKnownURLs(links));
                },
                error: () => {
                    this.notice.set({ kind: 'error', key: 'share.dialog.loadFailed' });
                }
            });
    }

    private deleteShareLink(link: ShareLink) {
        const siteId = this.siteService.activeSite()?.id;
        if (!siteId || !this.canManageShares()) {
            return;
        }

        this.deletingShareId.set(link.id);
        this.shareService
            .deleteShareLink(siteId, link.id)
            .pipe(finalize(() => this.deletingShareId.set(null)))
            .subscribe({
                next: () => {
                    this.shareLinks.update((links) => links.filter((existing) => existing.id !== link.id));
                    this.notice.set({ kind: 'success', key: 'share.dialog.deleteSuccess' });
                },
                error: () => {
                    this.notice.set({ kind: 'error', key: 'share.dialog.deleteFailed' });
                }
            });
    }

    private copyShareLink(link: ShareLink) {
        if (!link.url) {
            return;
        }
        const copied = this.clipboard.copy(link.url);
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

    private resetShareState() {
        this.shareLinks.set([]);
        this.linksLoading.set(false);
        this.createLoading.set(false);
        this.deletingShareId.set(null);
        this.notice.set(null);
    }
}
