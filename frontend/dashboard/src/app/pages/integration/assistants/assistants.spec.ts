import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { APP_BASE_HREF } from '@angular/common';
import { provideRouter } from '@angular/router';
import { TranslocoService, TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { AssistantConnection } from '@models/analytics.types';
import { TEAM_CAPABILITIES } from '@core/access/capabilities';
import { AccessService } from '@services/access.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { authGuard } from '@guards/auth-guard';
import { privateFrameGuard } from '@guards/private-frame-guard';
import { routes } from '../../../app.routes';
import { AssistantsPage } from './assistants';

const canonicalEndpoint = 'https://analytics.example/prefix/assistant/mcp';

describe('AssistantsPage', () => {
    let fixture: ComponentFixture<AssistantsPage>;
    const connection = signal<AssistantConnection>({ enabled: true, endpoint: canonicalEndpoint });
    const allowed = signal(false);
    const canActiveTeam = vi.fn(() => allowed());
    const writeText = vi.fn<(text: string) => Promise<void>>();

    beforeEach(async () => {
        connection.set({ enabled: true, endpoint: canonicalEndpoint });
        allowed.set(false);
        canActiveTeam.mockClear();
        writeText.mockReset().mockResolvedValue(undefined);
        vi.stubGlobal('navigator', { clipboard: { writeText } });
        TestBed.configureTestingModule({
            imports: [
                AssistantsPage,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: { nav: { integration: 'Integration' }, assistants: { title: 'Assistants', copied: 'Endpoint copied.' } },
                        de: { nav: { integration: 'Integrationen' }, assistants: { title: 'Assistenten', copied: 'Endpunkt kopiert.' } }
                    },
                    translocoConfig: { availableLangs: ['en', 'de'], defaultLang: 'en', reRenderOnLangChange: true },
                    preloadLangs: true
                })
            ],
            providers: [provideRouter([]), { provide: APP_BASE_HREF, useValue: '/prefix/' }, { provide: DashboardBootstrapService, useValue: { assistants: connection } }, { provide: AccessService, useValue: { canActiveTeam } }]
        });
        fixture = TestBed.createComponent(AssistantsPage);
        await fixture.whenStable();
    });

    afterEach(() => vi.unstubAllGlobals());

    it('shows viewer guidance and prefixed personal revocation without client-administration permission', () => {
        const element: HTMLElement = fixture.nativeElement;
        expect(element.querySelector('input')?.value).toBe(canonicalEndpoint);
        expect(element.querySelector('input')?.readOnly).toBe(true);
        expect(element.querySelector('a[href="/prefix/settings/connected-applications"]')).not.toBeNull();
        expect(element.querySelector('a[href="/prefix/integration/oauth-clients"]')).toBeNull();
        expect(element.querySelectorAll('a[target="_blank"]')).toHaveLength(3);
        expect(canActiveTeam).toHaveBeenCalledWith(TEAM_CAPABILITIES.manageApiClients);
    });

    it('updates registration administration when the active team capability changes', async () => {
        allowed.set(true);
        await fixture.whenStable();
        expect(fixture.nativeElement.querySelector('a[href="/prefix/integration/oauth-clients"]')).not.toBeNull();
        allowed.set(false);
        await fixture.whenStable();
        expect(fixture.nativeElement.querySelector('a[href="/prefix/integration/oauth-clients"]')).toBeNull();
    });

    it.each([{ enabled: false, endpoint: canonicalEndpoint }, { enabled: true }, { enabled: false }])('omits an unusable endpoint and copy action for %j', async (value) => {
        connection.set(value);
        await fixture.whenStable();
        expect(fixture.nativeElement.querySelector('input')).toBeNull();
        expect(fixture.nativeElement.querySelector('button')).toBeNull();
        expect(fixture.nativeElement.textContent).toContain('assistants.unavailable');
        expect(fixture.nativeElement.textContent).toContain('assistants.legacy');
        expect(fixture.nativeElement.querySelector('a[href="/prefix/integration/api-clients"]')).not.toBeNull();
        expect(writeText).not.toHaveBeenCalled();
    });

    it('copies the canonical endpoint with pending and localized accessible feedback', async () => {
        let complete!: () => void;
        const clipboardWrite = new Promise<void>((resolve) => (complete = resolve));
        writeText.mockReturnValue(clipboardWrite);
        const button: HTMLButtonElement = fixture.nativeElement.querySelector('button');
        button.click();
        await fixture.whenStable();
        expect(button.disabled).toBe(true);
        expect(writeText).toHaveBeenCalledExactlyOnceWith(canonicalEndpoint);
        complete();
        await clipboardWrite;
        await fixture.whenStable();
        expect(button.disabled).toBe(false);
        const feedback: HTMLElement = fixture.nativeElement.querySelector('p[role="status"][aria-live="polite"]');
        expect(feedback.textContent).toContain('Endpoint copied.');
        TestBed.inject(TranslocoService).setActiveLang('de');
        await fixture.whenStable();
        expect(feedback.textContent).toContain('Endpunkt kopiert.');
    });

    it('keeps the endpoint selectable and reports clipboard failure', async () => {
        writeText.mockRejectedValue(new Error('clipboard unavailable'));
        fixture.nativeElement.querySelector('button').click();
        await fixture.whenStable();
        expect(fixture.nativeElement.querySelector('p[role="status"][aria-live="polite"]').textContent).toContain('assistants.copyFailed');
        expect(fixture.nativeElement.querySelector('input').value).toBe(canonicalEndpoint);
        expect(fixture.nativeElement.querySelector('button').disabled).toBe(false);
    });
});

describe('Assistants route', () => {
    it('inherits authentication and private-frame guards without requiring client-administration capability', () => {
        const layout = routes.find((route) => route.path === '' && route.children);
        const route = layout?.children?.find((child) => child.path === 'integration/assistants');
        expect(route?.loadComponent).toBeDefined();
        expect(route?.data?.['titleKey']).toBe('assistants.title');
        expect(layout?.canActivate).toContain(authGuard);
        expect(layout?.canActivate).toContain(privateFrameGuard);
        expect(layout?.canActivateChild).toContain(privateFrameGuard);
        expect(route?.canActivate).toBeUndefined();
        expect(route?.data?.['activeTeamCapability']).toBeUndefined();
    });
});
