import { ChangeDetectionStrategy, Component, computed, effect, inject, input, model, output, signal } from '@angular/core';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { InputGroupModule } from '@openng/optimus-ui/inputgroup';
import { InputGroupAddonModule } from '@openng/optimus-ui/inputgroupaddon';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';
import { finalize, Observable } from 'rxjs';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { Goal } from '@models/analytics.types';
import { AnalyticsService } from '@services/analytics.service';
import { injectActiveLang } from '@core/i18n/active-lang';

@Component({
    selector: 'app-goal-manager',
    standalone: true,
    imports: [CrudDialog, InputGroupAddonModule, InputGroupModule, InputTextModule, MessageModule, ReactiveFormsModule, SelectButtonModule, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <app-crud-dialog
            [title]="dialogTitle()"
            [visible]="visible()"
            (visibleChange)="onVisibleChange($event)"
            [submitLabel]="submitLabel()"
            [cancelLabel]="'common.actions.cancel' | transloco"
            [saving]="saving()"
            (submitted)="saveGoal()"
        >
            <form class="flex flex-col gap-4" (ngSubmit)="saveGoal()">
                <div class="flex flex-col gap-1">
                    <label for="goal-name" class="text-sm font-medium">{{ 'common.columns.name' | transloco }}</label>
                    <input pInputText id="goal-name" [formControl]="form.name().control()" [placeholder]="'goals.manager.namePlaceholder' | transloco" autocomplete="off" />
                </div>
                <div class="flex flex-col gap-1">
                    <span id="goal-type-label" class="text-sm font-medium">{{ 'common.columns.type' | transloco }}</span>
                    <p-selectbutton [options]="types()" [formControl]="form.type().control()" optionLabel="label" optionValue="value" ariaLabelledBy="goal-type-label" />
                </div>
                <div class="flex flex-col gap-1">
                    <label for="goal-value" class="text-sm font-medium">
                        {{ form.type().value() === 'path' ? ('goals.manager.urlPathLabel' | transloco) : ('goals.manager.eventNameLabel' | transloco) }}
                    </label>
                    <p-inputgroup>
                        <p-inputgroup-addon><i [class]="form.type().value() === 'path' ? 'pi pi-link' : 'pi pi-bolt'" aria-hidden="true"></i></p-inputgroup-addon>
                        <input
                            pInputText
                            id="goal-value"
                            [formControl]="form.value().control()"
                            [attr.list]="form.type().value() === 'path' ? 'goal-path-suggestions' : 'goal-event-suggestions'"
                            [placeholder]="(form.type().value() === 'path' ? 'goals.manager.urlPathPlaceholder' : 'goals.manager.eventNamePlaceholder') | transloco"
                            autocomplete="off"
                        />
                    </p-inputgroup>
                    <datalist id="goal-path-suggestions">@for (value of pathSuggestions(); track value) { <option [value]="value"></option> }</datalist>
                    <datalist id="goal-event-suggestions">@for (value of eventSuggestions(); track value) { <option [value]="value"></option> }</datalist>
                    <small class="text-xs text-muted-color">{{ 'goals.manager.suggestionsHelp' | transloco }}</small>
                </div>
                @if (errorKey()) { <p-message severity="error" [text]="errorKey()! | transloco" /> }
            </form>
        </app-crud-dialog>
    `
})
export class GoalManager {
    visible = model(false);
    siteId = input<string | null>(null);
    goal = input<Goal | null>(null);
    pathSuggestions = input<string[]>([]);
    eventSuggestions = input<string[]>([]);
    goalsChanged = output<void>();

    private analytics = inject(AnalyticsService);
    private transloco = inject(TranslocoService);
    private activeLanguage = injectActiveLang();
    protected saving = signal(false);
    protected errorKey = signal<string | null>(null);
    private formModel = signal({
        name: new FormControl('', { nonNullable: true, validators: [Validators.required] }),
        type: new FormControl<'path' | 'event'>('path', { nonNullable: true, validators: [Validators.required] }),
        value: new FormControl('', { nonNullable: true, validators: [Validators.required] })
    });
    protected form = compatForm(this.formModel);
    protected types = computed(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('goals.manager.typePagePath'), value: 'path' },
            { label: this.transloco.translate('goals.manager.typeCustomEvent'), value: 'event' }
        ];
    });
    protected dialogTitle = computed(() => {
        this.activeLanguage();
        return this.transloco.translate(this.goal() ? 'goals.manager.editTitle' : 'goals.manager.addTitle');
    });
    protected submitLabel = computed(() => {
        this.activeLanguage();
        return this.transloco.translate(this.goal() ? 'goals.manager.saveAction' : 'goals.manager.createAction');
    });

    constructor() {
        effect(() => {
            if (!this.visible()) return;
            const goal = this.goal();
            this.errorKey.set(null);
            this.form
                .name()
                .control()
                .setValue(goal?.name ?? '');
            this.form
                .type()
                .control()
                .setValue(goal?.type ?? 'path');
            this.form
                .value()
                .control()
                .setValue(goal?.value ?? '');
        });
    }

    protected onVisibleChange(visible: boolean) {
        if (!visible && this.saving()) return;
        this.visible.set(visible);
    }

    protected saveGoal() {
        const siteId = this.siteId();
        const goal = this.goal();
        const payload = { name: this.form.name().value().trim(), type: this.form.type().value(), value: this.form.value().value().trim() };
        if (!siteId || !payload.name || !payload.value || this.saving()) return;
        this.saving.set(true);
        this.errorKey.set(null);
        const request: Observable<unknown> = goal ? this.analytics.updateGoal(siteId, goal.id, payload) : this.analytics.createGoal(siteId, payload);
        request.pipe(finalize(() => this.saving.set(false))).subscribe({
            next: () => {
                this.visible.set(false);
                this.goalsChanged.emit();
            },
            error: () => this.errorKey.set('goals.manager.errors.save')
        });
    }
}
