import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { TranslocoPipe } from '@jsverse/transloco';
import { CardModule } from '@openng/optimus-ui/card';
import { SelectModule } from '@openng/optimus-ui/select';

export interface ConversionSubjectOption {
    label: string;
    value: string | null;
}

@Component({
    selector: 'app-conversion-subject-card',
    standalone: true,
    imports: [CardModule, ReactiveFormsModule, SelectModule, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <p-card styleClass="mb-6 border border-surface-200 shadow-none dark:border-surface-700">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
                <div class="min-w-0 flex-1">
                    <h2 [id]="headingId()" class="text-base font-semibold">
                        {{ labelKey() | transloco }}
                    </h2>
                    <p class="mt-1 text-sm text-muted-color break-words">
                        {{ description() }}
                    </p>
                </div>

                <div class="w-full sm:w-80 lg:shrink-0">
                    <label class="sr-only" [for]="controlId()">{{ labelKey() | transloco }}</label>
                    <p-select
                        [inputId]="controlId()"
                        [options]="options()"
                        [formControl]="control()"
                        optionLabel="label"
                        optionValue="value"
                        [filter]="options().length > 6"
                        filterBy="label"
                        class="w-full"
                        (onChange)="subjectChanged.emit($event.value)"
                    />
                </div>
            </div>
        </p-card>
    `
})
export class ConversionSubjectCard {
    labelKey = input.required<string>();
    description = input.required<string>();
    options = input.required<ConversionSubjectOption[]>();
    control = input.required<FormControl<string | null>>();
    controlId = input.required<string>();
    headingId = input.required<string>();

    subjectChanged = output<string | null>();
}
