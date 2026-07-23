import { ChangeDetectionStrategy, Component, computed, effect, inject, input, model, output, signal } from '@angular/core';

import { CdkDragDrop, DragDropModule, moveItemInArray } from '@angular/cdk/drag-drop';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { rxResource, toSignal } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { switchMap, finalize } from 'rxjs';
import { ButtonModule } from '@openng/optimus-ui/button';
import { ConfirmDialogModule } from '@openng/optimus-ui/confirmdialog';
import { ConfirmationService } from '@openng/optimus-ui/api';
import { IconFieldModule } from '@openng/optimus-ui/iconfield';
import { InputIconModule } from '@openng/optimus-ui/inputicon';
import { InputGroupModule } from '@openng/optimus-ui/inputgroup';
import { InputGroupAddonModule } from '@openng/optimus-ui/inputgroupaddon';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { SelectButtonModule } from '@openng/optimus-ui/selectbutton';
import { TableModule } from '@openng/optimus-ui/table';
import { TagModule } from '@openng/optimus-ui/tag';
import { TooltipModule } from '@openng/optimus-ui/tooltip';
import { MessageModule } from '@openng/optimus-ui/message';
import { AnalyticsService } from '@services/analytics.service';
import { Funnel, FunnelStep } from '@models/analytics.types';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';

type FunnelManagerAction = 'create' | 'update' | 'delete';

interface FunnelStepControl {
    typeControl: FormControl<FunnelStep['type']>;
    valueControl: FormControl<string>;
}

@Component({
    selector: 'app-funnel-manager',
    imports: [
        DragDropModule,
        ReactiveFormsModule,
        ButtonModule,
        ConfirmDialogModule,
        IconFieldModule,
        InputIconModule,
        InputGroupModule,
        InputGroupAddonModule,
        InputTextModule,
        SelectButtonModule,
        TableModule,
        TagModule,
        TooltipModule,
        MessageModule,
        CrudDialog,
        DialogShell,
        TableRowActions,
        TranslocoPipe
    ],
    changeDetection: ChangeDetectionStrategy.OnPush,
    providers: [ConfirmationService],
    template: `
        <p-confirmdialog />

        <app-dialog-shell
            [title]="'funnels.manager.dialogTitle' | transloco"
            [visible]="visible()"
            (visibleChange)="onManagerVisibleChange($event)"
            width="880px"
            [secondaryLabel]="'common.actions.close' | transloco"
            [showPrimary]="false"
        >
            <p class="text-sm text-muted-color mb-4">{{ "funnels.manager.dialogDescription" | transloco }}</p>

            @if (successKey(); as key) {
                <p-message severity="success" styleClass="w-full mb-4" [text]="key | transloco" />
            } @else if (!isEditorDialogVisible() && errorKey(); as key) {
                <p-message severity="error" styleClass="w-full mb-4" [text]="key | transloco" />
            }

            <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between mb-3">
                <p-button [label]="'funnels.manager.newAction' | transloco" icon="pi pi-plus" (onClick)="openCreateDialog()" [outlined]="true" size="small" />
                <p-iconfield>
                    <p-inputicon class="pi pi-search" />
                    <input pInputText #funnelSearch [placeholder]="'common.searchPlaceholder' | transloco" (input)="funnelsTable.filterGlobal($any($event.target).value, 'contains')" class="w-full" />
                </p-iconfield>
            </div>

            <div class="hk-crud-table-wrap">
                <p-table #funnelsTable [value]="funnels()" [loading]="funnelsResource.isLoading()" styleClass="hk-crud-table p-datatable-sm" [rowHover]="true" [globalFilterFields]="['name']" [sortField]="'name'" [sortOrder]="1">
                    <ng-template pTemplate="header">
                        <tr>
                            <th pSortableColumn="name">
                                {{ "common.columns.name" | transloco }}
                                <p-sortIcon field="name" />
                            </th>
                            <th>{{ "funnels.manager.stepsLabel" | transloco }}</th>
                            <th class="hk-actions-column">{{ "common.columns.actions" | transloco }}</th>
                        </tr>
                    </ng-template>
                    <ng-template pTemplate="body" let-funnel>
                        <tr>
                            <td class="font-medium">{{ funnel.name }}</td>
                            <td>
                                <div class="flex items-center gap-1 flex-wrap">
                                    @for (step of funnel.steps; track $index; let last = $last) {
                                        <p-tag [value]="step.value" [severity]="step.type === 'event' ? 'info' : 'secondary'" [rounded]="true" [icon]="step.type === 'event' ? 'pi pi-bolt' : 'pi pi-link'" />
                                        @if (!last) {
                                            <i class="pi pi-arrow-right text-xs text-muted-color"></i>
                                        }
                                    }
                                </div>
                            </td>
                            <td class="hk-actions-cell">
                                <app-table-row-actions [items]="funnelActions(funnel)" [loading]="deletingFunnelId() === funnel.id" />
                            </td>
                        </tr>
                    </ng-template>
                    <ng-template pTemplate="emptymessage">
                        <tr>
                            <td colspan="3" class="text-center text-muted-color py-4">{{ "funnels.manager.empty" | transloco }}</td>
                        </tr>
                    </ng-template>
                </p-table>
            </div>
        </app-dialog-shell>

        <app-crud-dialog
            [title]="editorTitle()"
            [visible]="isEditorDialogVisible()"
            (visibleChange)="onEditorDialogVisibleChange($event)"
            [submitLabel]="primaryActionLabel()"
            [cancelLabel]="'common.actions.cancel' | transloco"
            [saving]="saving()"
            width="48rem"
            (submitted)="saveFunnel()"
        >
            <form class="flex flex-col gap-4" (ngSubmit)="saveFunnel()">
                <div class="flex flex-col gap-1">
                    <label for="f-name" class="text-xs font-medium">{{ "common.columns.name" | transloco }}</label>
                    <input pInputText id="f-name" [formControl]="newFunnelForm.name().control()" [placeholder]="'funnels.manager.namePlaceholder' | transloco" class="w-full" />
                </div>

                <div class="flex flex-col gap-1">
                    <span id="funnel-steps-label" class="text-xs font-medium mb-1">{{ "funnels.manager.stepsLabel" | transloco }}</span>

                    <div cdkDropList (cdkDropListDropped)="reorderStep($event)" class="flex flex-col gap-3">
                        @for (step of stepControls(); track $index; let i = $index) {
                            <div cdkDrag class="flex gap-2 items-center">
                                <i cdkDragHandle class="pi pi-bars text-muted-color cursor-grab shrink-0"></i>
                                <span
                                    class="flex w-7 h-7 shrink-0 items-center justify-center rounded-full text-xs font-bold"
                                    [class]="i === 0 ? 'bg-primary text-primary-contrast' : 'bg-surface-200 dark:bg-surface-700 text-surface-600 dark:text-surface-300'"
                                >
                                    {{ i + 1 }}
                                </span>
                                <p-selectbutton [options]="types()" [formControl]="step.typeControl" optionLabel="label" optionValue="value" size="small" ariaLabelledBy="funnel-steps-label" />
                                <p-inputgroup>
                                    <p-inputgroup-addon>
                                        <i [class]="step.typeControl.value === 'event' ? 'pi pi-bolt' : 'pi pi-link'"></i>
                                    </p-inputgroup-addon>
                                    <input pInputText [formControl]="step.valueControl" [placeholder]="step.typeControl.value === 'path' ? ('funnels.manager.stepPathPlaceholder' | transloco) : ('funnels.manager.stepEventPlaceholder' | transloco)" />
                                </p-inputgroup>
                                <p-button icon="pi pi-times" (onClick)="removeStep(i)" severity="danger" [text]="true" [rounded]="true" size="small" [disabled]="stepControls().length <= 2 || isBusy()" />
                                <div *cdkDragPlaceholder class="rounded-md border-2 border-dashed border-primary/30 bg-primary/5 h-10 w-full"></div>
                            </div>
                        }
                    </div>

                    <p-button [label]="'funnels.manager.addStep' | transloco" icon="pi pi-plus" (onClick)="addStep()" [text]="true" size="small" class="mt-1" [disabled]="isBusy()" />
                </div>

                @if (errorKey(); as key) {
                    <p-message severity="error" styleClass="w-full" [text]="key | transloco" />
                }
            </form>
        </app-crud-dialog>
    `
})
export class FunnelManager {
    visible = model(false);
    readonly siteId = input<string | null>(null);
    readonly editFunnelId = input<string | null>(null);
    readonly funnelsChanged = output();
    readonly viewFunnel = output<Funnel>();

    private analyticsService = inject(AnalyticsService);
    private transloco = inject(TranslocoService);
    private confirmationService = inject(ConfirmationService);
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    protected readonly saving = signal(false);
    protected readonly deletingFunnelId = signal<string | null>(null);
    protected readonly isEditorDialogVisible = signal(false);
    protected readonly editingFunnel = signal<Funnel | null>(null);
    protected readonly isBusy = computed(() => this.saving() || this.deletingFunnelId() !== null);
    protected readonly successKey = signal<string | null>(null);
    protected readonly errorKey = signal<string | null>(null);
    private readonly lastAppliedEditFunnelId = signal<string | null>(null);

    protected readonly funnelsResource = rxResource({
        params: () => {
            const siteId = this.siteId();
            return this.visible() && siteId ? { siteId } : undefined;
        },
        stream: ({ params }) => this.analyticsService.getFunnels(params.siteId)
    });

    protected readonly funnels = computed(() => this.funnelsResource.value() ?? []);

    protected readonly types = computed(() => {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('funnels.manager.typePagePath'), value: 'path' },
            { label: this.transloco.translate('funnels.manager.typeCustomEvent'), value: 'event' }
        ];
    });

    protected readonly editorTitle = computed(() => {
        this.activeLanguage();
        return this.editingFunnel() ? this.transloco.translate('funnels.manager.editTitle') : this.transloco.translate('funnels.manager.createTitle');
    });

    protected readonly primaryActionLabel = computed(() => {
        this.activeLanguage();
        return this.editingFunnel() ? this.transloco.translate('funnels.manager.saveAction') : this.transloco.translate('funnels.manager.createAction');
    });

    private readonly newFunnelModel = signal({
        name: new FormControl('', { nonNullable: true, validators: [Validators.required] })
    });
    protected readonly newFunnelForm = compatForm(this.newFunnelModel);
    protected readonly stepControls = signal<FunnelStepControl[]>([this.createStepControl(), this.createStepControl()]);

    protected readonly canSave = computed(() => {
        const name = this.newFunnelForm.name().value().trim();
        const steps = this.stepControls();
        return !this.isBusy() && !this.newFunnelForm().invalid() && name.length > 0 && steps.length >= 2 && steps.every((step) => step.valueControl.value.trim().length > 0);
    });

    constructor() {
        effect(() => {
            const editFunnelId = this.editFunnelId();
            if (!this.visible() || !editFunnelId || this.isBusy() || this.lastAppliedEditFunnelId() === editFunnelId) return;
            const funnel = this.funnels().find((item) => item.id === editFunnelId);
            if (!funnel) return;
            this.lastAppliedEditFunnelId.set(editFunnelId);
            this.editFunnel(funnel);
        });
    }

    protected openCreateDialog() {
        if (this.isBusy()) return;
        this.resetEditor();
        this.isEditorDialogVisible.set(true);
    }

    addStep() {
        if (this.isBusy()) return;
        this.stepControls.update((steps) => [...steps, this.createStepControl()]);
    }

    removeStep(index: number) {
        if (this.isBusy()) return;
        this.stepControls.update((steps) => (steps.length > 2 ? steps.filter((_, i) => i !== index) : steps));
    }

    reorderStep(event: CdkDragDrop<FunnelStepControl[]>) {
        if (this.isBusy()) return;
        this.stepControls.update((steps) => {
            const reordered = [...steps];
            moveItemInArray(reordered, event.previousIndex, event.currentIndex);
            return reordered;
        });
    }

    editFunnel(funnel: Funnel) {
        if (this.isBusy()) return;
        this.clearFeedback();
        this.editingFunnel.set(funnel);
        this.newFunnelForm.name().control().setValue(funnel.name);
        const steps = [...funnel.steps];
        while (steps.length < 2) {
            steps.push({ type: 'path', value: '' });
        }
        this.stepControls.set(steps.map((step) => this.createStepControl(step.type, step.value)));
        this.isEditorDialogVisible.set(true);
    }

    resetEditor() {
        this.clearFeedback();
        this.editingFunnel.set(null);
        this.newFunnelForm.name().control().reset('');
        this.stepControls.set([this.createStepControl(), this.createStepControl()]);
    }

    onHide() {
        this.visible.set(false);
        this.isEditorDialogVisible.set(false);
        this.resetEditor();
        this.lastAppliedEditFunnelId.set(null);
    }

    protected onManagerVisibleChange(visible: boolean) {
        if (visible) {
            this.visible.set(true);
            return;
        }
        this.onHide();
    }

    protected onEditorDialogVisibleChange(visible: boolean) {
        if (!visible && this.saving()) {
            this.isEditorDialogVisible.set(true);
            return;
        }
        this.isEditorDialogVisible.set(visible);
        if (!visible) {
            this.resetEditor();
        }
    }

    protected funnelActions(funnel: Funnel): TableRowActionItem[] {
        this.activeLanguage();
        return [
            {
                label: this.transloco.translate('funnels.manager.viewStats'),
                icon: 'pi pi-chart-bar',
                disabled: this.isBusy(),
                command: () => this.viewFunnel.emit(funnel)
            },
            {
                label: this.transloco.translate('funnels.manager.editTooltip'),
                icon: 'pi pi-pencil',
                disabled: this.isBusy(),
                command: () => this.editFunnel(funnel)
            },
            { separator: true },
            {
                label: this.transloco.translate('funnels.manager.deleteTooltip'),
                icon: 'pi pi-trash',
                danger: true,
                disabled: this.isBusy(),
                command: () => this.confirmDeleteFunnel(funnel)
            }
        ];
    }

    protected confirmDeleteFunnel(funnel: Funnel) {
        if (this.isBusy()) return;
        this.confirmationService.confirm({
            message: this.transloco.translate('funnels.manager.confirmDelete', { name: funnel.name }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('funnels.manager.deleteTooltip')),
            accept: () => this.deleteFunnel(funnel.id)
        });
    }

    saveFunnel() {
        const siteId = this.siteId();
        if (!siteId || !this.canSave()) return;
        const payload: { name: string; steps: FunnelStep[] } = {
            name: this.newFunnelForm.name().value().trim(),
            steps: this.stepControls().map((step) => ({
                type: step.typeControl.value,
                value: step.valueControl.value.trim()
            }))
        };
        const editingFunnel = this.editingFunnel();
        const action: FunnelManagerAction = editingFunnel ? 'update' : 'create';

        this.clearFeedback();
        this.saving.set(true);
        const request = editingFunnel ? this.analyticsService.createFunnel(siteId, payload).pipe(switchMap(() => this.analyticsService.deleteFunnel(siteId, editingFunnel.id))) : this.analyticsService.createFunnel(siteId, payload);

        request.pipe(finalize(() => this.saving.set(false))).subscribe({
            next: () => {
                this.resetEditor();
                this.isEditorDialogVisible.set(false);
                this.funnelsResource.reload();
                this.funnelsChanged.emit();
                this.setSuccess(action);
            },
            error: () => this.setError(action)
        });
    }

    deleteFunnel(id: string) {
        const siteId = this.siteId();
        if (!siteId || this.isBusy()) return;
        this.clearFeedback();
        this.deletingFunnelId.set(id);
        this.analyticsService
            .deleteFunnel(siteId, id)
            .pipe(finalize(() => this.deletingFunnelId.set(null)))
            .subscribe({
                next: () => {
                    if (this.editingFunnel()?.id === id) {
                        this.resetEditor();
                    }
                    this.funnelsResource.reload();
                    this.funnelsChanged.emit();
                    this.setSuccess('delete');
                },
                error: () => this.setError('delete')
            });
    }

    private createStepControl(type: FunnelStep['type'] = 'path', value = ''): FunnelStepControl {
        return {
            typeControl: new FormControl(type, { nonNullable: true, validators: [Validators.required] }),
            valueControl: new FormControl(value, { nonNullable: true, validators: [Validators.required] })
        };
    }

    private clearFeedback() {
        this.successKey.set(null);
        this.errorKey.set(null);
    }

    private setSuccess(action: FunnelManagerAction) {
        this.successKey.set(`funnels.manager.messages.${action}Success`);
        this.errorKey.set(null);
    }

    private setError(action: FunnelManagerAction) {
        this.errorKey.set(`funnels.manager.messages.${action}Error`);
        this.successKey.set(null);
    }
}
