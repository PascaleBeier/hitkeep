import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { SiteService } from '@features/sites/services/site.service';
import { AccessService } from '@services/access.service';
import { WebhooksService } from '@services/webhooks.service';
import { WebhooksPage } from './webhooks';

describe('WebhooksPage', () => {
    let fixture: ComponentFixture<WebhooksPage>;
    const activeSite = signal({ id: 'site-1', domain: 'example.com' });
    const service = {
        catalog: vi.fn((scope: string, siteID?: string) => {
            void scope;
            void siteID;
            return of([{ type: 'goal.created', site_scoped: true, scopes: ['site'] }]);
        }),
        list: vi.fn((scope: string, siteID?: string) => {
            void scope;
            void siteID;
            return of([]);
        }),
        create: vi.fn(),
        update: vi.fn(),
        rotate: vi.fn(),
        test: vi.fn(),
        deliveries: vi.fn(() => of([])),
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
                                    actions: { create: 'Create webhook', refresh: 'Refresh' },
                                    secret: { title: 'Save this signing secret now', message: 'It is shown once.', copy: 'Copy secret' },
                                    empty: { title: 'No webhooks yet', message: 'Create a webhook to start delivering operational events.' }
                                }
                            },
                            common: { copyControl: { copy: 'Copy', copied: 'Copied', failed: 'Copy failed', ariaLabel: 'Copy to clipboard' } }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }
                })
            ],
            providers: [
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

    it('presents a one-time signing secret with the shared API credential pattern', () => {
        fixture.componentInstance['revealedSecret'].set('whsec_test_value');
        fixture.detectChanges();

        const notice = fixture.nativeElement.querySelector('.hk-feedback-message--token') as HTMLElement | null;
        expect(notice).not.toBeNull();
        expect(notice?.getAttribute('role')).toBe('status');
        expect(notice?.getAttribute('aria-live')).toBe('polite');
        expect(notice?.querySelector('.one-time-credential__value')?.textContent).toContain('whsec_test_value');
        expect(notice?.querySelector('.p-button-text')).not.toBeNull();
    });
});
