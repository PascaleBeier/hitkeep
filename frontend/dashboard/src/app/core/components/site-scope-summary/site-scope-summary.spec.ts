import { TestBed } from '@angular/core/testing';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';

import { SiteScopeSummary } from './site-scope-summary';

describe('SiteScopeSummary', () => {
    afterEach(() => {
        document.querySelectorAll('.p-popover').forEach((element) => element.remove());
    });

    it('renders one site as a compact globe tag', () => {
        const fixture = TestBed.configureTestingModule({ imports: [SiteScopeSummary, NoopAnimationsModule] }).createComponent(SiteScopeSummary);
        fixture.componentRef.setInput('items', [{ id: 'site-1', label: 'shop.example.com', detail: 'Viewer', severity: 'secondary' }]);
        fixture.componentRef.setInput('countLabel', '1 site');
        fixture.componentRef.setInput('popoverTitle', 'Sites');
        fixture.detectChanges();

        const tag = fixture.nativeElement.querySelector('.hk-site-scope-tag') as HTMLElement | null;
        expect(tag?.textContent).toContain('shop.example.com · Viewer');
        expect(tag?.querySelector('.pi-globe')).not.toBeNull();
    });

    it('summarizes multiple sites and exposes their full scope in a popover', async () => {
        const fixture = TestBed.configureTestingModule({ imports: [SiteScopeSummary, NoopAnimationsModule] }).createComponent(SiteScopeSummary);
        fixture.componentRef.setInput('items', [
            { id: 'site-1', label: 'shop.example.com', detail: 'Viewer', severity: 'secondary' },
            { id: 'site-2', label: 'docs.example.com', detail: 'Admin', severity: 'info' }
        ]);
        fixture.componentRef.setInput('countLabel', '2 sites');
        fixture.componentRef.setInput('popoverTitle', 'Sites');
        fixture.detectChanges();

        const trigger = fixture.nativeElement.querySelector('.hk-site-scope-count-button') as HTMLButtonElement;
        expect(trigger.textContent).toContain('2 sites');
        trigger.click();
        fixture.detectChanges();
        await fixture.whenStable();

        expect(document.body.textContent).toContain('shop.example.com');
        expect(document.body.textContent).toContain('docs.example.com');
        expect(document.body.textContent).toContain('Admin');
    });

    it('renders a supplied empty-scope label', () => {
        const fixture = TestBed.configureTestingModule({ imports: [SiteScopeSummary, NoopAnimationsModule] }).createComponent(SiteScopeSummary);
        fixture.componentRef.setInput('items', []);
        fixture.componentRef.setInput('countLabel', '0 sites');
        fixture.componentRef.setInput('popoverTitle', 'Sites');
        fixture.componentRef.setInput('emptyLabel', 'No site access');
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('No site access');
        expect(fixture.nativeElement.querySelector('.pi-ban')).not.toBeNull();
    });
});
