import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';
import { ApiClientsHub } from './api-clients-hub';

interface ApiClientsHubAccess {
    onTabChange(value: string | number | undefined): void;
}

describe('ApiClientsHub', () => {
    let fixture: ComponentFixture<ApiClientsHub>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                ApiClientsHub,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: {
                                apiClients: 'API'
                            },
                            integration: {
                                apiClients: {
                                    tabs: {
                                        clients: 'Clients',
                                        reference: 'Reference'
                                    }
                                }
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ],
            providers: [provideRouter([])]
        }).compileComponents();

        fixture = TestBed.createComponent(ApiClientsHub);
        fixture.detectChanges();
    });

    it('renders Clients and Reference tabs', () => {
        const text = fixture.nativeElement.textContent;
        expect(text).toContain('Clients');
        expect(text).toContain('Reference');
    });

    it('navigates to the selected tab route', () => {
        const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
        const component = fixture.componentInstance as unknown as ApiClientsHubAccess;

        component.onTabChange('reference');

        expect(navigate).toHaveBeenCalledWith(['/integration/api-clients', 'reference']);
    });
});
