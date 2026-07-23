import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { BreadcrumbModule } from '@openng/optimus-ui/breadcrumb';
import { MenuItem } from '@openng/optimus-ui/api';
import { Site } from '@models/analytics.types';
import { SiteFavicon } from '@features/sites/components/site-favicon';

export type PageBreadcrumbItem = MenuItem & {
    favicon?: Site | null;
    isCurrent?: boolean;
};

@Component({
    selector: 'app-page-breadcrumb',
    standalone: true,
    imports: [BreadcrumbModule, RouterLink, SiteFavicon],
    templateUrl: './page-breadcrumb.html',
    styleUrl: './page-breadcrumb.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class PageBreadcrumb {
    items = input.required<PageBreadcrumbItem[]>();
}
