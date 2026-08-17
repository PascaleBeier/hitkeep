import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';
import { of } from 'rxjs';

import { SiteGeneralSettings } from './site-general-settings';
import { AccessService } from '@services/access.service';
import { SiteService } from '@features/sites/services/site.service';

describe('SiteGeneralSettings', () => {
    let fixture: ComponentFixture<SiteGeneralSettings>;
    let canSite: boolean;
    let renameSiteDomain: ReturnType<typeof vi.fn>;

    beforeEach(async () => {
        canSite = false;
        renameSiteDomain = vi.fn(() => of({ id: 'site-1', domain: 'renamed.example.com' }));

        await TestBed.configureTestingModule({
            imports: [
                SiteGeneralSettings,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: { actions: { copy: 'Copy' } },
                            sites: {
                                settings: {
                                    general: {
                                        title: 'General settings',
                                        domainLabel: 'Domain',
                                        renameAction: 'Save domain',
                                        renameHint: 'Renaming changes which domain the tracker matches. Existing analytics stay with this site.',
                                        siteIdLabel: 'Site ID',
                                        exportShortcutTitle: 'Analytics export',
                                        exportShortcutDescription: 'Export this site, or any other accessible site, from the Import & Export hub.',
                                        exportShortcutAction: 'Open Import & Export'
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
            providers: [provideRouter([]), { provide: AccessService, useValue: { canSite: () => canSite } }, { provide: SiteService, useValue: { renameSiteDomain } }]
        }).compileComponents();

        fixture = TestBed.createComponent(SiteGeneralSettings);
        fixture.componentRef.setInput('site', {
            id: 'site-1',
            domain: 'example.com',
            created_at: '2026-05-05T00:00:00Z'
        });
        fixture.detectChanges();
    });

    it('replaces the site takeout split button with a shortcut to the Import & Export hub', () => {
        const text = fixture.nativeElement.textContent;
        const links = Array.from(fixture.nativeElement.querySelectorAll('a')) as HTMLAnchorElement[];

        expect(text).toContain('Analytics export');
        expect(text).toContain('Export this site, or any other accessible site, from the Import & Export hub.');
        expect(links.some((link) => link.getAttribute('href') === '/import-export/export')).toBe(true);
        expect(fixture.nativeElement.querySelector('p-splitbutton')).toBeNull();
    });

    it('shows the domain read-only without data-management access', () => {
        expect(fixture.nativeElement.querySelector('#siteDomain')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('example.com');
    });

    it('lets site admins rename the domain', () => {
        canSite = true;
        fixture = TestBed.createComponent(SiteGeneralSettings);
        fixture.componentRef.setInput('site', {
            id: 'site-1',
            domain: 'example.com',
            created_at: '2026-05-05T00:00:00Z'
        });
        fixture.detectChanges();

        const input = fixture.nativeElement.querySelector('#siteDomain') as HTMLInputElement;
        expect(input).not.toBeNull();
        expect(input.value).toBe('example.com');

        input.value = 'renamed.example.com';
        input.dispatchEvent(new Event('input'));
        fixture.detectChanges();

        const saveButton = fixture.nativeElement.querySelector('p-button button') as HTMLButtonElement;
        expect(saveButton.disabled).toBe(false);
        saveButton.click();

        expect(renameSiteDomain).toHaveBeenCalledWith('site-1', 'renamed.example.com');
    });

    it('accepts hyphenated multi-level domains when renaming', async () => {
        canSite = true;
        fixture = TestBed.createComponent(SiteGeneralSettings);
        fixture.componentRef.setInput('site', {
            id: 'site-1',
            domain: 'example.com',
            created_at: '2026-05-05T00:00:00Z'
        });
        await fixture.whenStable();

        const input = fixture.nativeElement.querySelector('#siteDomain') as HTMLInputElement;
        input.value = 'sub.example-app.com.br';
        input.dispatchEvent(new Event('input'));
        await fixture.whenStable();

        const saveButton = fixture.nativeElement.querySelector('p-button button') as HTMLButtonElement;
        expect(saveButton.disabled).toBe(false);
        saveButton.click();
        await fixture.whenStable();

        expect(renameSiteDomain).toHaveBeenCalledWith('site-1', 'sub.example-app.com.br');
    });
});
