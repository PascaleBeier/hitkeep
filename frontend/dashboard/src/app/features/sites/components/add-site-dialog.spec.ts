import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { SiteService } from '@features/sites/services/site.service';
import { AddSiteDialog } from './add-site-dialog';

describe('AddSiteDialog', () => {
    let fixture: ComponentFixture<AddSiteDialog>;
    let createSite: ReturnType<typeof vi.fn>;

    beforeEach(async () => {
        createSite = vi.fn(() =>
            of({
                id: 'site-1',
                user_id: 'user-1',
                domain: 'sub.example-app.com.br',
                created_at: '2026-05-05T00:00:00Z'
            })
        );

        await TestBed.configureTestingModule({
            imports: [
                AddSiteDialog,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: { actions: { cancel: 'Cancel' } },
                            sites: {
                                addDialog: {
                                    title: 'Add site',
                                    addAction: 'Add site',
                                    instructionsLine1: 'Enter a domain such as {{apex}} or {{subdomain}}.',
                                    instructionsLine2: 'We automatically track {{www}}.',
                                    domainLabel: 'Domain',
                                    domainPlaceholder: 'example.com',
                                    errors: {
                                        domainRequired: 'Domain is required.',
                                        domainInvalid: 'Invalid domain format.',
                                        removeProtocol: 'Remove the protocol.',
                                        removeWww: 'Enter the domain without the www prefix.',
                                        createFailed: 'Failed to create site.'
                                    }
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
            providers: [{ provide: SiteService, useValue: { createSite } }]
        }).compileComponents();

        fixture = TestBed.createComponent(AddSiteDialog);
        fixture.componentRef.setInput('visible', true);
        await fixture.whenStable();
    });

    afterEach(() => {
        fixture.destroy();
        document.querySelectorAll('.p-dialog-mask, .p-dialog').forEach((element) => element.remove());
    });

    it('submits an issue-style hyphenated multi-level domain', async () => {
        const input = document.body.querySelector('#domain') as HTMLInputElement;
        input.value = 'sub.example-app.com.br';
        input.dispatchEvent(new Event('input'));
        input.dispatchEvent(new Event('blur'));
        await fixture.whenStable();

        const buttons = Array.from(document.body.querySelectorAll('.dialog-shell-footer button')) as HTMLButtonElement[];
        const addButton = buttons.at(-1);
        expect(addButton?.disabled).toBe(false);

        addButton?.click();
        await fixture.whenStable();

        expect(createSite).toHaveBeenCalledWith('sub.example-app.com.br');
    });
});
