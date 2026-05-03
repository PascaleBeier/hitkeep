import { ComponentFixture, TestBed } from '@angular/core/testing';
import { MainLayout } from '@layout/main-layout';
import { provideRouter } from '@angular/router';
import { By } from '@angular/platform-browser';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { PermissionService } from '@services/permission.service';
import { TeamService } from '@services/team.service';
import { vi } from 'vitest';

interface MainLayoutTestAccess {
    beforeTeamSwitch(): boolean;
    isSiteSettingsVisible: {
        (): boolean;
        set(value: boolean): void;
    };
}

describe('MainLayout', () => {
    let component: MainLayout;
    let fixture: ComponentFixture<MainLayout>;
    let httpMock: HttpTestingController;
    let bootstrap: DashboardBootstrapService;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                MainLayout,
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()]
        }).compileComponents();

        fixture = TestBed.createComponent(MainLayout);
        component = fixture.componentInstance;
        httpMock = TestBed.inject(HttpTestingController);
        bootstrap = TestBed.inject(DashboardBootstrapService);
        seedLayoutState();
        fixture.detectChanges();
        fixture.detectChanges();
    });

    afterEach(() => {
        httpMock.verify();
        vi.restoreAllMocks();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('A11Y: should have correct landmarks', () => {
        const aside = fixture.debugElement.query(By.css('aside'));
        const main = fixture.debugElement.query(By.css('main'));
        const nav = fixture.debugElement.query(By.css('nav'));

        expect(aside).toBeTruthy();
        expect(main).toBeTruthy();
        expect(nav).toBeTruthy();

        // Check labels
        expect(aside.attributes['aria-label']).toBeTruthy();
        expect(main.attributes['role']).toBe('main');
    });

    it('A11Y: buttons should have accessible labels', () => {
        const buttons = fixture.debugElement.queryAll(By.css('button'));
        const buttonsWithAria = buttons.filter((btn) => !!btn.attributes['aria-label']);
        expect(buttonsWithAria.length).toBeGreaterThan(0);
    });

    it('should always render team switcher', () => {
        const switchers = fixture.debugElement.queryAll(By.css('app-team-switcher'));
        expect(switchers.length).toBeGreaterThan(0);
    });

    it('should not render the legacy floating desktop account cluster', () => {
        const topbarCluster = fixture.nativeElement.querySelector('.layout-topbar__account-cluster');
        expect(topbarCluster).toBeNull();
    });

    it('should show administration section for team owner/admin role', () => {
        const adminLinks = Array.from(fixture.nativeElement.querySelectorAll('nav a')) as HTMLElement[];
        const hasTeamLink = adminLinks.some((link: HTMLElement) => link.getAttribute('href') === '/admin/team');
        expect(hasTeamLink).toBeTruthy();
    });

    it('should hide administration section for team member role', () => {
        const teamService = TestBed.inject(TeamService);
        teamService.teams.set([{ id: '00000000-0000-0000-0000-000000000001', name: 'Alpha Team', logo_url: '', role: 'member', created_at: '2026-01-01T00:00:00Z' }]);
        teamService.activeTeamId.set('00000000-0000-0000-0000-000000000001');
        fixture.detectChanges();
        const adminLinks = Array.from(fixture.nativeElement.querySelectorAll('nav a')) as HTMLElement[];
        const hasTeamLink = adminLinks.some((link: HTMLElement) => link.getAttribute('href') === '/admin/team');
        expect(hasTeamLink).toBeFalsy();
    });

    it('should hide create team actions in hosted cloud', () => {
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: { hosted: true, signup_enabled: false }
        });
        fixture.detectChanges();

        const switchers = fixture.debugElement.queryAll(By.css('app-team-switcher'));
        expect(switchers.length).toBeGreaterThan(0);
        for (const switcher of switchers) {
            expect(switcher.componentInstance.showAdd()).toBe(false);
        }
    });

    it('should show docs link in oss mode and hide support link', () => {
        const links = Array.from(fixture.nativeElement.querySelectorAll('a[href]')) as HTMLAnchorElement[];

        const docsLink = links.find((link) => link.href === 'https://hitkeep.com/guides/introduction/');
        const supportLink = links.find((link) => link.href === 'https://hitkeep.com/support/help/');

        expect(docsLink).toBeTruthy();
        expect(docsLink?.querySelector('.pi-external-link')).toBeTruthy();
        expect(supportLink).toBeFalsy();
    });

    it('should show support link in hosted cloud mode', () => {
        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: {
                hosted: true,
                signup_enabled: false,
                support_url: 'https://hitkeep.com/support/help/'
            }
        });
        fixture.detectChanges();

        const links = Array.from(fixture.nativeElement.querySelectorAll('a[href]')) as HTMLAnchorElement[];
        const supportLink = links.find((link) => link.href === 'https://hitkeep.com/support/help/');

        expect(supportLink).toBeTruthy();
        expect(supportLink?.querySelector('.pi-headphones')).toBeTruthy();
        expect(supportLink?.querySelector('.pi-external-link')).toBeTruthy();
    });

    it('should allow team switch without confirmation when settings drawer is closed', () => {
        const confirmSpy = vi.spyOn(window, 'confirm');
        const access = component as unknown as MainLayoutTestAccess;
        const result = access.beforeTeamSwitch();
        expect(result).toBe(true);
        expect(confirmSpy).not.toHaveBeenCalled();
    });

    it('should block team switch when settings drawer is open and user cancels', () => {
        const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
        const access = component as unknown as MainLayoutTestAccess;
        access.isSiteSettingsVisible.set(true);
        const result = access.beforeTeamSwitch();
        expect(result).toBe(false);
        expect(confirmSpy).toHaveBeenCalled();
        expect(access.isSiteSettingsVisible()).toBe(true);
    });

    it('should close settings drawer when switch is confirmed', () => {
        const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
        const access = component as unknown as MainLayoutTestAccess;
        access.isSiteSettingsVisible.set(true);
        const result = access.beforeTeamSwitch();
        expect(result).toBe(true);
        expect(confirmSpy).toHaveBeenCalled();
        expect(access.isSiteSettingsVisible()).toBe(false);
    });

    function seedLayoutState() {
        const teamService = TestBed.inject(TeamService);
        const permissions = TestBed.inject(PermissionService);

        teamService.applyTeams({
            active_team_id: '00000000-0000-0000-0000-000000000001',
            teams: [
                {
                    id: '00000000-0000-0000-0000-000000000001',
                    name: 'Alpha Team',
                    logo_url: '',
                    role: 'owner',
                    created_at: '2026-01-01T00:00:00Z'
                },
                {
                    id: '00000000-0000-0000-0000-000000000002',

                    name: 'Beta Team',
                    logo_url: '',
                    role: 'admin',
                    created_at: '2026-01-02T00:00:00Z'
                }
            ]
        });

        bootstrap.status.set({
            needs_setup: false,
            version: 'v2.0.0',
            cloud: {
                hosted: false,
                signup_enabled: false
            }
        });

        permissions.applyPermissions({
            instance_role: 'owner',
            permissions: {}
        });
    }
});
