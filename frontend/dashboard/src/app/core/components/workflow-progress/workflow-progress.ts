import { ChangeDetectionStrategy, Component, input } from '@angular/core';

export type WorkflowProgressStepState = 'complete' | 'current' | 'pending';

export interface WorkflowProgressStep {
    id: string;
    label: string;
    state: WorkflowProgressStepState;
}

@Component({
    selector: 'app-workflow-progress',
    templateUrl: './workflow-progress.html',
    styleUrl: './workflow-progress.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class WorkflowProgress {
    readonly steps = input.required<readonly WorkflowProgressStep[]>();
    readonly ariaLabel = input<string | null>(null);
}
