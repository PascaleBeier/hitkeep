import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, inject, input, output } from '@angular/core';
import type { EChartsCoreOption, EChartsInitOpts } from 'echarts/core';
import { NgxEchartsDirective } from 'ngx-echarts';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { injectActiveLang } from '@core/i18n/active-lang';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { buildHitkeepChartMergeOptions, buildHitkeepChartOptions, hitkeepChartTheme, withChartAlpha, type HitkeepChartDesign, type HitkeepChartSeries } from '@core/charts/hitkeep-chart-options';
import { provideHitkeepEcharts } from '@core/charts/hitkeep-echarts.provider';
import { AskAIAction, AskAIChart, AskAIResponse } from '@models/analytics.types';
import { PreferencesService } from '@services/preferences.service';
import MarkdownIt from 'markdown-it';

const markdownConverter = new MarkdownIt({ html: false, linkify: false, typographer: false });

@Component({
    selector: 'app-ask-ai-answer',
    standalone: true,
    imports: [ButtonModule, NgxEchartsDirective, TranslocoPipe],
    providers: [provideHitkeepEcharts()],
    templateUrl: './ask-ai-answer.html',
    styleUrl: './ask-ai-answer.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AskAIAnswer {
    private static readonly palette = ['#2563eb', '#0f766e', '#ca8a04', '#be123c', '#7c3aed'];
    private readonly document = inject(DOCUMENT);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = injectActiveLang();
    private readonly prefs = inject(PreferencesService);

    readonly answer = input<AskAIResponse | null>(null);
    readonly partialMarkdown = input('');
    readonly loading = input(false);
    readonly runningActionKey = input('');
    readonly actionRequested = output<AskAIAction>();
    protected readonly chartInitOptions: EChartsInitOpts = { renderer: 'canvas' };
    protected readonly hitkeepIconUrl = computed(() => browserAppUrl(this.document, '/favicon.svg'));
    protected readonly renderedMarkdown = computed(() => this.renderMarkdown(this.answer()?.answer_markdown ?? this.partialMarkdown(), this.answer()?.citations ?? []));

    protected actionIcon(action: AskAIAction): string {
        return action.type === 'download_export' ? 'pi pi-download' : 'pi pi-arrow-right';
    }
    protected isActionRunning(action: AskAIAction): boolean {
        return this.runningActionKey() === `${action.type}:${action.target}:${action.label}`;
    }
    protected isTable(chart: AskAIChart): boolean {
        return chart.type === 'table';
    }
    protected tableColumns(chart: AskAIChart): string[] {
        const columns = new Set<string>();
        for (const row of chart.rows ?? []) for (const key of Object.keys(row)) columns.add(key);
        return Array.from(columns).slice(0, 12);
    }
    protected formatColumnHeader(key: string): string {
        const value = key.trim();
        if (!value) return '—';
        return value
            .replace(/[_-]+/g, ' ')
            .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
            .replace(/\s+/g, ' ')
            .replace(/\b\w/g, (letter) => letter.toUpperCase());
    }
    private readonly chartOptionCache = computed(() => {
        this.activeLanguage();
        const locale = this.transloco.getActiveLang();
        const theme = hitkeepChartTheme(this.prefs.isDarkMode());
        const cache = new Map<AskAIChart, { options: EChartsCoreOption; merge: EChartsCoreOption }>();
        for (const chart of this.answer()?.charts ?? []) {
            if (chart.type === 'table') continue;
            const rows = chart.rows ?? [];
            const design = this.chartDesign(chart);
            const xKey = chart.x_key ?? '';
            const settings = { ariaLabel: chart.title, design, labels: rows.map((row) => this.formatCell(row[xKey] ?? '')), locale, series: this.chartSeries(chart, rows, design), theme, yAxisTicks: 5 };
            cache.set(chart, { options: buildHitkeepChartOptions(settings), merge: buildHitkeepChartMergeOptions(settings) });
        }
        return cache;
    });
    protected chartOptions(chart: AskAIChart): EChartsCoreOption {
        return this.chartOptionCache().get(chart)?.options ?? {};
    }
    protected chartMergeOptions(chart: AskAIChart): EChartsCoreOption {
        return this.chartOptionCache().get(chart)?.merge ?? {};
    }
    protected formatCell(value: string | number | boolean | null | undefined): string {
        this.activeLanguage();
        if (value === null || value === undefined || value === '') return '—';
        if (typeof value === 'number') return new Intl.NumberFormat(this.transloco.getActiveLang()).format(value);
        if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value)) {
            const date = new Date(`${value}T00:00:00Z`);
            if (!Number.isNaN(date.getTime())) return new Intl.DateTimeFormat(this.transloco.getActiveLang(), { dateStyle: 'medium', timeZone: 'UTC' }).format(date);
        }
        return String(value);
    }
    private chartDesign(chart: AskAIChart): HitkeepChartDesign {
        return chart.type === 'bar' ? 'bar' : 'area';
    }
    private chartSeries(chart: AskAIChart, rows: AskAIChart['rows'], design: HitkeepChartDesign): HitkeepChartSeries[] {
        return (chart.series ?? []).map((item, index) => {
            const color = AskAIAnswer.palette[index % AskAIAnswer.palette.length];
            return { id: item.key, label: item.label, data: rows.map((row) => this.numericCell(row[item.key])), color, gradientFrom: withChartAlpha(color, 0.14), gradientTo: withChartAlpha(color, 0), design };
        });
    }
    private numericCell(value: string | number | boolean | null | undefined): number {
        const numberValue = Number(value ?? 0);
        return Number.isFinite(numberValue) ? numberValue : 0;
    }
    private renderMarkdown(markdown: string, citations: AskAIResponse['citations']): string {
        if (!markdown.trim()) return '';
        const template = this.document.createElement('template');
        template.innerHTML = markdownConverter.render(markdown);
        for (const image of Array.from(template.content.querySelectorAll('img'))) {
            image.replaceWith(this.document.createTextNode(image.getAttribute('alt') ?? ''));
        }
        for (const link of Array.from(template.content.querySelectorAll('a'))) {
            const href = link.getAttribute('href');
            if (!href || !/^https?:\/\//i.test(href)) {
                link.replaceWith(...Array.from(link.childNodes));
                continue;
            }
            try {
                const url = new URL(href);
                if (url.protocol !== 'http:' && url.protocol !== 'https:') throw new Error('Unsafe link');
                link.setAttribute('href', url.href);
                link.setAttribute('target', '_blank');
                link.setAttribute('rel', 'noopener noreferrer nofollow');
            } catch {
                link.replaceWith(...Array.from(link.childNodes));
            }
        }
        const citedCalls = new Set(citations.map((citation) => citation.tool_call_id));
        const textWalker = this.document.createTreeWalker(template.content, 4);
        const textNodes: Node[] = [];
        while (textWalker.nextNode()) textNodes.push(textWalker.currentNode);
        for (const node of textNodes) {
            if (node.parentElement?.closest('code, pre')) continue;
            node.textContent = (node.textContent ?? '').replace(/[ \t]*\[Source:\s*([^\]\n]+)\]/gi, (marker, id: string) => (citedCalls.has(id.trim()) ? '' : marker));
        }
        for (const paragraph of Array.from(template.content.querySelectorAll('p'))) {
            if (!paragraph.textContent?.trim()) paragraph.remove();
        }
        return template.innerHTML;
    }
}
