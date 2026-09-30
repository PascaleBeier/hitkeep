import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { vi } from 'vitest';
import { AnnotationDialog } from '@features/annotations/annotation-dialog';
import { SiteAnnotationsService, type AnnotationDraft } from '@features/annotations/site-annotations.service';

describe('AnnotationDialog', () => {
    let fixture: ComponentFixture<AnnotationDialog>;
    let notes: { draft: ReturnType<typeof signal<AnnotationDraft | null>>; canWrite: ReturnType<typeof signal<boolean>>; save: ReturnType<typeof vi.fn>; remove: ReturnType<typeof vi.fn>; close: ReturnType<typeof vi.fn> };

    const dialog = () => fixture.componentInstance as unknown as { save: () => Promise<void>; remove: () => Promise<void>; noteForm: { body: () => { value: { set: (v: string) => void } } } };
    const body = () => document.body.querySelector<HTMLTextAreaElement>('#annotation-body');

    beforeEach(async () => {
        notes = {
            draft: signal<AnnotationDraft | null>(null),
            canWrite: signal(true),
            save: vi.fn().mockResolvedValue(undefined),
            remove: vi.fn().mockResolvedValue(undefined),
            close: vi.fn(() => notes.draft.set(null))
        };
        await TestBed.configureTestingModule({
            imports: [AnnotationDialog, TranslocoTestingModule.forRoot({ langs: { en: {} }, translocoConfig: { availableLangs: ['en'], defaultLang: 'en' } })],
            providers: [{ provide: SiteAnnotationsService, useValue: notes }, provideTranslocoLocale({ defaultLocale: 'en-US', langToLocaleMapping: { en: 'en-US' } })]
        }).compileComponents();
        fixture = TestBed.createComponent(AnnotationDialog);
        fixture.detectChanges();
    });

    function open(draft: Partial<AnnotationDraft> = {}) {
        notes.draft.set({ startsAt: new Date('2026-09-10T00:00:00Z'), endsAt: null, body: '', granularity: 'day', ...draft });
        fixture.detectChanges();
    }

    it('refuses an empty note without calling the API', async () => {
        open({ body: '   ' });
        await dialog().save();
        expect(notes.save).not.toHaveBeenCalled();
    });

    it('saves a day range as UTC bucket starts and closes', async () => {
        open({ body: ' Campaign ', endsAt: new Date('2026-09-14T00:00:00Z') });
        await dialog().save();
        expect(notes.save).toHaveBeenCalledWith(undefined, { starts_at: '2026-09-10T00:00:00.000Z', ends_at: '2026-09-14T00:00:00.000Z', body: 'Campaign' });
        expect(notes.close).toHaveBeenCalled();
    });

    it('keeps the dialog open with an error when saving fails', async () => {
        notes.save.mockRejectedValueOnce(new Error('boom'));
        open({ body: 'Launch' });
        await dialog().save();
        fixture.detectChanges();
        expect(notes.close).not.toHaveBeenCalled();
        expect(document.body.textContent).toContain('annotations.dialog.errors.saveFailed');
    });

    it('asks for a second click before deleting', async () => {
        open({ id: 'n1', body: 'Launch' });
        await dialog().remove();
        expect(notes.remove).not.toHaveBeenCalled();
        await dialog().remove();
        expect(notes.remove).toHaveBeenCalledWith('n1');
    });

    it('shows the note read-only to people who cannot write', () => {
        notes.canWrite.set(false);
        open({ id: 'n1', body: 'Outage' });
        expect(body()).toBeNull();
        expect(document.body.textContent).toContain('Outage');
    });
});
