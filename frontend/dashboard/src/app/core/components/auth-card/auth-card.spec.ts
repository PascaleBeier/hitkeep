import { ComponentFixture, TestBed } from '@angular/core/testing';

import { AuthCard } from './auth-card';

describe('AuthCard', () => {
    let fixture: ComponentFixture<AuthCard>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({ imports: [AuthCard] }).compileComponents();
        fixture = TestBed.createComponent(AuthCard);
    });

    it('renders a scoped OptimusUI auth card', async () => {
        await fixture.whenStable();

        const card = fixture.nativeElement.querySelector('p-card.hk-auth-card');
        expect(card).toBeTruthy();
        expect(card.classList.contains('p-card')).toBe(true);
    });
});
