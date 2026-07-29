import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { GoalManager } from './goal-manager';
import { AnalyticsService } from '@services/analytics.service';
import { Goal } from '@models/analytics.types';

describe('GoalManager', () => {
    let fixture: ComponentFixture<GoalManager>;
    const goal: Goal = { id: 'goal-1', site_id: 'site-1', name: 'Signup', type: 'event', value: 'signup', created_at: '2026-01-01T00:00:00Z' };
    const analytics = { createGoal: vi.fn(() => of(void 0)), updateGoal: vi.fn(() => of(goal)) };

    beforeEach(async () => {
        vi.clearAllMocks();
        await TestBed.configureTestingModule({
            imports: [
                GoalManager,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: { actions: { cancel: 'Cancel' }, columns: { name: 'Name', type: 'Type' } },
                            goals: {
                                manager: {
                                    addTitle: 'Add goal',
                                    editTitle: 'Edit goal',
                                    createAction: 'Create',
                                    saveAction: 'Save',
                                    namePlaceholder: 'Name',
                                    typePagePath: 'Path',
                                    typeCustomEvent: 'Event',
                                    urlPathLabel: 'Path',
                                    eventNameLabel: 'Event',
                                    urlPathPlaceholder: '/done',
                                    eventNamePlaceholder: 'signup',
                                    suggestionsHelp: 'Suggestions',
                                    errors: { save: 'Save failed' }
                                }
                            }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }
                })
            ],
            providers: [{ provide: AnalyticsService, useValue: analytics }]
        }).compileComponents();
        fixture = TestBed.createComponent(GoalManager);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('visible', true);
        fixture.detectChanges();
        await fixture.whenStable();
    });

    afterEach(() => document.querySelectorAll('.p-dialog-mask, .p-dialog').forEach((element) => element.remove()));

    it('opens one CRUD dialog populated for an edit', async () => {
        fixture.componentRef.setInput('goal', goal);
        fixture.detectChanges();
        await fixture.whenStable();
        expect(document.body.textContent).toContain('Edit goal');
        expect((document.body.querySelector('#goal-name') as HTMLInputElement).value).toBe('Signup');
        expect(document.body.querySelectorAll('.p-dialog').length).toBe(1);
    });

    it('keeps a failed form open with local feedback', async () => {
        analytics.createGoal.mockReturnValueOnce(throwError(() => new Error('failed')));
        (document.body.querySelector('#goal-name') as HTMLInputElement).value = 'Signup';
        (document.body.querySelector('#goal-name') as HTMLInputElement).dispatchEvent(new Event('input'));
        const value = document.body.querySelector('#goal-value') as HTMLInputElement;
        value.value = '/done';
        value.dispatchEvent(new Event('input'));
        fixture.detectChanges();
        (fixture.componentInstance as unknown as { saveGoal: () => void }).saveGoal();
        fixture.detectChanges();
        await fixture.whenStable();
        expect(document.body.textContent).toContain('Save failed');
        expect(document.body.querySelector('.p-dialog')).not.toBeNull();
    });
});
