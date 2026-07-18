import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

@Component({
    selector: 'app-admin-panel',
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: {
        class: 'block min-w-0'
    },
    template: `
        <section class="min-w-0 overflow-hidden rounded-lg border border-[var(--p-content-border-color)] bg-[var(--p-content-background)]" [attr.aria-labelledby]="titleId()">
            <header [class]="headerClass()">
                <div class="grid min-w-0 gap-1">
                    <h2 class="m-0 text-base leading-5 font-semibold text-[var(--p-text-color)]" [id]="titleId()">{{ title() }}</h2>
                    @if (subtitle()) {
                        <p class="m-0 max-w-[70ch] text-sm leading-5 text-[var(--p-text-muted-color)]">{{ subtitle() }}</p>
                    }
                </div>

                <div class="admin-panel__header-actions flex min-w-0 flex-wrap items-center justify-end gap-2">
                    <ng-content select="[admin-panel-header]" />
                </div>
            </header>

            <div class="admin-panel__messages grid gap-3 px-4 pt-4 sm:px-5">
                <ng-content select="[admin-panel-messages]" />
            </div>

            <div class="min-w-0" [class.p-4]="padded()">
                <ng-content />
            </div>

            <footer class="admin-panel__footer flex flex-wrap items-center justify-between gap-3 border-t border-[var(--p-content-border-color)] px-4 py-3 sm:px-5">
                <ng-content select="[admin-panel-footer]" />
            </footer>
        </section>
    `,
    styles: `
        .admin-panel__header-actions:not(:has(*)),
        .admin-panel__messages:not(:has(*)),
        .admin-panel__footer:not(:has(*)) {
            display: none;
        }
    `
})
export class AdminPanel {
    readonly titleId = input.required<string>();
    readonly title = input.required<string>();
    readonly subtitle = input('');
    readonly padded = input(true);
    readonly stackedHeader = input(false);

    protected readonly headerClass = computed(() => {
        const base = 'flex gap-3 border-b border-[var(--p-content-border-color)] px-4 py-3 sm:px-5 sm:py-4';
        return this.stackedHeader() ? `${base} flex-col items-stretch` : `${base} flex-col items-stretch sm:flex-row sm:items-start sm:justify-between`;
    });
}
