import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { ShareService } from '@services/share.service';
import { UtmHub } from './utm';

describe('UtmHub', () => {
    let fixture: ComponentFixture<UtmHub>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                UtmHub,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            nav: {
                                utm: 'UTM',
                                utmBuilder: 'UTM Builder',
                                qrCodes: 'QR codes'
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

        fixture = TestBed.createComponent(UtmHub);
        fixture.detectChanges();
    });

    it('renders Builder and QR codes tabs', () => {
        const text = fixture.nativeElement.textContent;
        expect(text).toContain('UTM Builder');
        expect(text).toContain('QR codes');
    });

    it('hides the Builder tab in share mode', () => {
        TestBed.inject(ShareService).setToken('share-token');
        fixture.detectChanges();

        const text = fixture.nativeElement.textContent;
        expect(text).not.toContain('UTM Builder');
        expect(text).toContain('QR codes');
    });
});
