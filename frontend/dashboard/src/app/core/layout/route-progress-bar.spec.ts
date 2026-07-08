import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { vi } from 'vitest';
import { RouteProgressBar } from './route-progress-bar';

@Component({ template: '' })
class BlankPage {}

describe('RouteProgressBar', () => {
    let fixture: ComponentFixture<RouteProgressBar>;
    let router: Router;
    let releaseGuard: (allowed: boolean) => void;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [RouteProgressBar],
            providers: [
                provideRouter([
                    { path: 'fast', component: BlankPage },
                    {
                        path: 'slow',
                        component: BlankPage,
                        canActivate: [
                            () =>
                                new Promise<boolean>((resolve) => {
                                    releaseGuard = resolve;
                                })
                        ]
                    }
                ])
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(RouteProgressBar);
        router = TestBed.inject(Router);
        fixture.detectChanges();
        vi.useFakeTimers();
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    function bar(): HTMLElement {
        return fixture.nativeElement.querySelector('.hk-route-progress') as HTMLElement;
    }

    it('stays hidden for navigations that finish before the show delay', async () => {
        const navigation = router.navigateByUrl('/fast');
        fixture.detectChanges();
        await navigation;
        fixture.detectChanges();

        vi.advanceTimersByTime(1000);
        fixture.detectChanges();

        expect(bar().classList.contains('loading')).toBe(false);
        expect(bar().classList.contains('done')).toBe(false);
    });

    it('shows the bar for slow navigations and completes when navigation ends', async () => {
        const navigation = router.navigateByUrl('/slow');
        await vi.advanceTimersByTimeAsync(0);
        fixture.detectChanges();

        await vi.advanceTimersByTimeAsync(120);
        fixture.detectChanges();
        expect(bar().classList.contains('loading')).toBe(true);

        releaseGuard(true);
        await navigation;
        fixture.detectChanges();
        fixture.detectChanges();
        expect(bar().classList.contains('done')).toBe(true);
        expect(bar().classList.contains('loading')).toBe(false);

        vi.advanceTimersByTime(300);
        fixture.detectChanges();
        expect(bar().classList.contains('done')).toBe(false);
        expect(bar().classList.contains('loading')).toBe(false);
    });
});
