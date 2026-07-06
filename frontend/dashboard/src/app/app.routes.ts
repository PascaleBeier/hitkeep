import { Routes } from '@angular/router';
import { setupGuard } from '@guards/setup-guard';
import { authGuard } from '@guards/auth-guard';
import { capabilityGuard } from '@guards/capability-guard';
import { cloudSignupGuard } from '@guards/cloud-signup-guard';
import { importExportDefaultGuard } from '@pages/import-export/import-export-default.guard';
import { overviewDefaultGuard } from '@pages/overview/overview-default.guard';
import { INSTANCE_CAPABILITIES, TEAM_CAPABILITIES } from '@core/access/capabilities';
import type { DashboardTitleScope } from '@services/dashboard-title.service';

const titleData = (titleKey: string, titleScope: DashboardTitleScope = 'none') => ({ titleKey, titleScope });

export const routes: Routes = [
    {
        path: 'setup',
        loadComponent: () => import('@pages/setup/setup').then((m) => m.Setup),
        canActivate: [setupGuard],
        data: titleData('setup.createAdminAction')
    },
    {
        path: 'login',
        loadComponent: () => import('@pages/login/login').then((m) => m.Login),
        canActivate: [setupGuard],
        data: titleData('login.signIn')
    },
    {
        path: 'signup',
        loadComponent: () => import('@pages/signup/signup').then((m) => m.Signup),
        canActivate: [cloudSignupGuard],
        data: titleData('signup.title')
    },
    {
        path: 'forgot-password',
        loadComponent: () => import('@pages/password/forgot-password').then((m) => m.ForgotPassword),
        data: titleData('password.forgot.title')
    },
    {
        path: 'reset-password',
        loadComponent: () => import('@pages/password/reset-password').then((m) => m.ResetPassword),
        data: titleData('password.reset.title')
    },
    {
        path: 'accept-invite',
        loadComponent: () => import('@pages/invite/accept-invite').then((m) => m.AcceptInvite),
        data: titleData('invite.accept.title')
    },
    {
        path: 'qr-share/:token',
        loadComponent: () => import('@pages/qr-share/qr-share').then((m) => m.QRSharePage),
        canActivate: [setupGuard],
        data: titleData('qrCodes.share.title')
    },
    {
        path: '',
        loadComponent: () => import('@layout/main-layout').then((m) => m.MainLayout),
        canActivate: [setupGuard, authGuard],
        children: [
            {
                path: 'share/:token',
                loadComponent: () => import('@pages/share/share').then((m) => m.ShareDashboard),
                children: [
                    {
                        path: 'dashboard',
                        loadComponent: () => import('@pages/dashboard/dashboard').then((m) => m.Dashboard),
                        data: titleData('nav.dashboard', 'site')
                    },
                    {
                        path: 'opportunities',
                        loadComponent: () => import('@pages/opportunities/opportunities').then((m) => m.OpportunitiesPage),
                        data: titleData('nav.opportunities', 'site')
                    },
                    {
                        path: 'events',
                        loadComponent: () => import('@pages/events/events').then((m) => m.Events),
                        data: titleData('nav.events', 'site')
                    },
                    {
                        path: 'web-vitals',
                        loadComponent: () => import('@pages/web-vitals/web-vitals').then((m) => m.WebVitalsPage),
                        data: titleData('nav.webVitals', 'site')
                    },
                    {
                        path: 'ai-visibility',
                        loadComponent: () => import('@pages/ai-visibility/ai-visibility').then((m) => m.AIVisibility),
                        data: titleData('nav.aiVisibility', 'site')
                    },
                    {
                        path: 'ai-chatbots',
                        loadComponent: () => import('@pages/ai-chatbots/ai-chatbots').then((m) => m.AIChatbots),
                        data: titleData('nav.aiChatbots', 'site')
                    },
                    {
                        path: 'ecommerce',
                        loadComponent: () => import('@pages/ecommerce/ecommerce').then((m) => m.EcommercePage),
                        data: titleData('nav.ecommerce', 'site')
                    },
                    {
                        path: 'goals',
                        loadComponent: () => import('@pages/goals/goals').then((m) => m.Goals),
                        data: titleData('nav.goals', 'site')
                    },
                    {
                        path: 'funnels',
                        loadComponent: () => import('@pages/funnels/funnels').then((m) => m.Funnels),
                        data: titleData('nav.funnels', 'site')
                    },
                    {
                        path: 'utm',
                        loadComponent: () => import('@pages/utm/utm').then((m) => m.UtmDashboard),
                        data: titleData('nav.utm', 'site')
                    },
                    {
                        path: 'utm/qr-codes',
                        loadComponent: () => import('@pages/utm/qr-codes/qr-codes').then((m) => m.QRCodesPage),
                        data: titleData('nav.qrCodes', 'site')
                    },
                    {
                        path: 'utm/qr-codes/:qrID',
                        loadComponent: () => import('@pages/utm/qr-codes/qr-codes').then((m) => m.QRCodesPage),
                        data: titleData('nav.qrCodes', 'site')
                    },
                    { path: '', redirectTo: 'dashboard', pathMatch: 'full' }
                ]
            },
            {
                path: 'dashboard',
                loadComponent: () => import('@pages/dashboard/dashboard').then((m) => m.Dashboard),
                data: titleData('nav.dashboard', 'site')
            },
            {
                path: 'overview',
                loadComponent: () => import('@pages/overview/overview').then((m) => m.OverviewPage),
                data: titleData('nav.overview', 'team')
            },
            {
                path: 'opportunities',
                loadComponent: () => import('@pages/opportunities/opportunities').then((m) => m.OpportunitiesPage),
                data: titleData('nav.opportunities', 'site')
            },
            {
                path: 'goals',
                loadComponent: () => import('@pages/goals/goals').then((m) => m.Goals),
                data: titleData('nav.goals', 'site')
            },
            {
                path: 'funnels',
                loadComponent: () => import('@pages/funnels/funnels').then((m) => m.Funnels),
                data: titleData('nav.funnels', 'site')
            },
            {
                path: 'events',
                loadComponent: () => import('@pages/events/events').then((m) => m.Events),
                data: titleData('nav.events', 'site')
            },
            {
                path: 'web-vitals',
                loadComponent: () => import('@pages/web-vitals/web-vitals').then((m) => m.WebVitalsPage),
                data: titleData('nav.webVitals', 'site')
            },
            {
                path: 'ai-visibility',
                loadComponent: () => import('@pages/ai-visibility/ai-visibility').then((m) => m.AIVisibility),
                data: titleData('nav.aiVisibility', 'site')
            },
            {
                path: 'ai-chatbots',
                loadComponent: () => import('@pages/ai-chatbots/ai-chatbots').then((m) => m.AIChatbots),
                data: titleData('nav.aiChatbots', 'site')
            },
            {
                path: 'ecommerce',
                loadComponent: () => import('@pages/ecommerce/ecommerce').then((m) => m.EcommercePage),
                data: titleData('nav.ecommerce', 'site')
            },
            {
                path: 'utm',
                loadComponent: () => import('@pages/utm/utm').then((m) => m.UtmDashboard),
                data: titleData('nav.utm', 'site')
            },
            {
                path: 'utm/builder',
                loadComponent: () => import('@pages/utm/builder/utm-builder').then((m) => m.UtmBuilder),
                data: titleData('nav.utmBuilder', 'site')
            },
            {
                path: 'utm/qr-codes',
                loadComponent: () => import('@pages/utm/qr-codes/qr-codes').then((m) => m.QRCodesPage),
                data: titleData('nav.qrCodes', 'site')
            },
            {
                path: 'utm/qr-codes/:qrID',
                loadComponent: () => import('@pages/utm/qr-codes/qr-codes').then((m) => m.QRCodesPage),
                data: titleData('nav.qrCodes', 'site')
            },
            {
                path: 'settings',
                loadChildren: () => import('@pages/settings/settings.routes').then((m) => m.SETTINGS_ROUTES)
            },
            {
                path: 'integration/api-clients',
                loadComponent: () => import('@pages/integration/api-clients/api-clients').then((m) => m.APIClientsPage),
                data: titleData('nav.apiClients', 'team')
            },
            {
                path: 'integration/api-reference',
                loadComponent: () => import('@pages/integration/api-reference/api-reference').then((m) => m.APIReferencePage),
                data: titleData('nav.apiReference')
            },
            {
                path: 'integration/google-search-console',
                loadComponent: () => import('@pages/integration/google-search-console/google-search-console').then((m) => m.GoogleSearchConsolePage),
                canActivate: [capabilityGuard],
                data: { ...titleData('nav.googleSearchConsole', 'team'), activeTeamCapability: TEAM_CAPABILITIES.manageIntegrations }
            },
            {
                path: 'import-export',
                loadComponent: () => import('@pages/import-export/import-export').then((m) => m.ImportExportPage),
                data: titleData('nav.importExport', 'site'),
                children: [
                    {
                        path: '',
                        pathMatch: 'full',
                        canActivate: [importExportDefaultGuard],
                        children: []
                    },
                    {
                        path: 'import',
                        loadComponent: () => import('@pages/imports/imports').then((m) => m.ImportsPage),
                        data: titleData('importExport.import.title', 'site')
                    },
                    {
                        path: 'export',
                        loadComponent: () => import('@pages/import-export/import-export-export').then((m) => m.ImportExportExportPage),
                        data: titleData('importExport.export.title', 'site')
                    }
                ]
            },
            {
                path: 'admin',
                children: [
                    {
                        path: 'status',
                        loadComponent: () => import('@pages/admin/admin-settings').then((m) => m.AdminSettings),
                        canActivate: [capabilityGuard],
                        data: { ...titleData('nav.systemStatus'), adminPage: 'status', instanceCapability: INSTANCE_CAPABILITIES.viewSystem }
                    },
                    {
                        path: 'system',
                        loadComponent: () => import('@pages/admin/admin-settings').then((m) => m.AdminSettings),
                        canActivate: [capabilityGuard],
                        data: { ...titleData('nav.systemSettings'), adminPage: 'settings', instanceCapability: INSTANCE_CAPABILITIES.manageUsers }
                    },
                    {
                        path: 'team',
                        loadComponent: () => import('@pages/admin/team/team-admin').then((m) => m.TeamAdminPage),
                        canActivate: [capabilityGuard],
                        data: { ...titleData('nav.team', 'team'), activeTeamCapability: TEAM_CAPABILITIES.manageSettings }
                    },
                    { path: 'team/overview', redirectTo: 'team', pathMatch: 'full' },
                    { path: 'team/members', redirectTo: 'team', pathMatch: 'full' },
                    { path: 'team/settings', redirectTo: 'team', pathMatch: 'full' },
                    { path: '', redirectTo: 'team', pathMatch: 'full' }
                ]
            },
            {
                path: '',
                pathMatch: 'full',
                canActivate: [overviewDefaultGuard],
                children: []
            }
        ]
    },
    { path: '**', redirectTo: '/dashboard' }
];
