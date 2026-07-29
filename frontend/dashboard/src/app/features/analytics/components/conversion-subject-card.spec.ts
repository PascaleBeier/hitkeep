import { FormControl } from '@angular/forms';
import { By } from '@angular/platform-browser';
import { TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { ConversionSubjectCard } from './conversion-subject-card';

describe('ConversionSubjectCard', () => {
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                ConversionSubjectCard,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            subject: { label: 'Reporting subject' }
                        }
                    },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }
                })
            ]
        }).compileComponents();
    });

    it('groups report context and subject selection in one card', () => {
        const fixture = TestBed.createComponent(ConversionSubjectCard);
        fixture.componentRef.setInput('labelKey', 'subject.label');
        fixture.componentRef.setInput('description', '4 steps · / → /pricing → /signup → trial_started');
        fixture.componentRef.setInput('options', [
            { label: 'All funnels', value: null },
            { label: 'Acquisition Funnel', value: 'funnel-1' }
        ]);
        fixture.componentRef.setInput('control', new FormControl<string | null>('funnel-1'));
        fixture.componentRef.setInput('controlId', 'funnel-subject');
        fixture.componentRef.setInput('headingId', 'funnel-subject-heading');
        fixture.detectChanges();

        expect(fixture.debugElement.queryAll(By.css('p-card')).length).toBe(1);
        expect(fixture.nativeElement.textContent).toContain('Reporting subject');
        expect(fixture.nativeElement.textContent).toContain('4 steps · / → /pricing → /signup → trial_started');

        let selected: string | null | undefined;
        fixture.componentInstance.subjectChanged.subscribe((value) => (selected = value));
        fixture.debugElement.query(By.css('p-select')).triggerEventHandler('onChange', { value: null });

        expect(selected).toBeNull();
        expect(fixture.debugElement.query(By.css('p-button'))).toBeNull();
    });
});
