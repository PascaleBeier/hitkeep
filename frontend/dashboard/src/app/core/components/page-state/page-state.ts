import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';

@Component({
    selector: 'app-page-state',
    imports: [ButtonModule, TranslocoPipe],
    template: `
        <section class="page-state" [attr.aria-labelledby]="titleId()">
            <span class="page-state__icon"><i [class]="icon()" aria-hidden="true"></i></span>
            <h2 [id]="titleId()">{{ titleKey() | transloco }}</h2>
            <p>{{ messageKey() | transloco }}</p>
            @if (actionLabelKey()) {
                <p-button [label]="actionLabelKey()! | transloco" [icon]="actionIcon()" [outlined]="true" (onClick)="actionClicked.emit()" />
            }
        </section>
    `,
    styleUrl: './page-state.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class PageState {
    private static nextID = 0;
    private readonly defaultTitleID = `page-state-${PageState.nextID++}`;

    titleKey = input.required<string>();
    messageKey = input.required<string>();
    icon = input('pi pi-info-circle');
    titleId = input(this.defaultTitleID);
    actionLabelKey = input<string>();
    actionIcon = input('pi pi-arrow-right');
    actionClicked = output<void>();
}
