import { ChangeDetectionStrategy, Component, ElementRef, computed, effect, inject, input, output, signal, viewChild } from '@angular/core';
import { DOCUMENT, NgOptimizedImage, NgTemplateOutlet } from '@angular/common';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoDecimalPipe } from '@jsverse/transloco-locale';
import { CardModule } from 'primeng/card';
import { SkeletonModule } from 'primeng/skeleton';
import { browserIconUrl } from '@core/i18n/browser-utils';
import { countryFlagUrl, languageFlagUrl } from '@core/i18n/flag-utils';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { MetricStat } from '@models/analytics.types';

@Component({
    selector: 'app-metric-list',
    imports: [CardModule, SkeletonModule, TranslocoPipe, TranslocoDecimalPipe, NgOptimizedImage, NgTemplateOutlet],
    templateUrl: './metric-list.html',
    styleUrl: './metric-list.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class MetricList {
    private readonly transloco = inject(TranslocoService);
    private readonly document = inject(DOCUMENT);
    private readonly scrollFrame = viewChild<ElementRef<HTMLElement>>('scrollFrame');

    title = input.required<string>();
    icon = input<string>('pi-list');
    data = input.required<MetricStat[]>();
    isLoading = input<boolean>(false);
    linkMode = input<'none' | 'path' | 'url'>('none');
    siteDomain = input<string | null>(null);
    isRowClickable = input<boolean>(false);
    activeValue = input<string | null>(null);
    showBrowserIcons = input<boolean>(false);
    showCountryFlags = input<boolean>(false);
    showCountryNames = input<boolean>(false);
    showLanguageFlags = input<boolean>(false);
    showLanguageNames = input<boolean>(false);
    framed = input<boolean>(true);
    showHeader = input<boolean>(true);
    rowClicked = output<MetricStat>();

    protected readonly isScrollFrameScrollable = signal(false);
    protected readonly isScrollFrameAtBottom = signal(true);
    protected readonly scrollThumbTop = signal(0);
    protected readonly scrollThumbHeight = signal(100);
    protected readonly totalValue = computed(() => this.data().reduce((sum, item) => sum + item.value, 0));
    protected readonly maxValue = computed(() => {
        const list = this.data();
        if (list.length === 0) return 0;
        return Math.max(...list.map((item) => item.value), 0);
    });

    constructor() {
        effect((onCleanup) => {
            this.data();
            this.isLoading();
            const frame = this.scrollFrame()?.nativeElement;
            if (!frame) return;

            const resizeObserver = new ResizeObserver(() => this.updateScrollFrameState());
            resizeObserver.observe(frame);
            queueMicrotask(() => this.updateScrollFrameState());

            onCleanup(() => resizeObserver.disconnect());
        });
    }

    protected linkInfo(item: MetricStat): { href: string; faviconUrl: string | null } | null {
        const mode = this.linkMode();
        if (mode === 'none' || !item.name) return null;

        if (mode === 'path') {
            const domain = this.siteDomain();
            if (!domain) return null;
            const path = item.name.startsWith('/') ? item.name : `/${item.name}`;
            return {
                href: `https://${domain}${path}`,
                faviconUrl: null
            };
        }

        const url = this.normalizeUrl(item.name);
        if (!url) return null;

        return {
            href: url.href,
            faviconUrl: this.buildFaviconUrl(url.hostname)
        };
    }

    protected countryFlagUrl(value: string): string | null {
        return countryFlagUrl(value);
    }

    protected languageFlagUrl(value: string): string | null {
        return languageFlagUrl(value);
    }

    protected displayLabel(item: MetricStat): string {
        if (this.showCountryNames()) {
            const name = this.countryDisplayName(item.name);
            return name ?? item.name;
        }
        if (this.showLanguageNames()) {
            const name = this.languageDisplayName(item.name);
            return name ?? item.name;
        }
        return item.name;
    }

    protected titleForItem(item: MetricStat): string {
        const display = this.displayLabel(item);
        if (display === item.name) return item.name;
        return `${item.name} · ${display}`;
    }

    protected shareForItem(item: MetricStat): number {
        const total = this.totalValue();
        if (!total) return 0;
        return (item.value / total) * 100;
    }

    protected browserIconUrl(item: MetricStat): string {
        return browserIconUrl(item.name);
    }

    protected isDeviceMetric(): boolean {
        return this.icon() === 'pi-mobile' && this.linkMode() === 'none' && !this.showCountryFlags();
    }

    protected deviceIconClass(item: MetricStat): string {
        const normalized = item.name.trim().toLowerCase();
        if (normalized.includes('tablet')) {
            return 'pi pi-tablet';
        }
        if (normalized.includes('mobile')) {
            return 'pi pi-mobile';
        }
        return 'pi pi-desktop';
    }

    protected barWidth(item: MetricStat): number {
        const max = this.maxValue();
        if (!max) return 0;
        return (item.value / max) * 100;
    }

    protected onRowClick(item: MetricStat): void {
        if (!this.isRowClickable()) return;
        this.rowClicked.emit(item);
    }

    protected onScrollFrame(): void {
        this.updateScrollFrameState();
    }

    protected scrollShellClass(): string {
        const scrollable = this.isScrollFrameScrollable() ? ' metric-list__scroll-shell--scrollable' : '';
        const atBottom = this.isScrollFrameAtBottom() ? ' metric-list__scroll-shell--at-bottom' : '';
        return `metric-list__scroll-shell${scrollable}${atBottom}`;
    }

    protected rowClass(item: MetricStat): string {
        const base = 'metric-list__row group relative flex items-center justify-between overflow-hidden rounded-md text-sm transition-colors';
        const clickable = this.isRowClickable() ? ' cursor-pointer hover:bg-surface-50 dark:hover:bg-surface-800' : '';
        const active = this.isActive(item) ? ' metric-list__row--active' : '';
        return base + clickable + active;
    }

    private buildFaviconUrl(domain: string): string {
        return browserAppUrl(this.document, `/api/favicon/${encodeURIComponent(domain)}`);
    }

    private updateScrollFrameState(): void {
        const frame = this.scrollFrame()?.nativeElement;
        if (!frame) {
            this.isScrollFrameScrollable.set(false);
            this.isScrollFrameAtBottom.set(true);
            return;
        }

        const scrollable = frame.scrollHeight > frame.clientHeight + 1;
        const atBottom = !scrollable || frame.scrollTop + frame.clientHeight >= frame.scrollHeight - 1;
        this.isScrollFrameScrollable.set(scrollable);
        this.isScrollFrameAtBottom.set(atBottom);
        if (!scrollable) {
            this.scrollThumbTop.set(0);
            this.scrollThumbHeight.set(100);
            return;
        }

        const thumbHeight = Math.max(18, (frame.clientHeight / frame.scrollHeight) * 100);
        const maxTop = 100 - thumbHeight;
        const scrollRange = frame.scrollHeight - frame.clientHeight;
        const thumbTop = scrollRange > 0 ? (frame.scrollTop / scrollRange) * maxTop : 0;
        this.scrollThumbHeight.set(thumbHeight);
        this.scrollThumbTop.set(thumbTop);
    }

    private normalizeUrl(raw: string): URL | null {
        const trimmed = raw.trim();
        if (!trimmed || trimmed.startsWith('(')) return null;
        const normalized = /^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;
        try {
            return new URL(normalized);
        } catch {
            return null;
        }
    }

    private countryDisplayName(value: string): string | null {
        const code = value.trim().toUpperCase();
        if (!/^[A-Z]{2}$/.test(code)) return null;
        try {
            const language = this.transloco.getActiveLang();
            const displayNames = new Intl.DisplayNames([language], { type: 'region' });
            return displayNames.of(code) ?? null;
        } catch {
            return null;
        }
    }

    private languageDisplayName(value: string): string | null {
        const code = value.trim().toLowerCase();
        if (!/^[a-z]{2,3}$/.test(code)) return null;
        try {
            const language = this.transloco.getActiveLang();
            const displayNames = new Intl.DisplayNames([language], { type: 'language' });
            return displayNames.of(code) ?? null;
        } catch {
            return null;
        }
    }

    private isActive(item: MetricStat): boolean {
        const active = this.activeValue();
        return !!active && active === item.name;
    }
}
