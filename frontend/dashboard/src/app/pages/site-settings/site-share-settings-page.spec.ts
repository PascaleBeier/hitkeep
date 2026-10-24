import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { Subject } from 'rxjs';
import { AccessService } from '@services/access.service';
import { ShareLink, ShareLinkResponse, ShareService } from '@services/share.service';
import { SiteService } from '@features/sites/services/site.service';
import { SiteShareSettingsPage } from './site-share-settings-page';

describe('SiteShareSettingsPage request scope', () => {
    const site = (id: string) => ({ id, domain: `${id}.example.test`, user_id: 'owner', created_at: '' });
    const link = (siteId: string, id = siteId): ShareLinkResponse => ({ id, site_id: siteId, token: id, token_hint: id, url: `https://example.test/share/${id}`, created_at: '' });
    let activeSite: ReturnType<typeof signal<ReturnType<typeof site>>>;
    let lists: Subject<ShareLink[]>[];
    let creates: Subject<ShareLinkResponse>[];
    let deletes: Subject<void>[];
    let share: { listShareLinks: ReturnType<typeof vi.fn>; createShareLink: ReturnType<typeof vi.fn>; deleteShareLink: ReturnType<typeof vi.fn>; isShareMode: () => boolean };

    beforeEach(() => {
        activeSite = signal(site('a'));
        lists = [];
        creates = [];
        deletes = [];
        share = {
            isShareMode: () => false,
            listShareLinks: vi.fn(() => {
                const request = new Subject<ShareLink[]>();
                lists.push(request);
                return request;
            }),
            createShareLink: vi.fn(() => {
                const request = new Subject<ShareLinkResponse>();
                creates.push(request);
                return request;
            }),
            deleteShareLink: vi.fn(() => {
                const request = new Subject<void>();
                deletes.push(request);
                return request;
            })
        };
        TestBed.configureTestingModule({
            imports: [SiteShareSettingsPage, TranslocoTestingModule.forRoot({ langs: { en: {} }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }, preloadLangs: true })],
            providers: [provideTranslocoLocale({ langToLocaleMapping: { en: 'en-US' } }), { provide: SiteService, useValue: { activeSite } }, { provide: ShareService, useValue: share }, { provide: AccessService, useValue: { canSite: () => true } }]
        });
    });

    function createPage() {
        const fixture = TestBed.createComponent(SiteShareSettingsPage);
        const page = fixture.componentInstance as unknown as {
            shareLinks(): ShareLink[];
            linksLoading(): boolean;
            createLoading(): boolean;
            notice(): { key: string } | null;
            generateShareLink(): void;
            confirmDeleteShareLink(link: ShareLink): void;
        };
        fixture.detectChanges();
        return { fixture, page };
    }

    it('cancels superseded lists through A to B to A and on destruction', async () => {
        const { fixture, page } = createPage();
        activeSite.set(site('b'));
        await fixture.whenStable();
        lists[1].next([link('b')]);
        lists[0].next([link('a')]);
        expect(page.shareLinks().map((row) => row.site_id)).toEqual(['b']);
        expect(lists[0].observed).toBe(false);
        activeSite.set(site('a'));
        await fixture.whenStable();
        lists[2].next([link('a', 'new-a')]);
        lists[0].next([link('a', 'old-a')]);
        expect(page.shareLinks().map((row) => row.id)).toEqual(['new-a']);
        fixture.destroy();
        expect(lists[2].observed).toBe(false);
    });

    it('ignores old creates and their finalizers while a new site request is pending', async () => {
        const { fixture, page } = createPage();
        page.generateShareLink();
        activeSite.set(site('b'));
        await fixture.whenStable();
        lists[1].next([link('b')]);
        page.generateShareLink();
        creates[0].next(link('a', 'secret-a'));
        creates[0].complete();
        expect(page.shareLinks().map((row) => row.site_id)).toEqual(['b']);
        expect(page.createLoading()).toBe(true);
        expect(page.notice()).toBeNull();
        creates[1].next(link('b', 'new-b'));
        creates[1].complete();
        expect(page.shareLinks().map((row) => row.id)).toEqual(['new-b', 'b']);
        fixture.destroy();
    });

    it('rejects a stale delete confirmation and cancels an in-flight delete on scope change', async () => {
        const { fixture, page } = createPage();
        const confirmation = fixture.debugElement.injector.get(ConfirmationService);
        const confirm = vi.spyOn(confirmation, 'confirm').mockReturnValue(confirmation);
        lists[0].next([link('a')]);
        page.confirmDeleteShareLink(link('a'));
        const accept = confirm.mock.calls[0][0].accept!;
        activeSite.set(site('b'));
        await fixture.whenStable();
        accept();
        expect(share.deleteShareLink).not.toHaveBeenCalled();
        lists[1].next([link('b')]);
        page.confirmDeleteShareLink(link('b'));
        confirm.mock.calls[1][0].accept!();
        activeSite.set(site('a'));
        await fixture.whenStable();
        expect(deletes[0].observed).toBe(false);
        deletes[0].error(new Error('late failure'));
        expect(page.notice()).toBeNull();
        fixture.destroy();
    });
});
