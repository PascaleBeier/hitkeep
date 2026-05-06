import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { SiteGeneralSettings } from './site-general-settings';

describe('SiteGeneralSettings', () => {
    let fixture: ComponentFixture<SiteGeneralSettings>;

    beforeEach(async () => {
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
            providers: [provideRouter([])]
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
});
