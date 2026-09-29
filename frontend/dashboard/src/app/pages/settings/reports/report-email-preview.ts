import { ChangeDetectionStrategy, Component, ElementRef, computed, inject, input, model, output, signal, viewChild } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { MessageModule } from '@openng/optimus-ui/message';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { injectActiveLang } from '@core/i18n/active-lang';
import { ReportPreview, ReportRecipientKind } from '@models/analytics.types';

type PreviewDevice = 'desktop' | 'mobile';
type PreviewTheme = 'light' | 'dark';
type PreviewView = 'email' | 'text';

const DEVICE_WIDTHS: Record<PreviewDevice, number> = { desktop: 600, mobile: 375 };
const DARK_QUERY = /@media\s*\(\s*prefers-color-scheme\s*:\s*dark\s*\)/g;

/**
 * Shows the exact email a report would send: an inbox-style envelope and the
 * rendered message in a sandboxed frame with device, theme, and audience
 * toggles. The HTML comes from the server's mail renderer, never from input.
 */
@Component({
    selector: 'app-report-email-preview',
    imports: [NgTemplateOutlet, ButtonModule, MessageModule, SkeletonModule, TranslocoPipe],
    templateUrl: './report-email-preview.html',
    styleUrl: './report-email-preview.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ReportEmailPreview {
    private readonly sanitizer = inject(DomSanitizer);
    private readonly activeLanguage = injectActiveLang();

    readonly preview = input<ReportPreview | null>(null);
    readonly loading = input(false);
    readonly stale = input(false);
    readonly hasExternalRecipients = input(false);
    readonly audience = model<ReportRecipientKind>('member');
    readonly refresh = output<void>();

    protected readonly device = signal<PreviewDevice>('desktop');
    protected readonly theme = signal<PreviewTheme>('light');
    protected readonly view = signal<PreviewView>('email');
    protected readonly frameHeight = signal(480);
    // The large view renders its own frame only while open.
    protected readonly expandedOpen = signal(false);
    protected readonly expanded = viewChild<ElementRef<HTMLDialogElement>>('expandedDialog');

    protected readonly frameWidth = computed(() => DEVICE_WIDTHS[this.device()]);
    protected readonly hasEmail = computed(() => !!this.preview()?.html);
    protected readonly frameDocument = computed<SafeHtml | null>(() => {
        const html = this.preview()?.html;
        if (!html) return null;
        // Force the chosen scheme regardless of the viewer's OS, and keep link
        // clicks inside the sandbox inert (no popups are allowed).
        const themed = html.replace(DARK_QUERY, this.theme() === 'dark' ? '@media all' : '@media not all').replace(/<head>/i, '<head><base target="_blank">');
        // Server-rendered mail template output, isolated by a script-less sandbox.
        return this.sanitizer.bypassSecurityTrustHtml(themed);
    });
    protected readonly sender = computed(() => {
        const preview = this.preview();
        if (!preview?.from_address) return '';
        return preview.from_name ? `${preview.from_name} <${preview.from_address}>` : preview.from_address;
    });
    protected readonly arrival = computed(() => {
        const preview = this.preview();
        if (!preview?.scheduled_for) return '';
        const format = new Intl.DateTimeFormat(this.activeLanguage(), {
            weekday: 'short',
            day: 'numeric',
            month: 'short',
            hour: '2-digit',
            minute: '2-digit',
            timeZone: preview.schedule.timezone
        });
        return format.format(new Date(preview.scheduled_for));
    });
    protected readonly period = computed(() => {
        const preview = this.preview();
        if (!preview) return '';
        const format = new Intl.DateTimeFormat(this.activeLanguage(), { day: 'numeric', month: 'short', year: 'numeric', timeZone: preview.schedule.timezone });
        // period_end is exclusive; show the last covered day.
        return format.formatRange(new Date(preview.period_start), new Date(new Date(preview.period_end).getTime() - 1));
    });

    protected setDevice(device: PreviewDevice): void {
        this.device.set(device);
    }

    protected setTheme(theme: PreviewTheme): void {
        this.theme.set(theme);
    }

    protected setView(view: PreviewView): void {
        this.view.set(view);
    }

    protected setAudience(audience: ReportRecipientKind): void {
        this.audience.set(audience);
    }

    protected fitFrame(frame: HTMLIFrameElement): void {
        // allow-same-origin (without allow-scripts) lets the host measure the
        // document so the whole email is visible without a nested scrollbar.
        const height = frame.contentDocument?.documentElement.scrollHeight;
        if (height) this.frameHeight.set(height);
    }

    protected openExpanded(): void {
        this.expandedOpen.set(true);
        this.expanded()?.nativeElement.showModal();
    }

    protected closeExpanded(): void {
        this.expanded()?.nativeElement.close();
    }
}
