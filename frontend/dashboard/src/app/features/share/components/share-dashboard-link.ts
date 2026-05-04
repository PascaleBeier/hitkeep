import { ChangeDetectionStrategy, Component, computed, effect, inject, signal } from '@angular/core';

import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';
import { DialogModule } from 'primeng/dialog';
import { InputTextModule } from 'primeng/inputtext';
import { TableModule } from 'primeng/table';
import { ConfirmDialogModule } from 'primeng/confirmdialog';
import { ConfirmationService } from 'primeng/api';
import { ShareLink, ShareService } from '@services/share.service';
import { SiteService } from '@features/sites/services/site.service';
import { CopyControl } from '@components/copy-control/copy-control';
import { RelativeDateTime } from '@components/relative-date-time/relative-date-time';

interface ShareNotice {
    kind: 'success' | 'error';
    key: string;
}

@Component({
    selector: 'app-share-dashboard-link',
    imports: [ButtonModule, DialogModule, InputTextModule, TableModule, ConfirmDialogModule, CopyControl, RelativeDateTime, TranslocoPipe],
    providers: [ConfirmationService],
    templateUrl: './share-dashboard-link.html',
    styleUrl: './share-dashboard-link.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ShareDashboardLink {
    private shareService = inject(ShareService);
    protected siteService = inject(SiteService);
    private confirmation = inject(ConfirmationService);
    private transloco = inject(TranslocoService);

    protected isShareMode = computed(() => this.shareService.isShareMode());
    protected showShareDialog = signal(false);
    protected shareLinks = signal<ShareLink[]>([]);
    protected linksLoading = signal(false);
    protected createLoading = signal(false);
    protected deletingShareId = signal<string | null>(null);
    protected notice = signal<ShareNotice | null>(null);
    private shareSiteId = signal<string | null>(null);

    constructor() {
        effect(() => {
            const siteId = this.siteService.activeSite()?.id ?? null;
            if (this.shareSiteId() === siteId) {
                return;
            }

            this.shareSiteId.set(siteId);
            this.resetShareState();

            if (this.showShareDialog() && siteId) {
                this.loadShareLinks(siteId);
            }
        });
    }

    protected openShareDialog() {
        this.notice.set(null);
        this.showShareDialog.set(true);

        const siteId = this.siteService.activeSite()?.id;
        if (siteId) {
            this.loadShareLinks(siteId);
        }
    }

    open() {
        if (this.isShareMode()) return;
        this.openShareDialog();
    }

    protected generateShareLink() {
        const siteId = this.siteService.activeSite()?.id;
        if (!siteId || this.createLoading()) {
            return;
        }

        this.createLoading.set(true);
        this.notice.set(null);

        this.shareService.createShareLink(siteId).subscribe({
            next: (link) => {
                this.shareLinks.update((links) => [link, ...links.filter((existing) => existing.id !== link.id)]);
                this.createLoading.set(false);
                this.notice.set({ kind: 'success', key: 'share.dialog.createSuccess' });
            },
            error: () => {
                this.createLoading.set(false);
                this.notice.set({ kind: 'error', key: 'share.dialog.generateFailed' });
            }
        });
    }

    protected confirmDeleteShareLink(link: ShareLink) {
        if (this.deletingShareId() !== null) {
            return;
        }

        this.confirmation.confirm({
            message: this.transloco.translate('share.dialog.deleteConfirmMessage'),
            header: this.transloco.translate('share.dialog.deleteConfirmTitle'),
            icon: 'pi pi-exclamation-triangle',
            acceptLabel: this.transloco.translate('share.dialog.deleteAction'),
            rejectLabel: this.transloco.translate('common.actions.cancel'),
            acceptButtonStyleClass: 'p-button-danger',
            accept: () => this.deleteShareLink(link)
        });
    }

    protected resetShareDialog() {
        this.showShareDialog.set(false);
        this.notice.set(null);
        this.deletingShareId.set(null);
    }

    private loadShareLinks(siteId: string) {
        this.linksLoading.set(true);
        this.notice.set(null);

        this.shareService.listShareLinks(siteId).subscribe({
            next: (links) => {
                this.shareLinks.set(this.mergeKnownURLs(links));
                this.linksLoading.set(false);
            },
            error: () => {
                this.linksLoading.set(false);
                this.notice.set({ kind: 'error', key: 'share.dialog.loadFailed' });
            }
        });
    }

    private deleteShareLink(link: ShareLink) {
        const siteId = this.siteService.activeSite()?.id;
        if (!siteId) {
            return;
        }

        this.deletingShareId.set(link.id);
        this.shareService.deleteShareLink(siteId, link.id).subscribe({
            next: () => {
                this.shareLinks.update((links) => links.filter((existing) => existing.id !== link.id));
                this.deletingShareId.set(null);
                this.notice.set({ kind: 'success', key: 'share.dialog.deleteSuccess' });
            },
            error: () => {
                this.deletingShareId.set(null);
                this.notice.set({ kind: 'error', key: 'share.dialog.deleteFailed' });
            }
        });
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
            url: knownByID.get(link.id)
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
