import { ComponentFixture, TestBed } from '@angular/core/testing';

import { WorkflowProgress } from './workflow-progress';

describe('WorkflowProgress', () => {
    let fixture: ComponentFixture<WorkflowProgress>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [WorkflowProgress]
        }).compileComponents();

        fixture = TestBed.createComponent(WorkflowProgress);
        fixture.componentRef.setInput('ariaLabel', 'Import steps');
        fixture.componentRef.setInput('steps', [
            { id: 'driver', label: 'Choose importer', state: 'complete' },
            { id: 'files', label: 'Upload files', state: 'current' },
            { id: 'review', label: 'Review validation', state: 'pending' }
        ]);
        fixture.detectChanges();
    });

    it('renders a non-clickable progress list', () => {
        const element = fixture.nativeElement as HTMLElement;
        const list = element.querySelector('ol');
        const buttons = element.querySelectorAll('button, a, [role="button"]');

        expect(list?.getAttribute('aria-label')).toBe('Import steps');
        expect(buttons.length).toBe(0);
    });

    it('marks only the current item with aria-current step', () => {
        const currentItems = fixture.nativeElement.querySelectorAll('[aria-current="step"]') as NodeListOf<HTMLElement>;

        expect(currentItems.length).toBe(1);
        expect(currentItems[0]?.textContent).toContain('Upload files');
    });
});
