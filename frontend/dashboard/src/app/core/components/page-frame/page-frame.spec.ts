import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';

import { PageFrame } from './page-frame';

@Component({
    imports: [PageFrame],
    template: `
        <app-page-frame subtitle="Scheduled delivery">
            <main>Report list</main>
        </app-page-frame>
    `
})
class PageFrameTestHost {}

@Component({
    imports: [PageFrame],
    template: `
        <app-page-frame>
            <main>Bare frame</main>
        </app-page-frame>
    `
})
class BareFrameHost {}

describe('PageFrame', () => {
    let fixture: ComponentFixture<PageFrameTestHost>;

    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [PageFrameTestHost]
        });
        fixture = TestBed.createComponent(PageFrameTestHost);
        fixture.detectChanges();
    });

    it('owns the bounded body and renders the subtitle as an intro paragraph', () => {
        expect(fixture.nativeElement.querySelector('.page-frame__body main')?.textContent).toContain('Report list');
        expect(fixture.nativeElement.querySelector('.page-frame__subtitle')?.textContent).toContain('Scheduled delivery');
        expect(fixture.nativeElement.querySelector('.page-frame__subtitle--intro')).toBeTruthy();
    });

    it('renders no subtitle paragraph when none is provided', () => {
        TestBed.resetTestingModule();
        TestBed.configureTestingModule({
            imports: [BareFrameHost]
        });
        const bareFixture = TestBed.createComponent(BareFrameHost);
        bareFixture.detectChanges();

        expect(bareFixture.nativeElement.querySelector('.page-frame__body main')?.textContent).toContain('Bare frame');
        expect(bareFixture.nativeElement.querySelector('.page-frame__subtitle')).toBeNull();
    });
});
