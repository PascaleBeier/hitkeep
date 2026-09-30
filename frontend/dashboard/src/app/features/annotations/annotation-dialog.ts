import { ChangeDetectionStrategy, Component, computed, inject, linkedSignal, signal } from '@angular/core';
import { FormField, form, maxLength, validate } from '@angular/forms/signals';
import { TranslocoPipe } from '@jsverse/transloco';
import { TranslocoLocaleService } from '@jsverse/transloco-locale';
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { formatAnnotationWhen, fromDateTimeInputs, toDateTimeInputs } from '@features/annotations/annotation-dates';
import { SiteAnnotationsService, type AnnotationDraft } from '@features/annotations/site-annotations.service';

export const ANNOTATION_BODY_LIMIT = 280;

interface AnnotationFormModel {
    body: string;
    startDate: string;
    startTime: string;
    endDate: string;
    endTime: string;
}

function toFormModel(draft: AnnotationDraft | null): AnnotationFormModel {
    if (!draft) {
        return { body: '', startDate: '', startTime: '', endDate: '', endTime: '' };
    }
    const start = toDateTimeInputs(draft.startsAt, draft.granularity);
    const end = draft.endsAt ? toDateTimeInputs(draft.endsAt, draft.granularity) : { date: '', time: '' };
    return { body: draft.body, startDate: start.date, startTime: start.time, endDate: end.date, endTime: end.time };
}

/** The one note editor for every chart; opened through SiteAnnotationsService. */
@Component({
    selector: 'app-annotation-dialog',
    changeDetection: ChangeDetectionStrategy.OnPush,
    // The note field borrows the input styles without an OptimusUI directive: pTextarea throws under Signal Forms,
    // and pInputText would show the empty field as invalid before the reader has typed anything.
    imports: [DialogShell, FormField, ButtonModule, InputTextModule, TranslocoPipe],
    template: `
        <app-dialog-shell
            [title]="title() | transloco"
            [visible]="draft() !== null"
            (visibleChange)="$event || close()"
            [secondaryLabel]="(canWrite() ? 'common.actions.cancel' : 'common.actions.close') | transloco"
            [primaryLabel]="'common.actions.save' | transloco"
            [showPrimary]="canWrite()"
            [primaryDisabled]="noteForm().invalid()"
            [primaryLoading]="saving()"
            [busy]="saving() || deleting()"
            width="30rem"
            (primaryAction)="save()"
        >
            @if (canWrite()) {
                <form class="flex flex-col gap-5 pt-1" novalidate (submit)="save($event)">
                    <div class="flex flex-col gap-2">
                        <label for="annotation-body" class="text-sm font-semibold text-[var(--p-text-color)]">{{ 'annotations.dialog.bodyLabel' | transloco }}</label>
                        <textarea
                            id="annotation-body"
                            rows="3"
                            class="p-inputtext p-component w-full resize-none"
                            [class.p-invalid]="showBodyError()"
                            [formField]="noteForm.body"
                            [placeholder]="'annotations.dialog.bodyPlaceholder' | transloco"
                            [attr.aria-describedby]="showBodyError() ? null : 'annotation-body-hint'"
                            [attr.aria-invalid]="showBodyError() || null"
                            (keydown.control.enter)="save($event)"
                            (keydown.meta.enter)="save($event)"
                        ></textarea>
                        <div class="flex justify-between gap-3 text-xs text-[var(--p-text-muted-color)]">
                            @if (showBodyError()) {
                                <span class="text-[var(--p-red-500)]" role="alert">{{ 'annotations.dialog.errors.bodyRequired' | transloco }}</span>
                            } @else {
                                <span id="annotation-body-hint">{{ 'annotations.dialog.bodyHint' | transloco }}</span>
                            }
                            <span class="tabular-nums" [attr.aria-live]="remaining() <= 20 ? 'polite' : null">{{ 'annotations.dialog.remaining' | transloco: { count: remaining() } }}</span>
                        </div>
                    </div>

                    <fieldset class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                        <legend class="sr-only">{{ 'annotations.dialog.whenLegend' | transloco }}</legend>
                        <div class="flex flex-col gap-2">
                            <label for="annotation-start-date" class="text-sm font-semibold text-[var(--p-text-color)]">{{ 'annotations.dialog.startLabel' | transloco }}</label>
                            <input pInputText id="annotation-start-date" type="date" class="w-full" [formField]="noteForm.startDate" />
                            @if (hourly()) {
                                <input pInputText type="time" class="w-full" [formField]="noteForm.startTime" [attr.aria-label]="'annotations.dialog.startTimeLabel' | transloco" />
                            }
                        </div>
                        <div class="flex flex-col gap-2">
                            <label for="annotation-end-date" class="text-sm font-semibold text-[var(--p-text-color)]">
                                {{ 'annotations.dialog.endLabel' | transloco }}
                                <span class="font-normal text-[var(--p-text-muted-color)]">{{ 'annotations.dialog.optional' | transloco }}</span>
                            </label>
                            <input
                                pInputText
                                id="annotation-end-date"
                                type="date"
                                class="w-full"
                                [formField]="noteForm.endDate"
                                [attr.aria-invalid]="endBeforeStart() || null"
                                [attr.aria-describedby]="endBeforeStart() ? 'annotation-end-error' : null"
                            />
                            @if (hourly()) {
                                <input pInputText type="time" class="w-full" [formField]="noteForm.endTime" [attr.aria-label]="'annotations.dialog.endTimeLabel' | transloco" />
                            }
                            @if (endBeforeStart()) {
                                <small id="annotation-end-error" class="text-[var(--p-red-500)]" role="alert">{{ 'annotations.dialog.errors.endBeforeStart' | transloco }}</small>
                            }
                        </div>
                    </fieldset>

                    @if (error()) {
                        <p class="m-0 text-sm text-[var(--p-red-500)]" role="alert">{{ error()! | transloco }}</p>
                    }

                    @if (draft()?.id) {
                        <div class="flex items-center gap-2 border-t border-[var(--p-content-border-color)] pt-4">
                            <p-button
                                type="button"
                                severity="danger"
                                [text]="!confirmingDelete()"
                                size="small"
                                icon="pi pi-trash"
                                [label]="(confirmingDelete() ? 'annotations.dialog.confirmDelete' : 'annotations.dialog.delete') | transloco"
                                [loading]="deleting()"
                                (onClick)="remove()"
                            />
                            @if (confirmingDelete()) {
                                <p-button type="button" [text]="true" severity="secondary" size="small" [label]="'common.actions.cancel' | transloco" (onClick)="confirmingDelete.set(false)" />
                            }
                        </div>
                    }
                </form>
            } @else if (draft(); as note) {
                <div class="flex flex-col gap-2 pt-1">
                    <p class="m-0 text-xs font-medium uppercase tracking-wide text-[var(--p-text-muted-color)]">{{ readOnlyWhen() }}</p>
                    <p class="m-0 whitespace-pre-line break-words text-[var(--p-text-color)]">{{ note.body }}</p>
                </div>
            }
        </app-dialog-shell>
    `
})
export class AnnotationDialog {
    private readonly annotations = inject(SiteAnnotationsService);
    private readonly locale = inject(TranslocoLocaleService);

    protected readonly draft = this.annotations.draft;
    protected readonly canWrite = this.annotations.canWrite;
    protected readonly hourly = computed(() => this.draft()?.granularity === 'hour');
    protected readonly saving = signal(false);
    protected readonly deleting = signal(false);
    protected readonly error = signal<string | null>(null);
    protected readonly confirmingDelete = linkedSignal({ source: this.draft, computation: () => false });

    private readonly model = linkedSignal<AnnotationFormModel>(() => toFormModel(this.draft()));
    protected readonly noteForm = form(this.model, (note) => {
        validate(note.body, ({ value }) => (value().trim() ? null : { kind: 'required' }));
        maxLength(note.body, ANNOTATION_BODY_LIMIT);
        validate(note.startDate, ({ value }) => (value() ? null : { kind: 'required' }));
        validate(note.endDate, ({ value, valueOf }) => {
            const end = value();
            if (!end) {
                return null;
            }
            // ISO date and 24h time strings order the same way as the instants they name.
            const startKey = `${valueOf(note.startDate)}T${valueOf(note.startTime) || '00:00'}`;
            return `${end}T${valueOf(note.endTime) || '00:00'}` < startKey ? { kind: 'endBeforeStart' } : null;
        });
    });

    protected readonly title = computed(() => {
        const draft = this.draft();
        if (!this.canWrite()) {
            return 'annotations.dialog.viewTitle';
        }
        return draft?.id ? 'annotations.dialog.editTitle' : 'annotations.dialog.addTitle';
    });
    protected readonly remaining = computed(() => ANNOTATION_BODY_LIMIT - this.model().body.length);
    protected readonly showBodyError = computed(() => this.noteForm.body().touched() && this.noteForm.body().invalid());
    protected readonly endBeforeStart = computed(() =>
        this.noteForm
            .endDate()
            .errors()
            .some((e) => e.kind === 'endBeforeStart')
    );
    protected readonly readOnlyWhen = computed(() => {
        const draft = this.draft();
        if (!draft) {
            return '';
        }
        return formatAnnotationWhen({ starts_at: draft.startsAt.toISOString(), ends_at: draft.endsAt?.toISOString() }, draft.granularity, this.locale.getLocale());
    });

    protected close(): void {
        this.error.set(null);
        this.annotations.close();
    }

    protected async save(event?: Event): Promise<void> {
        event?.preventDefault();
        const draft = this.draft();
        if (!draft || !this.canWrite() || this.saving()) {
            return;
        }
        if (this.noteForm().invalid()) {
            this.noteForm.body().markAsTouched();
            return;
        }
        const value = this.model();
        const startsAt = fromDateTimeInputs({ date: value.startDate, time: value.startTime }, draft.granularity);
        const endsAt = value.endDate ? fromDateTimeInputs({ date: value.endDate, time: value.endTime }, draft.granularity) : null;
        this.saving.set(true);
        this.error.set(null);
        try {
            await this.annotations.save(draft.id, {
                starts_at: startsAt.toISOString(),
                ends_at: endsAt && endsAt.getTime() !== startsAt.getTime() ? endsAt.toISOString() : undefined,
                body: value.body.trim()
            });
            this.annotations.close();
        } catch {
            this.error.set('annotations.dialog.errors.saveFailed');
        } finally {
            this.saving.set(false);
        }
    }

    protected async remove(): Promise<void> {
        const id = this.draft()?.id;
        if (!id) {
            return;
        }
        if (!this.confirmingDelete()) {
            this.confirmingDelete.set(true);
            return;
        }
        this.deleting.set(true);
        this.error.set(null);
        try {
            await this.annotations.remove(id);
            this.annotations.close();
        } catch {
            this.error.set('annotations.dialog.errors.deleteFailed');
        } finally {
            this.deleting.set(false);
        }
    }
}
