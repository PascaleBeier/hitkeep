import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { provideTranslocoLocale } from '@jsverse/transloco-locale';
import { Subject } from 'rxjs';
import { vi } from 'vitest';

import type { QRCode } from '@models/analytics.types';
import { QRCodesService } from '@services/qr-codes.service';
import { QRSharePage } from './qr-share';

interface QRShareTestAccess {
    qr(): QRCode | null;
}

describe('QRSharePage route inputs', () => {
    let fixture: ComponentFixture<QRSharePage>;
    let responses: Subject<QRCode>[];

    beforeEach(async () => {
        responses = [];
        await TestBed.configureTestingModule({
            imports: [
                QRSharePage,
                TranslocoTestingModule.forRoot({
                    langs: { en: { common: { loading: 'Loading' }, qrCodes: { share: { invalid: 'Invalid' } } } },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' }
                })
            ],
            providers: [
                provideTranslocoLocale({ langToLocaleMapping: { en: 'en-US' } }),
                {
                    provide: QRCodesService,
                    useValue: {
                        getQRShare: vi.fn(() => {
                            const response = new Subject<QRCode>();
                            responses.push(response);
                            return response.asObservable();
                        })
                    }
                }
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(QRSharePage);
    });

    it('cancels stale metadata and accepts only the changed path token response', () => {
        fixture.componentRef.setInput('token', 'old-token');
        fixture.detectChanges();

        fixture.componentRef.setInput('token', 'new-token');
        fixture.detectChanges();

        expect(responses[0].observed).toBe(false);
        expect(responses[1].observed).toBe(true);

        const stale = { id: 'qr-old' } as QRCode;
        const fresh = { id: 'qr-new' } as QRCode;
        responses[0].next(stale);
        responses[1].next(fresh);

        expect((fixture.componentInstance as unknown as QRShareTestAccess).qr()).toBe(fresh);
    });

    it('tears down the active metadata request with the component', () => {
        fixture.componentRef.setInput('token', 'active-token');
        fixture.detectChanges();
        expect(responses[0].observed).toBe(true);

        fixture.destroy();
        expect(responses[0].observed).toBe(false);
    });
});
