import { signal, WritableSignal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { of } from 'rxjs';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { vi } from 'vitest';

import { SITE_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { ShareService } from '@services/share.service';
import { SiteService } from '@features/sites/services/site.service';
import { ShareDashboardLink } from './share-dashboard-link';

interface ShareDashboardLinkTestAccess {
    open(): void;
    generateShareLink(): void;
    showShareDialog(): boolean;
}

describe('ShareDashboardLink', () => {
    let component: ShareDashboardLinkTestAccess;
    let fixture: ComponentFixture<ShareDashboardLink>;
    let allowedSiteCapabilities: WritableSignal<string[] | null>;
    let listShareLinksMock: ReturnType<typeof vi.fn>;
    let createShareLinkMock: ReturnType<typeof vi.fn>;
    let deleteShareLinkMock: ReturnType<typeof vi.fn>;

    const activeSite = {
        id: 'site-1',
        user_id: 'user-1',
        domain: 'example.com',
        created_at: '2026-01-01T00:00:00Z'
    };

    beforeEach(() => {
        allowedSiteCapabilities = signal<string[] | null>(null);
        listShareLinksMock = vi.fn(() =>
            of([
                {
                    id: 'share-1',
                    site_id: 'site-1',
                    token_hint: 'tok...',
                    url: 'https://example.test/share/token',
                    created_at: '2026-01-01T00:00:00Z'
                }
            ])
        );
        createShareLinkMock = vi.fn(() =>
            of({
                id: 'share-1',
                site_id: 'site-1',
                token: 'token',
                token_hint: 'tok...',
                url: 'https://example.test/share/token',
                created_at: '2026-01-01T00:00:00Z'
            })
        );
        deleteShareLinkMock = vi.fn(() => of(void 0));

        TestBed.configureTestingModule({
            imports: [
                ShareDashboardLink,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                actions: {
                                    cancel: 'Cancel',
                                    close: 'Close',
                                    more: 'More actions'
                                },
                                columns: {
                                    actions: 'Actions',
                                    created: 'Created'
                                },
                                copyControl: {
                                    copy: 'Copy',
                                    copied: 'Copied',
                                    failed: 'Copy failed'
                                },
                                searchPlaceholder: 'Search'
                            },
                            share: {
                                dialog: {
                                    title: 'Share dashboard',
                                    description: 'Anyone with this link can view the dashboard.',
                                    generateAction: 'Generate link',
                                    shareUrlLabel: 'Share URL',
                                    tokenHintLabel: 'Token',
                                    urlUnavailable: 'URL hidden',
                                    empty: 'No share links',
                                    generateFailed: 'Unable to create a share link.',
                                    loadFailed: 'Unable to load share links.',
                                    deleteAction: 'Delete',
                                    deleteConfirmTitle: 'Delete share link',
                                    deleteConfirmMessage: 'Anyone using this link will lose access.',
                                    createSuccess: 'Share link created.',
                                    deleteSuccess: 'Share link deleted.',
                                    deleteFailed: 'Unable to delete share link.'
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [
                {
                    provide: AccessService,
                    useValue: {
                        canSite: (_siteId: string, capability: string) => allowedSiteCapabilities()?.includes(capability) ?? true
                    }
                },
                {
                    provide: SiteService,
                    useValue: {
                        activeSite: signal(activeSite)
                    }
                },
                {
                    provide: ShareService,
                    useValue: {
                        isShareMode: () => false,
                        listShareLinks: listShareLinksMock,
                        createShareLink: createShareLinkMock,
                        deleteShareLink: deleteShareLinkMock
                    }
                },
                ConfirmationService,
                provideTranslocoLocale({
                    langToLocaleMapping: {
                        en: 'en-US'
                    }
                })
            ]
        });

        fixture = TestBed.createComponent(ShareDashboardLink);
        component = fixture.componentInstance as unknown as ShareDashboardLinkTestAccess;
        fixture.detectChanges();
    });

    afterEach(() => {
        document.querySelectorAll('.p-dialog-mask, .p-confirm-dialog, .table-row-actions-menu').forEach((element) => element.remove());
    });

    it('does not open or create share links without site team-management capability', () => {
        allowedSiteCapabilities.set([SITE_CAPABILITIES.view]);

        component.open();
        component.generateShareLink();

        expect(component.showShareDialog()).toBe(false);
        expect(listShareLinksMock).not.toHaveBeenCalled();
        expect(createShareLinkMock).not.toHaveBeenCalled();
    });

    it('shows share links in a table-first dialog with shared row actions', async () => {
        component.open();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.body.textContent).toContain('tok...');
        expect((document.body.querySelector('.share-url-input') as HTMLInputElement | null)?.value).toBe('https://example.test/share/token');

        const actionsTrigger = document.body.querySelector('button[aria-label="More actions"]') as HTMLButtonElement | null;
        actionsTrigger?.click();
        fixture.detectChanges();
        await fixture.whenStable();

        const menuText = document.body.textContent ?? '';
        expect(menuText).toContain('Copy');
        expect(menuText).toContain('Delete');
    });

    it('shows create feedback near the share-link table', async () => {
        component.open();
        fixture.detectChanges();
        await fixture.whenStable();

        component.generateShareLink();
        fixture.detectChanges();

        expect(document.body.textContent).toContain('Share link created.');
    });
});
