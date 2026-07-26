import { ComponentFixture, TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { CopyControl } from '@components/copy-control/copy-control';
import { CodeBlock } from './code-block';

describe('CodeBlock', () => {
    let fixture: ComponentFixture<CodeBlock>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                CodeBlock,
                TranslocoTestingModule.forRoot({
                    langs: { en: {} },
                    translocoConfig: { availableLangs: ['en'], defaultLang: 'en' },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(CodeBlock);
        fixture.componentRef.setInput('code', 'npm install @hitkeep/tracker');
        fixture.detectChanges();
    });

    it('renders the code verbatim', () => {
        const pre: HTMLPreElement = fixture.nativeElement.querySelector('.code-block-code');
        expect(pre.textContent).toBe('npm install @hitkeep/tracker');
    });

    it('hands the code to the shared copy control', () => {
        const copyControl = fixture.debugElement.query(By.directive(CopyControl));
        expect(copyControl).toBeTruthy();
        expect((copyControl.componentInstance as CopyControl).value()).toBe('npm install @hitkeep/tracker');
    });

    it('omits the label row content until a label is provided', () => {
        expect(fixture.nativeElement.querySelector('.code-block-label')).toBeNull();

        fixture.componentRef.setInput('label', 'Install the package');
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('.code-block-label')?.textContent?.trim()).toBe('Install the package');
    });

    it('wraps long lines only when asked', () => {
        const pre: HTMLPreElement = fixture.nativeElement.querySelector('.code-block-code');
        expect(pre.classList.contains('code-block-code--wrap')).toBe(false);

        fixture.componentRef.setInput('wrap', true);
        fixture.detectChanges();

        expect(pre.classList.contains('code-block-code--wrap')).toBe(true);
    });
});
