import { Injector, runInInjectionContext, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ReportSubjectService, injectSkeletonGate } from '@services/report-subject.service';

describe('injectSkeletonGate', () => {
    let injector: Injector;
    let subject: ReportSubjectService;

    beforeEach(() => {
        TestBed.configureTestingModule({});
        injector = TestBed.inject(Injector);
        subject = TestBed.inject(ReportSubjectService);
        subject.set('site-a');
    });

    function gateFor(loading: ReturnType<typeof signal<boolean>>) {
        return runInInjectionContext(injector, () => injectSkeletonGate(loading));
    }

    it('keeps painted content through later reloads of the same subject', () => {
        const loading = signal(true);
        const showSkeleton = gateFor(loading);

        expect(showSkeleton()).toBe(true);
        loading.set(false);
        expect(showSkeleton()).toBe(false);

        loading.set(true);
        expect(showSkeleton()).toBe(false);
    });

    // The latch only records states that were actually read, which is every
    // state in the app: change detection reads the gate on each pass.
    it('paints again after the new subject resolves', () => {
        const loading = signal(true);
        const showSkeleton = gateFor(loading);

        loading.set(false);
        expect(showSkeleton()).toBe(false);

        subject.set('site-b');
        loading.set(true);
        expect(showSkeleton()).toBe(true);

        loading.set(false);
        expect(showSkeleton()).toBe(false);

        loading.set(true);
        expect(showSkeleton()).toBe(false);
    });

    it('ignores an empty surface when content is reported', () => {
        const loading = signal(true);
        const hasContent = signal(false);
        const showSkeleton = runInInjectionContext(injector, () => injectSkeletonGate(loading, hasContent));

        loading.set(false);
        expect(showSkeleton()).toBe(false);

        // Nothing was ever painted, so the next load starts from the skeleton.
        loading.set(true);
        expect(showSkeleton()).toBe(true);

        loading.set(false);
        hasContent.set(true);
        expect(showSkeleton()).toBe(false);

        loading.set(true);
        expect(showSkeleton()).toBe(false);
    });
});
