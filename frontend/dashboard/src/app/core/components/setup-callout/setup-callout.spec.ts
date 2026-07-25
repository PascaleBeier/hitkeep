import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { SetupCallout } from '@components/setup-callout/setup-callout';

@Component({
    imports: [SetupCallout],
    template: `<app-setup-callout icon="pi pi-cloud-upload" titleKey="page.setup.title" descriptionKey="page.setup.description" docsUrl="https://hitkeep.test/guide/" docsActionKey="page.setup.docsAction" testId="page-setup-callout" />`
})
class SetupCalloutHost {}

describe('SetupCallout', () => {
    let fixture: ComponentFixture<SetupCalloutHost>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                SetupCalloutHost,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            page: {
                                setup: {
                                    title: 'Finish the setup',
                                    description: 'Send data to fill this page.',
                                    docsAction: 'Read the guide'
                                }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(SetupCalloutHost);
        fixture.detectChanges();
    });

    it('renders the translated headline, description and docs link under the supplied test id', () => {
        const callout = fixture.nativeElement.querySelector('[data-testid="page-setup-callout"]');

        expect(callout).not.toBeNull();
        expect(callout.textContent).toContain('Finish the setup');
        expect(callout.textContent).toContain('Send data to fill this page.');

        const link = callout.querySelector('a[href="https://hitkeep.test/guide/"]');
        expect(link).not.toBeNull();
        expect(link.getAttribute('target')).toBe('_blank');
        expect(link.getAttribute('rel')).toBe('noreferrer');
        expect(link.getAttribute('aria-label')).toBe('Read the guide');
        expect(link.textContent).toContain('Read the guide');
    });

    it('renders the caller supplied icon', () => {
        const icon = fixture.nativeElement.querySelector('[data-testid="page-setup-callout"] i');

        expect(icon.className).toContain('pi-cloud-upload');
        expect(icon.getAttribute('aria-hidden')).toBe('true');
    });
});
