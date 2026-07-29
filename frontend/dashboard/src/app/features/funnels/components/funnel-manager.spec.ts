import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { FunnelManager } from './funnel-manager';
import { AnalyticsService } from '@services/analytics.service';
import { Funnel } from '@models/analytics.types';

describe('FunnelManager', () => {
    let fixture: ComponentFixture<FunnelManager>;
    const funnel: Funnel = {
        id: 'funnel-1',
        site_id: 'site-1',
        name: 'Checkout',
        steps: [
            { type: 'path', value: '/cart' },
            { type: 'event', value: 'purchase' }
        ],
        created_at: '2026-01-01T00:00:00Z'
    };
    const analytics = { createFunnel: vi.fn(() => of(void 0)), updateFunnel: vi.fn(() => of(funnel)) };

    beforeEach(async () => {
        vi.clearAllMocks();
        await TestBed.configureTestingModule({
            imports: [
                FunnelManager,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: { actions: { cancel: 'Cancel' }, columns: { name: 'Name' } },
                            funnels: {
                                manager: {
                                    createTitle: 'Create funnel',
                                    editTitle: 'Edit funnel',
                                    createAction: 'Create',
                                    saveAction: 'Save',
                                    namePlaceholder: 'Name',
                                    stepsTitle: 'Steps',
                                    reorderHelp: 'Reorder',
                                    addStep: 'Add step',
                                    typePagePath: 'Path',
                                    typeCustomEvent: 'Event',
                                    dragStepAria: 'Drag {{index}}',
                                    stepValueAria: 'Value {{index}}',
                                    moveUpAria: 'Move up',
                                    moveDownAria: 'Move down',
                                    removeStepAria: 'Remove',
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
        fixture = TestBed.createComponent(FunnelManager);
        fixture.componentRef.setInput('siteId', 'site-1');
        fixture.componentRef.setInput('visible', true);
        fixture.detectChanges();
        await fixture.whenStable();
    });

    afterEach(() => document.querySelectorAll('.p-dialog-mask, .p-dialog').forEach((element) => element.remove()));

    it('opens one CRUD dialog populated with ordered steps for an edit', async () => {
        fixture.componentRef.setInput('funnel', funnel);
        fixture.detectChanges();
        await fixture.whenStable();
        expect(document.body.textContent).toContain('Edit funnel');
        expect((document.body.querySelector('#funnel-name') as HTMLInputElement).value).toBe('Checkout');
        expect(document.body.querySelectorAll('.p-dialog').length).toBe(1);
        expect(document.body.querySelectorAll('.cdk-drag').length).toBe(2);
    });

    it('keeps a failed form open with local feedback', async () => {
        analytics.createFunnel.mockReturnValueOnce(throwError(() => new Error('failed')));
        (fixture.componentInstance as unknown as { nameControl: { setValue: (value: string) => void }; steps: () => { value: { setValue: (value: string) => void } }[]; saveFunnel: () => void }).nameControl.setValue('Checkout');
        const component = fixture.componentInstance as unknown as { steps: () => { value: { setValue: (value: string) => void } }[]; saveFunnel: () => void };
        component.steps().forEach((step, index) => step.value.setValue(index ? 'purchase' : '/cart'));
        component.saveFunnel();
        fixture.detectChanges();
        await fixture.whenStable();
        expect(document.body.textContent).toContain('Save failed');
        expect(document.body.querySelector('.p-dialog')).not.toBeNull();
    });
});
