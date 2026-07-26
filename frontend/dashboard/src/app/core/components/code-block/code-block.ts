import { ChangeDetectionStrategy, Component, input } from '@angular/core';

import { CopyControl } from '@components/copy-control/copy-control';

/**
 * Read-only code sample with a copy affordance. Used wherever the dashboard hands the
 * operator a snippet to paste elsewhere (tracker snippet, install commands, curl calls).
 */
@Component({
    selector: 'app-code-block',
    standalone: true,
    imports: [CopyControl],
    templateUrl: './code-block.html',
    styleUrl: './code-block.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class CodeBlock {
    /** The code to display and copy. */
    readonly code = input.required<string>();
    /** Optional caption rendered above the code, already translated. */
    readonly label = input('');
    /** Wrap long lines instead of scrolling horizontally. */
    readonly wrap = input(false);
}
