import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { PageHeader, PageHeaderLeft } from '@components/page-header/page-header';

@Component({
    selector: 'app-page-frame',
    imports: [PageHeader, PageHeaderLeft, PageBreadcrumb],
    templateUrl: './page-frame.html',
    styleUrl: './page-frame.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class PageFrame {
    breadcrumbItems = input.required<PageBreadcrumbItem[]>();
    subtitle = input('');
}
