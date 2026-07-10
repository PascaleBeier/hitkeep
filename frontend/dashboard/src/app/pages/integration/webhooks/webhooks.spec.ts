import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { Observable, of, Subject, throwError } from 'rxjs';
import { vi } from 'vitest';

import { SiteService } from '@features/sites/services/site.service';
import { AccessService } from '@services/access.service';
import { Webhook, WebhookDelivery, WebhooksService } from '@services/webhooks.service';
import { WebhooksPage } from './webhooks';

describe('WebhooksPage', () => {
    let fixture: ComponentFixture<WebhooksPage>;
    const activeSite = signal({ id: 'site-1', domain: 'example.com' });
    const webhook: Webhook = {
        id: 'webhook-1',
        site_id: 'site-1',
        scope: 'site' as const,
        name: 'Order processor',
        description: 'Sends completed orders',
        url: 'https://example.com/hooks/orders',
        enabled: true,
        events: ['goal.created'],
        created_at: '2026-07-09T12:00:00Z',
        updated_at: '2026-07-10T12:00:00Z'
    };
    const service = {
        catalog: vi.fn((scope: string, siteID?: string) => {
            void scope;
            void siteID;
            return of([{ type: 'goal.created', site_scoped: true, scopes: ['site'] }]);
        }),
        list: vi.fn((scope: string, siteID?: string): Observable<Webhook[]> => {
            void scope;
            void siteID;
            return of([]);
        }),
        create: vi.fn(),
        update: vi.fn(),
        rotate: vi.fn(),
        test: vi.fn(),
        deliveries: vi.fn((): Observable<WebhookDelivery[]> => of([])),
        delete: vi.fn()
    };

    beforeEach(async () => {
        vi.clearAllMocks();
        activeSite.set({ id: 'site-1', domain: 'example.com' });
        await TestBed.configureTestingModule({
            imports: [
                WebhooksPage,
                NoopAnimationsModule,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: { integration: 'Integration', webhooks: 'Webhooks' },
                            integration: {
                                webhooks: {
                                    subtitle: 'Send signed operational events to your systems.',
                                    scopes: { site: 'Site', instance: 'Instance' },
                                    actions: { create: 'Create webhook', refresh: 'Refresh', deliveries: 'Deliveries', test: 'Send test', edit: 'Edit', rotate: 'Rotate secret', delete: 'Delete' },
                                    secret: { title: 'Save this signing secret now', message: 'It is shown once.', copy: 'Copy secret' },
                                    empty: { title: 'No webhooks yet', message: 'Create a webhook to start delivering operational events.' },
                                    status: { enabled: 'Enabled', disabled: 'Disabled' },
                                    form: { url: 'Destination URL', events: 'Events' },
                                    deliveries: {
                                        title: 'Deliveries for {{name}}',
                                        subtitle: 'Automatic attempts are at-least-once.',
                                        event: 'Event',
                                        status: 'Status',
                                        attempts: 'Attempts',
                                        response: 'Response',
                                        created: 'Created',
                                        empty: 'No deliveries recorded yet.'
                                    },
                                    feedback: { deliveryError: 'Delivery history could not be loaded.' }
                                }
                            },
                            common: {
                                searchPlaceholder: 'Search...',
                                loading: 'Loading...',
                                columns: { name: 'Name', status: 'Status', updated: 'Updated', actions: 'Actions' },
                                actions: { more: 'More actions', close: 'Close' },
                                copyControl: { copy: 'Copy', copied: 'Copied', failed: 'Copy failed', ariaLabel: 'Copy to clipboard' }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }
                })
            ],
            providers: [
                provideTranslocoLocale({ langToLocaleMapping: { en: 'en-US' } }),
                { provide: WebhooksService, useValue: service },
                { provide: SiteService, useValue: { activeSite } },
                {
                    provide: AccessService,
                    useValue: { hasInstance: () => false, canActiveSite: () => true }
                }
            ]
        }).compileComponents();
        fixture = TestBed.createComponent(WebhooksPage);
        await fixture.whenStable();
    });

    it('loads site webhook settings and renders a teaching empty state', () => {
        expect(service.catalog).toHaveBeenCalledWith('site', 'site-1');
        expect(service.list).toHaveBeenCalledWith('site', 'site-1');
        expect(fixture.nativeElement.textContent).toContain('No webhooks yet');
        expect(fixture.nativeElement.querySelector('[data-testid="create-webhook"]')).toBeTruthy();
    });

    it('reloads and clears site-scoped state when the active site changes', async () => {
        fixture.componentInstance['revealedSecret'].set('whsec_old_site');
        activeSite.set({ id: 'site-2', domain: 'second.example.com' });
        await fixture.whenStable();

        expect(service.catalog).toHaveBeenCalledWith('site', 'site-2');
        expect(service.list).toHaveBeenCalledWith('site', 'site-2');
        expect(service.list).toHaveBeenCalledTimes(2);
        expect(fixture.componentInstance['revealedSecret']()).toBe('');
    });

    it('presents a one-time signing secret with the shared API credential pattern', async () => {
        fixture.componentInstance['revealedSecret'].set('whsec_test_value');
        await fixture.whenStable();

        const notice = fixture.nativeElement.querySelector('.hk-feedback-message--token') as HTMLElement | null;
        expect(notice).not.toBeNull();
        expect(notice?.getAttribute('role')).toBe('status');
        expect(notice?.getAttribute('aria-live')).toBe('polite');
        expect(notice?.querySelector('.one-time-credential__value')?.textContent).toContain('whsec_test_value');
        expect(notice?.querySelector('.p-button-text')).not.toBeNull();
    });

    it('renders configured webhooks in the shared searchable and sortable CRUD table', async () => {
        fixture.componentInstance['webhooks'].set([webhook]);
        await fixture.whenStable();

        const table = fixture.nativeElement.querySelector('.hk-crud-table') as HTMLElement | null;
        const row = table?.querySelector('tbody tr') as HTMLElement | null;

        expect(table).not.toBeNull();
        expect(table?.querySelector('input[placeholder="Search..."]')).not.toBeNull();
        expect(table?.querySelectorAll('th.p-datatable-sortable-column').length).toBe(4);
        expect(row?.textContent).toContain('Order processor');
        expect(row?.textContent).toContain('https://example.com/hooks/orders');
        expect(row?.querySelector('app-table-row-actions')).not.toBeNull();
        expect(row?.querySelector('button[aria-label="More actions"]')).not.toBeNull();
    });

    it('keeps the current table visible while refreshing like the API client list', async () => {
        const pending = new Subject<Webhook[]>();
        service.list.mockReturnValueOnce(pending.asObservable());
        fixture.componentInstance['webhooks'].set([webhook]);

        fixture.componentInstance['load']();
        await fixture.whenStable();

        expect(fixture.componentInstance['loading']()).toBe(true);
        expect(fixture.nativeElement.querySelector('tbody')?.textContent).toContain('Order processor');
        pending.next([webhook]);
        pending.complete();
    });

    it('exposes row actions through the shared action-menu model', () => {
        const actions = fixture.componentInstance['webhookActions'](webhook);

        expect(actions.filter((action) => !action['separator']).map((action) => action['label'])).toEqual(['Deliveries', 'Send test', 'Edit', 'Rotate secret', 'Delete']);
        expect(actions.at(-1)?.danger).toBe(true);
        expect(fixture.componentInstance['webhookActions']({ ...webhook, enabled: false })[1]['disabled']).toBe(true);
    });

    it('opens delivery history in a dialog from a row action', async () => {
        fixture.componentInstance['showDeliveries'](webhook);
        await fixture.whenStable();

        const dialog = document.body.querySelector('.p-dialog') as HTMLElement | null;
        const labelledBy = dialog?.getAttribute('aria-labelledby');
        expect(dialog).not.toBeNull();
        expect(labelledBy).toBeTruthy();
        expect(document.getElementById(labelledBy ?? '')?.textContent).toContain('Deliveries for Order processor');
        expect(dialog?.textContent).toContain('Deliveries for Order processor');
        expect(dialog?.textContent).toContain('No deliveries recorded yet.');
    });

    it('keeps delivery-load errors inside the delivery dialog', async () => {
        service.deliveries.mockReturnValueOnce(throwError(() => new Error('failed')));

        fixture.componentInstance['showDeliveries'](webhook);
        await fixture.whenStable();

        expect(document.body.querySelector('.p-dialog')?.textContent).toContain('Delivery history could not be loaded.');
    });
});
