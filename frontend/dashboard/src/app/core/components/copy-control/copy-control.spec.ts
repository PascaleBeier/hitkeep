import { ComponentFixture, TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';
import { vi } from 'vitest';

import { CopyControl } from './copy-control';

describe('CopyControl', () => {
    let fixture: ComponentFixture<CopyControl>;
    let originalExecCommand: typeof document.execCommand | undefined;
    let copiedText: string | null;
    const execCommand = vi.fn(() => true);

    beforeEach(async () => {
        originalExecCommand = document.execCommand;
        copiedText = null;
        execCommand.mockReset();
        execCommand.mockImplementation(() => {
            copiedText = document.querySelector('textarea')?.value ?? null;
            return true;
        });
        Object.defineProperty(document, 'execCommand', {
            configurable: true,
            value: execCommand
        });

        await TestBed.configureTestingModule({
            imports: [
                CopyControl,
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            common: {
                                copyControl: {
                                    copy: 'Copy',
                                    copied: 'Copied',
                                    failed: 'Copy failed',
                                    ariaLabel: 'Copy to clipboard'
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
            ]
        }).compileComponents();

        fixture = TestBed.createComponent(CopyControl);
        fixture.componentRef.setInput('value', 'resource-123');
        fixture.detectChanges();
    });

    afterEach(() => {
        vi.useRealTimers();
        Object.defineProperty(document, 'execCommand', {
            configurable: true,
            value: originalExecCommand
        });
    });

    it('copies the provided value through Angular CDK', () => {
        clickCopyButton();

        expect(execCommand).toHaveBeenCalled();
        expect(copiedText).toBe('resource-123');
    });

    it('shows copied feedback and resets', () => {
        vi.useFakeTimers();

        clickCopyButton();
        fixture.detectChanges();
        expect(fixture.nativeElement.textContent).toContain('Copied');

        vi.advanceTimersByTime(2000);
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Copy');
        expect(fixture.nativeElement.textContent).not.toContain('Copied');
    });

    it('does not require the native clipboard API', () => {
        const originalClipboard = navigator.clipboard;
        Object.defineProperty(navigator, 'clipboard', {
            configurable: true,
            value: undefined
        });

        clickCopyButton();
        fixture.detectChanges();

        expect(copiedText).toBe('resource-123');
        expect(fixture.nativeElement.textContent).toContain('Copied');

        Object.defineProperty(navigator, 'clipboard', {
            configurable: true,
            value: originalClipboard
        });
    });

    it('shows failure feedback when CDK reports copy failure', () => {
        execCommand.mockImplementationOnce(() => false);

        clickCopyButton();
        fixture.detectChanges();

        expect(fixture.nativeElement.textContent).toContain('Copy failed');
    });

    it('disables when value is empty', () => {
        fixture.componentRef.setInput('value', '');
        fixture.detectChanges();

        expect(copyButton().disabled).toBe(true);
    });

    function clickCopyButton(): void {
        copyButton().click();
        fixture.detectChanges();
    }

    function copyButton(): HTMLButtonElement {
        return fixture.nativeElement.querySelector('button');
    }
});
