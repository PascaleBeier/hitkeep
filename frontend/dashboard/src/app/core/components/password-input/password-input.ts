import { ChangeDetectionStrategy, Component, input, signal } from '@angular/core';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { TranslocoPipe } from '@jsverse/transloco';
import { InputTextModule } from '@openng/optimus-ui/inputtext';

@Component({
    selector: 'app-password-input',
    imports: [ReactiveFormsModule, InputTextModule, TranslocoPipe],
    templateUrl: './password-input.html',
    styles: [':host { display: block; }'],
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class PasswordInput {
    readonly control = input.required<FormControl<string>>();
    readonly inputId = input.required<string>();
    readonly autocomplete = input.required<'current-password' | 'new-password'>();
    readonly invalid = input(false);
    readonly describedBy = input<string | null>(null);

    protected readonly visible = signal(false);
}
