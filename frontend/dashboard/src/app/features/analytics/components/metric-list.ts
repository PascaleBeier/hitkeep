import { ChangeDetectionStrategy, Component, ElementRef, computed, effect, inject, input, output, signal, viewChild } from '@angular/core';
import { DOCUMENT, NgOptimizedImage, NgTemplateOutlet } from '@angular/common';
import { Router, RouterLink, UrlTree } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { TranslocoDecimalPipe } from '@jsverse/transloco-locale';
import { CardModule } from '@openng/optimus-ui/card';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { browserIconUrl } from '@core/i18n/browser-utils';
import { countryFlagUrl, languageFlagUrl } from '@core/i18n/flag-utils';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { aiCategoryLabel } from '@features/analytics/ai-category-labels';
import { AIAgentIconsService } from '@services/ai-agent-icons.service';
import { injectSkeletonGate } from '@services/report-subject.service';
import { AIActivityStat, MetricStat } from '@models/analytics.types';

export interface MetricListItem extends MetricStat {
    key?: string;
    shareLabel?: string;
    valueLabel?: string;
    detailsHref?: string;
    detailsAriaLabel?: string;
}

/** True for rows the AI activity report enriched with provenance counters. */
function hasProvenanceCounters(item: MetricStat): item is AIActivityStat {
    const candidate = item as Partial<AIActivityStat>;
    return typeof candidate.tracked_hits === 'number' && typeof candidate.fetch_count === 'number';
}

@Component({
    selector: 'app-metric-list',
    imports: [CardModule, SkeletonModule, TranslocoPipe, TranslocoDecimalPipe, NgOptimizedImage, NgTemplateOutlet, RouterLink],
    templateUrl: './metric-list.html',
    styleUrl: './metric-list.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class MetricList {
    private readonly transloco = inject(TranslocoService);
    private readonly document = inject(DOCUMENT);
    private readonly router = inject(Router);
    private readonly aiAgentIcons = inject(AIAgentIconsService);
    private readonly scrollFrame = viewChild<ElementRef<HTMLElement>>('scrollFrame');

    title = input.required<string>();
    icon = input<string>('pi-list');
    data = input.required<MetricListItem[]>();
    isLoading = input<boolean>(false);
    linkMode = input<'none' | 'path' | 'url' | 'details'>('none');
    siteDomain = input<string | null>(null);
    isRowClickable = input<boolean>(false);
    activeValue = input<string | null>(null);
    showBrowserIcons = input<boolean>(false);
    showCountryFlags = input<boolean>(false);
    showCountryNames = input<boolean>(false);
    showLanguageFlags = input<boolean>(false);
    showLanguageNames = input<boolean>(false);
    showAICategoryNames = input<boolean>(false);
    aiIconKind = input<'none' | 'agent' | 'source'>('none');
    /**
     * Annotates rows of the AI activity report with where their requests came
     * from. Only rows the forwarded logs contributed to get the hint; pure
     * tracked rows stay exactly as they render without this input.
     */
    showProvenance = input<boolean>(false);
    /**
     * Share of the list total next to every value. Switch it off for lists whose
     * rows are not parts of one whole — a correlation row's share of the visible
     * top-N says nothing about the metric the row reports.
     */
    showShare = input<boolean>(true);
    framed = input<boolean>(true);
    showHeader = input<boolean>(true);
    rowClicked = output<MetricListItem>();

    protected readonly hasRows = computed(() => this.data().length > 0);
    protected readonly showSkeleton = injectSkeletonGate(this.isLoading, this.hasRows);
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
            this.showSkeleton();
            const frame = this.scrollFrame()?.nativeElement;
            if (!frame) return;

            const resizeObserver = new ResizeObserver(() => this.updateScrollFrameState());
            resizeObserver.observe(frame);
            queueMicrotask(() => this.updateScrollFrameState());

            onCleanup(() => resizeObserver.disconnect());
        });
    }

    protected linkInfo(item: MetricListItem): { href?: string; routerLink?: UrlTree; faviconUrl: string | null; external: boolean; ariaLabel: string } | null {
        const mode = this.linkMode();
        if (mode === 'none' || !item.name) return null;

        if (mode === 'details') {
            if (!item.detailsHref) return null;
            return {
                routerLink: this.router.parseUrl(item.detailsHref),
                faviconUrl: null,
                external: false,
                ariaLabel: item.detailsAriaLabel ?? this.transloco.translate('common.actions.open')
            };
        }

        if (mode === 'path') {
            const domain = this.siteDomain();
            if (!domain) return null;
            const path = item.name.startsWith('/') ? item.name : `/${item.name}`;
            return {
                href: `https://${domain}${path}`,
                faviconUrl: null,
                external: true,
                ariaLabel: this.transloco.translate('common.openInNewTabAria')
            };
        }

        const url = this.normalizeUrl(item.name);
        if (!url) return null;

        return {
            href: url.href,
            faviconUrl: this.buildFaviconUrl(url.hostname),
            external: true,
            ariaLabel: this.transloco.translate('common.openInNewTabAria')
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
        if (this.showAICategoryNames()) {
            return aiCategoryLabel(this.transloco, item.name);
        }
        return item.name;
    }

    /** `provenance` comes from the template's single `provenanceHint` call per row. */
    protected titleForItem(item: MetricStat, provenance: string | null): string {
        const display = this.displayLabel(item);
        const base = display === item.name ? item.name : `${item.name} · ${display}`;
        return provenance ? `${base} · ${provenance}` : base;
    }

    /** Translated `tracked … · logs …` hint, or `null` for rows without log-side requests. */
    protected provenanceHint(item: MetricStat): string | null {
        if (!this.showProvenance() || !hasProvenanceCounters(item) || item.fetch_count <= 0) return null;
        return this.transloco.translate('aiAgents.provenance.hint', {
            tracked: item.tracked_hits,
            fetched: item.fetch_count
        });
    }

    protected shareForItem(item: MetricListItem): number {
        const total = this.totalValue();
        if (!total) return 0;
        return (item.value / total) * 100;
    }

    protected browserIconUrl(item: MetricStat): string {
        return browserIconUrl(item.name);
    }

    protected aiIconUrl(item: MetricStat): string | null {
        const kind = this.aiIconKind();
        if (kind === 'none') return null;
        const host = kind === 'agent' ? this.aiAgentIcons.agentIconHost(item.name) : this.aiAgentIcons.referrerIconHost(item.name);
        return host ? this.buildFaviconUrl(host) : null;
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

    protected onRowClick(item: MetricListItem): void {
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

    protected rowClass(item: MetricListItem): string {
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

    private isActive(item: MetricListItem): boolean {
        const active = this.activeValue();
        return !!active && active === (item.key ?? item.name);
    }
}
