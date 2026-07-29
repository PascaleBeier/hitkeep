import { CdkDragDrop, DragDropModule, moveItemInArray } from '@angular/cdk/drag-drop';
import { ChangeDetectionStrategy, Component, computed, effect, inject, input, model, output, signal } from '@angular/core';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';
import { SelectModule } from '@openng/optimus-ui/select';
import { finalize, Observable } from 'rxjs';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { injectActiveLang } from '@core/i18n/active-lang';
import { Funnel, FunnelStep } from '@models/analytics.types';
import { AnalyticsService } from '@services/analytics.service';

interface StepControl {
    key: number;
    type: FormControl<'path' | 'event'>;
    value: FormControl<string>;
}

@Component({
    selector: 'app-funnel-manager',
    standalone: true,
    imports: [ButtonModule, CrudDialog, DragDropModule, InputTextModule, MessageModule, ReactiveFormsModule, SelectModule, TranslocoPipe],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <app-crud-dialog [title]="dialogTitle()" [visible]="visible()" (visibleChange)="onVisibleChange($event)" [submitLabel]="submitLabel()" [cancelLabel]="'common.actions.cancel' | transloco" [saving]="saving()" (submitted)="saveFunnel()">
            <form class="flex flex-col gap-5" (ngSubmit)="saveFunnel()">
                <div class="flex flex-col gap-1">
                    <label for="funnel-name" class="text-sm font-medium">{{ 'common.columns.name' | transloco }}</label>
                    <input pInputText id="funnel-name" [formControl]="nameControl" [placeholder]="'funnels.manager.namePlaceholder' | transloco" autocomplete="off" />
                </div>

                <div>
                    <div class="flex items-center justify-between gap-3 mb-2">
                        <div>
                            <h3 class="font-medium">{{ 'funnels.manager.stepsTitle' | transloco }}</h3>
                            <p class="text-xs text-muted-color mt-1">{{ 'funnels.manager.reorderHelp' | transloco }}</p>
                        </div>
                        <p-button [label]="'funnels.manager.addStep' | transloco" icon="pi pi-plus" size="small" [outlined]="true" (onClick)="addStep()" />
                    </div>
                    <ol cdkDropList class="m-0 p-0 list-none flex flex-col gap-2" (cdkDropListDropped)="dropStep($event)">
                        @for (step of steps(); track step.key; let index = $index) {
                            <li cdkDrag class="grid grid-cols-[auto_minmax(7rem,9rem)_1fr_auto] items-center gap-2 rounded-md border border-surface-200 dark:border-surface-700 p-2">
                                <button type="button" cdkDragHandle class="min-w-11 min-h-11 text-muted-color cursor-grab" [attr.aria-label]="'funnels.manager.dragStepAria' | transloco: { index: index + 1 }">
                                    <i class="pi pi-bars" aria-hidden="true"></i>
                                </button>
                                <p-select [options]="types()" [formControl]="step.type" optionLabel="label" optionValue="value" appendTo="body" />
                                <input
                                    pInputText
                                    [formControl]="step.value"
                                    [attr.aria-label]="'funnels.manager.stepValueAria' | transloco: { index: index + 1 }"
                                    [attr.list]="step.type.value === 'path' ? 'funnel-path-suggestions' : 'funnel-event-suggestions'"
                                    autocomplete="off"
                                />
                                <div class="flex items-center">
                                    <p-button icon="pi pi-angle-up" [text]="true" size="small" [disabled]="index === 0" [ariaLabel]="'funnels.manager.moveUpAria' | transloco" (onClick)="moveStep(index, -1)" />
                                    <p-button icon="pi pi-angle-down" [text]="true" size="small" [disabled]="index === steps().length - 1" [ariaLabel]="'funnels.manager.moveDownAria' | transloco" (onClick)="moveStep(index, 1)" />
                                    <p-button icon="pi pi-trash" severity="danger" [text]="true" size="small" [disabled]="steps().length <= 2" [ariaLabel]="'funnels.manager.removeStepAria' | transloco" (onClick)="removeStep(index)" />
                                </div>
                            </li>
                        }
                    </ol>
                    <datalist id="funnel-path-suggestions">
                        @for (value of pathSuggestions(); track value) {
                            <option [value]="value"></option>
                        }
                    </datalist>
                    <datalist id="funnel-event-suggestions">
                        @for (value of eventSuggestions(); track value) {
                            <option [value]="value"></option>
                        }
                    </datalist>
                </div>
                @if (errorKey()) {
                    <p-message severity="error" [text]="errorKey()! | transloco" />
                }
            </form>
        </app-crud-dialog>
    `
})
export class FunnelManager {
    visible = model(false);
    siteId = input<string | null>(null);
    funnel = input<Funnel | null>(null);
    pathSuggestions = input<string[]>([]);
    eventSuggestions = input<string[]>([]);
    funnelsChanged = output<void>();

    private analytics = inject(AnalyticsService);
    private transloco = inject(TranslocoService);
    private activeLanguage = injectActiveLang();
    private nextKey = 0;
    protected nameControl = new FormControl('', { nonNullable: true, validators: [Validators.required] });
    protected steps = signal<StepControl[]>([]);
    protected saving = signal(false);
    protected errorKey = signal<string | null>(null);
    protected types = computed(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('funnels.manager.typePagePath'), value: 'path' },
            { label: this.transloco.translate('funnels.manager.typeCustomEvent'), value: 'event' }
        ];
    });
    protected dialogTitle = computed(() => {
        this.activeLanguage();
        return this.transloco.translate(this.funnel() ? 'funnels.manager.editTitle' : 'funnels.manager.createTitle');
    });
    protected submitLabel = computed(() => {
        this.activeLanguage();
        return this.transloco.translate(this.funnel() ? 'funnels.manager.saveAction' : 'funnels.manager.createAction');
    });

    constructor() {
        effect(() => {
            if (!this.visible()) return;
            const funnel = this.funnel();
            this.errorKey.set(null);
            this.nameControl.setValue(funnel?.name ?? '');
            const initialSteps: FunnelStep[] = funnel?.steps?.length
                ? funnel.steps
                : [
                      { type: 'path', value: '' },
                      { type: 'path', value: '' }
                  ];
            this.steps.set(initialSteps.map((step) => this.createStep(step)));
        });
    }

    private createStep(step: FunnelStep): StepControl {
        return {
            key: this.nextKey++,
            type: new FormControl(step.type, { nonNullable: true, validators: [Validators.required] }),
            value: new FormControl(step.value, { nonNullable: true, validators: [Validators.required] })
        };
    }

    protected onVisibleChange(visible: boolean) {
        if (!visible && this.saving()) return;
        this.visible.set(visible);
    }
    protected addStep() {
        this.steps.update((steps) => [...steps, this.createStep({ type: 'path', value: '' })]);
    }
    protected removeStep(index: number) {
        if (this.steps().length <= 2) return;
        this.steps.update((steps) => steps.filter((_, current) => current !== index));
    }
    protected moveStep(index: number, direction: -1 | 1) {
        const target = index + direction;
        if (target < 0 || target >= this.steps().length) return;
        this.steps.update((steps) => {
            const next = [...steps];
            moveItemInArray(next, index, target);
            return next;
        });
    }
    protected dropStep(event: CdkDragDrop<StepControl[]>) {
        this.steps.update((steps) => {
            const next = [...steps];
            moveItemInArray(next, event.previousIndex, event.currentIndex);
            return next;
        });
    }

    protected saveFunnel() {
        const siteId = this.siteId();
        const funnel = this.funnel();
        const steps = this.steps().map((step) => ({ type: step.type.value, value: step.value.value.trim() }));
        const payload = { name: this.nameControl.value.trim(), steps };
        if (!siteId || !payload.name || steps.length < 2 || steps.some((step) => !step.value) || this.saving()) return;
        this.saving.set(true);
        this.errorKey.set(null);
        const request: Observable<unknown> = funnel ? this.analytics.updateFunnel(siteId, funnel.id, payload) : this.analytics.createFunnel(siteId, payload);
        request.pipe(finalize(() => this.saving.set(false))).subscribe({
            next: () => {
                this.visible.set(false);
                this.funnelsChanged.emit();
            },
            error: () => this.errorKey.set('funnels.manager.errors.save')
        });
    }
}
