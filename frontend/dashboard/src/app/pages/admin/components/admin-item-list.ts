import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { CopyControl } from '@components/copy-control/copy-control';

export interface AdminItemListEntry {
    id: string;
    label: string;
    description?: string;
    descriptionMonospace?: boolean;
    copyValue?: string;
    meta?: string;
    metaLabel?: string;
}

@Component({
    selector: 'app-admin-item-list',
    imports: [CopyControl],
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: {
        class: 'block min-w-0'
    },
    template: `
        <ul class="m-0 list-none divide-y divide-[var(--p-content-border-color)] p-0" [attr.aria-label]="ariaLabel()">
            @for (item of items(); track item.id) {
                <li class="grid min-w-0 gap-2 py-3 first:pt-0 last:pb-0 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:gap-4">
                    <div class="grid min-w-0 gap-0.5">
                        <span class="text-sm leading-5 font-semibold text-[var(--p-text-color)]">{{ item.label }}</span>
                        @if (item.description) {
                            <span class="flex min-w-0 items-center gap-2">
                                <span class="min-w-0 flex-1 break-all text-xs leading-5 text-[var(--p-text-muted-color)] sm:truncate" [class.font-mono]="item.descriptionMonospace" [attr.title]="item.description">
                                    {{ item.description }}
                                </span>
                                @if (item.copyValue) {
                                    <app-copy-control [value]="item.copyValue" [text]="true" [rounded]="true" size="small" />
                                }
                            </span>
                        }
                    </div>

                    @if (item.meta) {
                        <span class="text-sm leading-5 font-semibold text-[var(--p-text-color)] tabular-nums sm:text-right">
                            @if (item.metaLabel) {
                                <span class="sr-only">{{ item.metaLabel }}: </span>
                            }
                            {{ item.meta }}
                        </span>
                    }
                </li>
            } @empty {
                <li class="py-5 text-center text-sm leading-5 text-[var(--p-text-muted-color)]">{{ emptyMessage() }}</li>
            }
        </ul>
    `
})
export class AdminItemList {
    readonly items = input.required<readonly AdminItemListEntry[]>();
    readonly ariaLabel = input.required<string>();
    readonly emptyMessage = input.required<string>();
}
