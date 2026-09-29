import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { ReportPreview } from '@models/analytics.types';
import { ReportEmailPreview } from './report-email-preview';

describe('ReportEmailPreview', () => {
    let fixture: ComponentFixture<ReportEmailPreview>;

    const preview = (overrides: Partial<ReportPreview> = {}): ReportPreview => ({
        subject: 'Weekly Report for shop.example',
        preset: 'site_summary',
        schedule: { frequency: 'weekly', timezone: 'Europe/Berlin', local_time: '08:00', weekly_day: 1 },
        site_count: 1,
        recipient_count: 2,
        pending_recipient_count: 1,
        period_start: '2026-09-21T22:00:00Z',
        period_end: '2026-09-28T22:00:00Z',
        suppressed: false,
        scheduled_for: '2026-09-29T06:00:00Z',
        audience: 'member',
        from_name: 'HitKeep',
        from_address: 'reports@hitkeep.example',
        preheader: 'shop.example · 48,213 Pageviews',
        html: '<html><head><style>@media (prefers-color-scheme: dark) { body { color: white; } }</style></head><body>Report body</body></html>',
        text: 'Plain report body',
        ...overrides
    });

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [ReportEmailPreview, TranslocoTestingModule.forRoot({ langs: { en: {} }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' } })]
        }).compileComponents();
        fixture = TestBed.createComponent(ReportEmailPreview);
    });

    const query = (selector: string) => fixture.nativeElement.querySelector(selector) as HTMLElement | null;
    const frame = () => query('[data-testid="report-email-frame"]') as HTMLIFrameElement;

    it('shows the envelope and the exact email in a script-less sandbox', async () => {
        fixture.componentRef.setInput('preview', preview());
        await fixture.whenStable();

        expect(query('[data-testid="report-email-subject"]')?.textContent).toContain('Weekly Report for shop.example');
        expect(query('[data-testid="report-email-envelope"]')?.textContent).toContain('HitKeep <reports@hitkeep.example>');
        expect(query('[data-testid="report-email-envelope"]')?.textContent).toContain('shop.example · 48,213 Pageviews');
        expect(frame().getAttribute('sandbox')).toBe('allow-same-origin');
        expect(frame().srcdoc).toContain('Report body');
        expect(frame().srcdoc).toContain('<base target="_blank">');
    });

    it('forces light or dark rendering independently of the viewer theme', async () => {
        fixture.componentRef.setInput('preview', preview());
        await fixture.whenStable();
        expect(frame().srcdoc).toContain('@media not all');

        (fixture.nativeElement.querySelectorAll('[role="group"]')[2].querySelectorAll('button')[1] as HTMLButtonElement).click();
        await fixture.whenStable();
        expect(frame().srcdoc).toContain('@media all');
        expect(frame().srcdoc).not.toContain('prefers-color-scheme');
    });

    it('switches to the plain-text part and to the mobile width', async () => {
        fixture.componentRef.setInput('preview', preview());
        await fixture.whenStable();
        (fixture.nativeElement.querySelectorAll('[role="group"]')[1].querySelectorAll('button')[1] as HTMLButtonElement).click();
        await fixture.whenStable();
        expect(frame().style.width).toBe('375px');

        (fixture.nativeElement.querySelectorAll('[role="group"]')[0].querySelectorAll('button')[1] as HTMLButtonElement).click();
        await fixture.whenStable();
        expect(query('[data-testid="report-email-text"]')?.textContent).toContain('Plain report body');
        expect(query('[data-testid="report-email-frame"]')).toBeNull();
    });

    it('offers the external recipient view only when the report has external recipients', async () => {
        fixture.componentRef.setInput('preview', preview());
        await fixture.whenStable();
        expect(query('[data-testid="report-email-audience-external"]')).toBeNull();

        const emitted: string[] = [];
        fixture.componentInstance.audience.subscribe((value) => emitted.push(value));
        fixture.componentRef.setInput('hasExternalRecipients', true);
        await fixture.whenStable();
        query('[data-testid="report-email-audience-external"]')!.click();
        expect(emitted).toEqual(['external']);
    });

    it('explains suppressed periods instead of showing an empty frame', async () => {
        fixture.componentRef.setInput('preview', preview({ suppressed: true, html: '', text: '', preheader: '' }));
        await fixture.whenStable();
        expect(query('[data-testid="report-email-suppressed"]')).not.toBeNull();
        expect(query('[data-testid="report-email-frame"]')).toBeNull();
    });
});
