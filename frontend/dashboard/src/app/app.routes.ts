import { Routes } from '@angular/router';
import { setupGuard } from '@guards/setup-guard';
import { adminGuard } from '@guards/admin-guard';
import { teamAdminGuard } from '@guards/team-admin-guard';
import { cloudSignupGuard } from '@guards/cloud-signup-guard';

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
        path: '',
        loadComponent: () => import('@layout/main-layout').then((m) => m.MainLayout),
        canActivate: [setupGuard],
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
                        path: 'events',
                        loadComponent: () => import('@pages/events/events').then((m) => m.Events)
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
                    { path: '', redirectTo: 'dashboard', pathMatch: 'full' }
                ]
            },
            {
                path: 'dashboard',
                loadComponent: () => import('@pages/dashboard/dashboard').then((m) => m.Dashboard)
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
                path: 'imports',
                loadComponent: () => import('@pages/imports/imports').then((m) => m.ImportsPage)
            },
            {
                path: 'admin',
                children: [
                    {
                        path: 'status',
                        loadComponent: () => import('@pages/admin/admin-settings').then((m) => m.AdminSettings),
                        canActivate: [adminGuard],
                        data: { adminPage: 'status' }
                    },
                    {
                        path: 'system',
                        loadComponent: () => import('@pages/admin/admin-settings').then((m) => m.AdminSettings),
                        canActivate: [adminGuard],
                        data: { adminPage: 'settings' }
                    },
                    {
                        path: 'team',
                        loadComponent: () => import('@pages/admin/team/team-admin').then((m) => m.TeamAdminPage),
                        canActivate: [teamAdminGuard]
                    },
                    { path: 'team/overview', redirectTo: 'team', pathMatch: 'full' },
                    { path: 'team/members', redirectTo: 'team', pathMatch: 'full' },
                    { path: 'team/settings', redirectTo: 'team', pathMatch: 'full' },
                    { path: '', redirectTo: 'team', pathMatch: 'full' }
                ]
            },
            { path: '', redirectTo: 'dashboard', pathMatch: 'full' }
        ]
    },
    { path: '**', redirectTo: '/dashboard' }
];
