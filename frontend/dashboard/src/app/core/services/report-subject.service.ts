import { Injectable, Signal, computed, inject, linkedSignal, signal } from '@angular/core';

/**
 * Identity of what the analytics surfaces are currently reporting on. A date
 * range, a filter or a selected metric all describe the same subject, so their
 * reloads may keep the previous values on screen. A different site is a
 * different subject and has to start over from an empty surface.
 */
@Injectable({ providedIn: 'root' })
export class ReportSubjectService {
    private readonly current = signal('');

    /** Empty while no site is active, for example on public share pages. */
    readonly subject = this.current.asReadonly();

    set(subject: string | null): void {
        this.current.set(subject ?? '');
    }
}

/**
 * A skeleton answers "there is nothing here yet", not "a request is running".
 * Once a surface has painted content for the current subject, later reloads
 * keep that content on screen so numbers and charts can animate from the old
 * values to the new ones instead of blinking through a placeholder.
 *
 * Call from an injection context and render the skeleton on the returned
 * signal instead of the raw loading flag. Pass `hasContent` wherever the
 * surface can tell real content from an empty state: an empty surface has
 * nothing worth holding on to, so it goes back to the skeleton.
 */
export function injectSkeletonGate(loading: Signal<boolean>, hasContent?: () => boolean): Signal<boolean> {
    const subject = inject(ReportSubjectService).subject;

    // A linked signal rather than an effect: the reset has to be visible in the
    // same change detection pass that flips `loading`, or a site switch paints
    // one frame of the previous site's numbers before the skeleton appears.
    const painted = linkedSignal({
        source: () => ({ subject: subject(), loading: loading(), hasContent: hasContent?.() ?? true }),
        computation: (source, previous) => {
            const keptFromLastPass = previous?.value === true && previous.source.subject === source.subject;
            return keptFromLastPass || (!source.loading && source.hasContent);
        }
    });

    return computed(() => {
        // Read the latch first: short-circuiting past it while loading is false
        // would never let it record that content reached the screen.
        const hasPaintedContent = painted();
        return loading() && !hasPaintedContent;
    });
}
