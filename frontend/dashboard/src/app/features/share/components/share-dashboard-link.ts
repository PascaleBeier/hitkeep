import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';
import { Clipboard } from '@angular/cdk/clipboard';
import { toSignal } from '@angular/core/rxjs-interop';

import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { finalize } from 'rxjs';
import { ButtonModule } from '@openng/optimus-ui/button';
import { IconFieldModule } from '@openng/optimus-ui/iconfield';
import { InputIconModule } from '@openng/optimus-ui/inputicon';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { TableModule } from '@openng/optimus-ui/table';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { ShareLink, ShareService } from '@services/share.service';
import { SiteService } from '@features/sites/services/site.service';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';

interface ShareNotice {
    kind: 'success' | 'error';
    key: string;
}

@Component({
    selector: 'app-share-dashboard-link',
    imports: [ButtonModule, DialogShell, IconFieldModule, InputIconModule, InputTextModule, TableModule, ConfirmDialogModule, RelativeDateTime, TableRowActions, TranslocoPipe],
    providers: [ConfirmationService],
    templateUrl: './share-dashboard-link.html',
    styleUrl: './share-dashboard-link.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ShareDashboardLink {
    private shareService = inject(ShareService);
    protected siteService = inject(SiteService);
    private access = inject(AccessService);
    private confirmation = inject(ConfirmationService);
    private transloco = inject(TranslocoService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });
    private clipboard = inject(Clipboard);

    protected isShareMode = computed(() => this.shareService.isShareMode());
    protected showShareDialog = signal(false);
    protected shareLinks = signal<ShareLink[]>([]);
    protected linksLoading = signal(false);
    protected createLoading = signal(false);
    protected deletingShareId = signal<string | null>(null);
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

            if (this.showShareDialog() && siteId && this.canManageShares()) {
                this.loadShareLinks(siteId);
            }
        });
    }

    protected openShareDialog() {
        if (!this.canManageShares()) {
            return;
        }

        this.notice.set(null);
        this.showShareDialog.set(true);

        const siteId = this.siteService.activeSite()?.id;
        if (siteId) {
            this.loadShareLinks(siteId);
        }
    }

    open() {
        if (this.isShareMode() || !this.canManageShares()) return;
        this.openShareDialog();
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

    protected shareLinkActions(link: ShareLink): TableRowActionItem[] {
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
    }

    protected resetShareDialog() {
        this.showShareDialog.set(false);
        this.notice.set(null);
        this.deletingShareId.set(null);
    }

    protected onShareDialogVisibleChange(visible: boolean) {
        this.showShareDialog.set(visible);
        if (!visible) {
            this.resetShareDialog();
        }
    }

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
