import { NgTemplateOutlet } from '@angular/common';
import { Component, TemplateRef, inject, signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';

import { MainLayoutContextService } from '@layout/main-layout-context.service';
import { PageFrame } from './page-frame';

@Component({
    imports: [NgTemplateOutlet, PageFrame],
    template: `
        <app-page-frame [breadcrumbItems]="[{ label: 'Reports', isCurrent: true }]" subtitle="Scheduled delivery">
            <main>Report list</main>
        </app-page-frame>
        @if (context.pageHeaderLeft(); as headerLeft) {
            <ng-container [ngTemplateOutlet]="headerLeft" />
        }
        @if (context.pageHeaderRight(); as headerRight) {
            <ng-container [ngTemplateOutlet]="headerRight" />
        }
    `
})
class PageFrameTestHost {
    protected readonly context = inject(MainLayoutContextService);
}

describe('PageFrame', () => {
    let fixture: ComponentFixture<PageFrameTestHost>;
    const pageHeaderLeft = signal<TemplateRef<unknown> | null>(null);
    const pageHeaderRight = signal<TemplateRef<unknown> | null>(null);
    const context = {
        pageHeaderLeft,
        pageHeaderRight,
        registerPageHeader: (_owner: symbol, left: TemplateRef<unknown> | null, right: TemplateRef<unknown> | null) => {
            pageHeaderLeft.set(left);
            pageHeaderRight.set(right);
        },
        clearPageHeader: () => {
            pageHeaderLeft.set(null);
            pageHeaderRight.set(null);
        }
    };

    beforeEach(() => {
        TestBed.configureTestingModule({
            imports: [PageFrameTestHost],
            providers: [{ provide: MainLayoutContextService, useValue: context }]
        });
        fixture = TestBed.createComponent(PageFrameTestHost);
        fixture.detectChanges();
    });

    it('owns the bounded body while registering the shared heading', () => {
        expect(fixture.nativeElement.querySelector('.page-frame__body main')?.textContent).toContain('Report list');
        expect(pageHeaderLeft()).not.toBeNull();
        expect(pageHeaderRight()).toBeNull();
        expect(fixture.nativeElement.querySelector('.page-frame__subtitle')?.textContent).toContain('Scheduled delivery');
    });
});
