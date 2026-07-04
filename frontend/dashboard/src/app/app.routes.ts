import { Routes } from '@angular/router';
import { setupGuard } from '@guards/setup-guard';
import { authGuard } from '@guards/auth-guard';
import { capabilityGuard } from '@guards/capability-guard';
import { cloudSignupGuard } from '@guards/cloud-signup-guard';
import { importExportDefaultGuard } from '@pages/import-export/import-export-default.guard';
import { overviewDefaultGuard } from '@pages/overview/overview-default.guard';
import { INSTANCE_CAPABILITIES, TEAM_CAPABILITIES } from '@core/access/capabilities';

export const routes: Routes = [
    {
        path: 'setup',
        loadComponent: () => import('@pages/setup/setup').then((m) => m.Setup),
        canActivate: [setupGuard]
    },
    {
        path: 'login',
        loadComponent: () => import('@pages/login/login').then((m) => m.Login),
        canActivate: [setupGuard]
    },
    {
        path: 'signup',
        loadComponent: () => import('@pages/signup/signup').then((m) => m.Signup),
        canActivate: [cloudSignupGuard]
    },
    {
        path: 'forgot-password',
        loadComponent: () => import('@pages/password/forgot-password').then((m) => m.ForgotPassword)
    },
    {
        path: 'reset-password',
        loadComponent: () => import('@pages/password/reset-password').then((m) => m.ResetPassword)
    },
    {
        path: 'accept-invite',
        loadComponent: () => import('@pages/invite/accept-invite').then((m) => m.AcceptInvite)
    },
    {
        path: 'qr-share/:token',
        loadComponent: () => import('@pages/qr-share/qr-share').then((m) => m.QRSharePage),
        canActivate: [setupGuard]
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
                        loadComponent: () => import('@pages/dashboard/dashboard').then((m) => m.Dashboard)
                    },
                    {
                        path: 'opportunities',
                        loadComponent: () => import('@pages/opportunities/opportunities').then((m) => m.OpportunitiesPage)
                    },
                    {
                        path: 'events',
                        loadComponent: () => import('@pages/events/events').then((m) => m.Events)
                    },
                    {
                        path: 'web-vitals',
                        loadComponent: () => import('@pages/web-vitals/web-vitals').then((m) => m.WebVitalsPage)
                    },
                    {
                        path: 'ai-visibility',
                        loadComponent: () => import('@pages/ai-visibility/ai-visibility').then((m) => m.AIVisibility)
                    },
                    {
                        path: 'ai-chatbots',
                        loadComponent: () => import('@pages/ai-chatbots/ai-chatbots').then((m) => m.AIChatbots)
                    },
                    {
                        path: 'ecommerce',
                        loadComponent: () => import('@pages/ecommerce/ecommerce').then((m) => m.EcommercePage)
                    },
                    {
                        path: 'goals',
                        loadComponent: () => import('@pages/goals/goals').then((m) => m.Goals)
                    },
                    {
                        path: 'funnels',
                        loadComponent: () => import('@pages/funnels/funnels').then((m) => m.Funnels)
                    },
                    {
                        path: 'utm',
                        loadComponent: () => import('@pages/utm/utm').then((m) => m.UtmDashboard)
                    },
                    {
                        path: 'utm/qr-codes',
                        loadComponent: () => import('@pages/utm/qr-codes/qr-codes').then((m) => m.QRCodesPage)
                    },
                    {
                        path: 'utm/qr-codes/:qrID',
                        loadComponent: () => import('@pages/utm/qr-codes/qr-codes').then((m) => m.QRCodesPage)
                    },
                    { path: '', redirectTo: 'dashboard', pathMatch: 'full' }
                ]
            },
            {
                path: 'dashboard',
                loadComponent: () => import('@pages/dashboard/dashboard').then((m) => m.Dashboard)
            },
            {
                path: 'overview',
                loadComponent: () => import('@pages/overview/overview').then((m) => m.OverviewPage)
            },
            {
                path: 'opportunities',
                loadComponent: () => import('@pages/opportunities/opportunities').then((m) => m.OpportunitiesPage)
            },
            {
                path: 'goals',
                loadComponent: () => import('@pages/goals/goals').then((m) => m.Goals)
            },
            {
                path: 'funnels',
                loadComponent: () => import('@pages/funnels/funnels').then((m) => m.Funnels)
            },
            {
                path: 'events',
                loadComponent: () => import('@pages/events/events').then((m) => m.Events)
            },
            {
                path: 'web-vitals',
                loadComponent: () => import('@pages/web-vitals/web-vitals').then((m) => m.WebVitalsPage)
            },
            {
                path: 'ai-visibility',
                loadComponent: () => import('@pages/ai-visibility/ai-visibility').then((m) => m.AIVisibility)
            },
            {
                path: 'ai-chatbots',
                loadComponent: () => import('@pages/ai-chatbots/ai-chatbots').then((m) => m.AIChatbots)
            },
            {
                path: 'ecommerce',
                loadComponent: () => import('@pages/ecommerce/ecommerce').then((m) => m.EcommercePage)
            },
            {
                path: 'utm',
                loadComponent: () => import('@pages/utm/utm').then((m) => m.UtmDashboard)
            },
            {
                path: 'utm/builder',
                loadComponent: () => import('@pages/utm/builder/utm-builder').then((m) => m.UtmBuilder)
            },
            {
                path: 'utm/qr-codes',
                loadComponent: () => import('@pages/utm/qr-codes/qr-codes').then((m) => m.QRCodesPage)
            },
            {
                path: 'utm/qr-codes/:qrID',
                loadComponent: () => import('@pages/utm/qr-codes/qr-codes').then((m) => m.QRCodesPage)
            },
            {
                path: 'settings',
                loadChildren: () => import('@pages/settings/settings.routes').then((m) => m.SETTINGS_ROUTES)
            },
            {
                path: 'integration/api-clients',
                loadComponent: () => import('@pages/integration/api-clients/api-clients').then((m) => m.APIClientsPage)
            },
            {
                path: 'integration/api-reference',
                loadComponent: () => import('@pages/integration/api-reference/api-reference').then((m) => m.APIReferencePage)
            },
            {
                path: 'integration/google-search-console',
                loadComponent: () => import('@pages/integration/google-search-console/google-search-console').then((m) => m.GoogleSearchConsolePage),
                canActivate: [capabilityGuard],
                data: { activeTeamCapability: TEAM_CAPABILITIES.manageIntegrations }
            },
            {
                path: 'import-export',
                loadComponent: () => import('@pages/import-export/import-export').then((m) => m.ImportExportPage),
                children: [
                    {
                        path: '',
                        pathMatch: 'full',
                        canActivate: [importExportDefaultGuard],
                        children: []
                    },
                    {
                        path: 'import',
                        loadComponent: () => import('@pages/imports/imports').then((m) => m.ImportsPage)
                    },
                    {
                        path: 'export',
                        loadComponent: () => import('@pages/import-export/import-export-export').then((m) => m.ImportExportExportPage)
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
                        data: { adminPage: 'status', instanceCapability: INSTANCE_CAPABILITIES.viewSystem }
                    },
                    {
                        path: 'system',
                        loadComponent: () => import('@pages/admin/admin-settings').then((m) => m.AdminSettings),
                        canActivate: [capabilityGuard],
                        data: { adminPage: 'settings', instanceCapability: INSTANCE_CAPABILITIES.manageUsers }
                    },
                    {
                        path: 'team',
                        loadComponent: () => import('@pages/admin/team/team-admin').then((m) => m.TeamAdminPage),
                        canActivate: [capabilityGuard],
                        data: { activeTeamCapability: TEAM_CAPABILITIES.manageSettings }
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
