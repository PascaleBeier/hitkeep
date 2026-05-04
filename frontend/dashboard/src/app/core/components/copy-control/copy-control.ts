import { ChangeDetectionStrategy, Component, DestroyRef, computed, inject, input, signal } from '@angular/core';
import { CdkCopyToClipboard } from '@angular/cdk/clipboard';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from 'primeng/button';

type CopyStatus = 'idle' | 'copied' | 'failed';
type CopyButtonSize = 'small' | 'large';

@Component({
    selector: 'app-copy-control',
    standalone: true,
    imports: [ButtonModule, CdkCopyToClipboard, TranslocoPipe],
    templateUrl: './copy-control.html',
    styleUrl: './copy-control.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class CopyControl {
    private readonly destroyRef = inject(DestroyRef);
    private resetTimer: ReturnType<typeof setTimeout> | null = null;

    readonly value = input<string | null | undefined>('');
    readonly labelKey = input('common.copyControl.copy');
    readonly copiedLabelKey = input('common.copyControl.copied');
    readonly failedLabelKey = input('common.copyControl.failed');
    readonly ariaLabelKey = input('common.copyControl.ariaLabel');
    readonly disabled = input(false);
    readonly text = input(false);
    readonly rounded = input(false);
    readonly fluid = input(false);
    readonly size = input<CopyButtonSize | undefined>(undefined);
    readonly styleClass = input('');

    protected readonly status = signal<CopyStatus>('idle');
    protected readonly isDisabled = computed(() => this.disabled() || !this.value()?.trim());
    protected readonly copyValue = computed(() => (this.isDisabled() ? '' : (this.value() ?? '')));
    protected readonly icon = computed(() => {
        switch (this.status()) {
            case 'copied':
                return 'pi pi-check';
            case 'failed':
                return 'pi pi-exclamation-triangle';
            default:
                return 'pi pi-copy';
        }
    });
    protected readonly currentLabelKey = computed(() => {
        switch (this.status()) {
            case 'copied':
                return this.copiedLabelKey();
            case 'failed':
                return this.failedLabelKey();
            default:
                return this.labelKey();
        }
    });
    protected readonly severity = computed(() => {
        switch (this.status()) {
            case 'copied':
                return 'success';
            case 'failed':
                return 'danger';
            default:
                return undefined;
        }
    });
    protected readonly liveMessageKey = computed(() => (this.status() === 'idle' ? '' : this.currentLabelKey()));

    constructor() {
        this.destroyRef.onDestroy(() => this.clearResetTimer());
    }

    protected onCopied(successful: boolean): void {
        this.setStatus(successful ? 'copied' : 'failed');
    }

    private setStatus(status: CopyStatus): void {
        this.status.set(status);
        this.clearResetTimer();
        this.resetTimer = setTimeout(() => {
            this.status.set('idle');
            this.resetTimer = null;
        }, 2000);
    }

    private clearResetTimer(): void {
        if (!this.resetTimer) {
            return;
        }
        clearTimeout(this.resetTimer);
        this.resetTimer = null;
    }
}
