import { Route } from '@angular/router';

import type { DashboardTitleScope } from '@services/dashboard-title.service';

const titleData = (titleKey: string, titleScope: DashboardTitleScope = 'none') => ({ titleKey, titleScope });

export const SETTINGS_ROUTES: Route[] = [
    {
        path: '',
        loadComponent: () => import('@pages/settings/user/user-settings').then((m) => m.UserSettings),
        data: titleData('settings.user.title')
    },
    { path: 'user', redirectTo: '', pathMatch: 'full' },
    { path: 'preferences', redirectTo: '', pathMatch: 'full' },
    {
        path: 'reports',
        loadComponent: () => import('@pages/settings/reports/report-settings').then((m) => m.ReportSettings),
        data: titleData('nav.emailReports')
    }
];
