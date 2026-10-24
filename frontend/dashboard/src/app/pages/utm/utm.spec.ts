import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter, Router, Route } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { provideHttpClient } from '@angular/common/http';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { userEvent } from 'vitest/browser';
import { routes } from '../../app.routes';
import { ShareService } from '@services/share.service';

@Component({ selector: 'app-utm-route-probe', template: '' })
class UtmRouteProbe {}

async function configure(shared = false) {
    const main = routes.find((route) => route.path === '')!.children!;
    const branch = shared ? main.find((route) => route.path === 'share/:token')!.children! : main;
    const utm = branch.find((route) => route.path === 'utm')!;
    const route: Route = { ...utm, path: shared ? 'share/:token/utm' : 'utm', children: utm.children!.map((child) => (child.redirectTo ? child : { ...child, loadComponent: undefined, component: UtmRouteProbe })) };
    await TestBed.configureTestingModule({
        imports: [TranslocoTestingModule.forRoot({ langs: { en: { nav: { utm: 'UTM', analytics: 'Analytics', utmBuilder: 'Generator', qrCodes: 'QR codes' } } }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }, preloadLangs: true })],
        providers: [provideRouter([route]), provideHttpClient()]
    }).compileComponents();
    if (shared) TestBed.inject(ShareService).setToken('public-token');
}

it('opens Analytics by default and preserves addressable Generator/QR navigation', async () => {
    await configure();
    const harness = await RouterTestingHarness.create('/utm');
    const selected = () => harness.routeNativeElement?.querySelector('[role="tab"][aria-selected="true"]')?.textContent;
    expect(TestBed.inject(Router).url).toBe('/utm');
    expect(selected()).toContain('Analytics');
    const generator = Array.from(harness.routeNativeElement!.querySelectorAll('[role="tab"]')).find((tab) => tab.textContent?.includes('Generator'))!;
    await userEvent.click(generator);
    await expect.poll(() => TestBed.inject(Router).url).toBe('/utm/builder');
    await harness.navigateByUrl('/utm/qr-codes');
    expect(selected()).toContain('QR codes');
    await harness.navigateByUrl('/utm');
    expect(selected()).toContain('Analytics');
});

it('keeps shared UTM default analytics read-only and preserves its QR deep link', async () => {
    await configure(true);
    const harness = await RouterTestingHarness.create('/share/public-token/utm');
    expect(TestBed.inject(Router).url).toBe('/share/public-token/utm');
    expect(harness.routeNativeElement?.querySelector('[aria-selected="true"]')?.textContent).toContain('Analytics');
    expect(harness.routeNativeElement?.textContent).not.toContain('Generator');
    await harness.navigateByUrl('/share/public-token/utm/qr-codes');
    expect(harness.routeNativeElement?.querySelector('[aria-selected="true"]')?.textContent).toContain('QR codes');
});
