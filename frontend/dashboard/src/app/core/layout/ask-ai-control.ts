import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, DestroyRef, ElementRef, computed, effect, inject, output, signal, viewChild } from '@angular/core';
import { Router } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import type { EChartsCoreOption, EChartsInitOpts } from 'echarts/core';
import { NgxEchartsDirective } from 'ngx-echarts';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { Subscription, finalize } from 'rxjs';
import { AvatarModule } from '@openng/optimus-ui/avatar';
import { ButtonModule } from '@openng/optimus-ui/button';
import { DrawerModule } from '@openng/optimus-ui/drawer';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';
import { injectActiveLang } from '@core/i18n/active-lang';
import { browserAppUrl } from '@core/interceptors/base-path.interceptor';
import { buildHitkeepChartMergeOptions, buildHitkeepChartOptions, hitkeepChartTheme, withChartAlpha, type HitkeepChartDesign, type HitkeepChartSeries } from '@core/charts/hitkeep-chart-options';
import { provideHitkeepEcharts } from '@core/charts/hitkeep-echarts.provider';
import { AskAIAction, AskAIChart, AskAIMessage, AskAIRequest, AskAIResponse, AskAIStreamEvent } from '@models/analytics.types';
import { AskAIService, AskAIStreamStatusError } from '@services/ask-ai.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { PreferencesService } from '@services/preferences.service';
import { ShareService } from '@services/share.service';
import { TakeoutDownloadService } from '@services/takeout-download.service';
import { UserProfileService } from '@services/user-profile.service';
import { SiteService } from '@features/sites/services/site.service';

type AnswerBlock = { type: 'heading'; text: string } | { type: 'list'; items: string[]; ordered: boolean } | { type: 'paragraph'; text: string };

interface ConversationTurn {
    id: string;
    user: string;
    assistant: string;
}

interface PromptSuggestion {
    key: string;
    labelKey: string;
    icon: string;
}

type ToolCallProgressState = 'running' | 'done' | 'failed';

interface ToolCallProgress {
    key: string;
    toolName: string;
    labelKey: string;
    state: ToolCallProgressState;
}

type SpeechRecognitionConstructor = new () => SpeechRecognitionLike;

interface SpeechRecognitionLike {
    continuous: boolean;
    interimResults: boolean;
    lang: string;
    onend: (() => void) | null;
    onerror: ((event: SpeechRecognitionErrorEventLike) => void) | null;
    onresult: ((event: SpeechRecognitionResultEventLike) => void) | null;
    abort(): void;
    start(): void;
    stop(): void;
}

interface SpeechRecognitionResultEventLike {
    results: SpeechRecognitionResultListLike;
}

interface SpeechRecognitionResultListLike {
    length: number;
    [index: number]: SpeechRecognitionResultLike;
}

interface SpeechRecognitionResultLike {
    isFinal: boolean;
    [index: number]: { transcript: string } | undefined;
}

interface SpeechRecognitionErrorEventLike {
    error?: string;
}

interface SpeechRecognitionWindow extends Window {
    SpeechRecognition?: SpeechRecognitionConstructor;
    webkitSpeechRecognition?: SpeechRecognitionConstructor;
}

@Component({
    selector: 'app-ask-ai-control',
    standalone: true,
    imports: [AvatarModule, ButtonModule, DrawerModule, InputTextModule, MessageModule, NgxEchartsDirective, TranslocoPipe],
    providers: [provideHitkeepEcharts()],
    templateUrl: './ask-ai-control.html',
    styleUrl: './ask-ai-control.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AskAIControl {
    private static readonly palette = ['#2563eb', '#0f766e', '#ca8a04', '#be123c', '#7c3aed'];
    private static readonly toolLabelKeys: Record<string, string> = {
        hitkeep_get_site_overview: 'askAi.tools.siteOverview',
        hitkeep_get_event_names: 'askAi.tools.eventNames',
        hitkeep_get_event_breakdown: 'askAi.tools.eventBreakdown',
        hitkeep_get_ecommerce: 'askAi.tools.ecommerce',
        hitkeep_get_web_vitals: 'askAi.tools.webVitals',
        hitkeep_get_ai_visibility: 'askAi.tools.aiVisibility'
    };
    protected readonly promptSuggestions: PromptSuggestion[] = [
        {
            key: 'traffic',
            labelKey: 'askAi.suggestions.traffic',
            icon: 'pi pi-chart-line'
        },
        { key: 'events', labelKey: 'askAi.suggestions.events', icon: 'pi pi-bolt' },
        {
            key: 'export',
            labelKey: 'askAi.suggestions.export',
            icon: 'pi pi-download'
        }
    ];

    private readonly askAI = inject(AskAIService);
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly siteService = inject(SiteService);
    private readonly share = inject(ShareService);
    private readonly router = inject(Router);
    private readonly takeout = inject(TakeoutDownloadService);
    private readonly document = inject(DOCUMENT);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = injectActiveLang();
    private readonly prefs = inject(PreferencesService);
    protected readonly profile = inject(UserProfileService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly promptInput = viewChild<ElementRef<HTMLInputElement>>('promptInput');
    private lastSiteId: string | null = null;
    private requestSequence = 0;
    private toolProgressSequence = 0;
    private activeRequest: Subscription | null = null;
    private speechRecognition: SpeechRecognitionLike | null = null;
    private dictationBaseQuery = '';
    private focusTimer: ReturnType<typeof setTimeout> | null = null;

    readonly opened = output<void>();
    protected readonly chartInitOptions: EChartsInitOpts = { renderer: 'canvas' };

    protected readonly query = signal('');
    protected readonly drawerVisible = signal(false);
    protected readonly isLoading = signal(false);
    protected readonly response = signal<AskAIResponse | null>(null);
    protected readonly partialAnswer = signal('');
    protected readonly errorKey = signal<string | null>(null);
    protected readonly progressMessageKey = signal<string>('askAi.loading');
    protected readonly toolProgress = signal<ToolCallProgress[]>([]);
    protected readonly feedbackStatus = signal<{
        severity: 'info' | 'success' | 'error';
        key: string;
    } | null>(null);
    protected readonly history = signal<AskAIMessage[]>([]);
    protected readonly currentQuestion = signal('');
    protected readonly runningActionKey = signal('');
    protected readonly isDictating = signal(false);
    protected readonly hitkeepIconUrl = computed(() => browserAppUrl(this.document, '/favicon.svg'));

    protected readonly activeSite = computed(() => this.siteService.activeSite());
    protected readonly activeSiteId = computed(() => this.activeSite()?.id ?? null);
    protected readonly status = computed(() => this.bootstrap.status()?.ask_ai ?? null);
    protected readonly shouldRender = computed(() => {
        const status = this.status();
        return !this.share.isShareMode() && !!this.activeSite() && !!status?.enabled;
    });
    protected readonly canSubmit = computed(() => this.shouldRender() && !!this.status()?.available && this.query().trim().length > 0 && !this.isLoading());
    protected readonly statusLabelKey = computed(() => {
        const status = this.status();
        if (status?.available) return 'askAi.status.ready';
        if (status?.status === 'budget_exhausted') return 'askAi.disabled.budget';
        if (status?.status === 'not_configured') return 'askAi.disabled.notConfigured';
        return 'askAi.disabled.unavailable';
    });
    protected readonly placeholderKey = computed(() => {
        const status = this.status();
        if (!status?.available) {
            if (status?.status === 'budget_exhausted') return 'askAi.disabled.budget';
            if (status?.status === 'not_configured') return 'askAi.disabled.notConfigured';
            return 'askAi.disabled.unavailable';
        }
        return this.history().length > 0 ? 'askAi.followUpTriggerPlaceholder' : 'askAi.triggerPlaceholder';
    });
    protected readonly promptPlaceholderKey = computed(() => (this.history().length > 0 ? 'askAi.followUpPlaceholder' : 'askAi.promptPlaceholder'));
    protected readonly answerBlocks = computed(() => this.markdownBlocks(this.response()?.answer_markdown ?? this.partialAnswer()));
    protected readonly previousTurns = computed(() => {
        const history = this.history();
        const visibleHistory = this.response() ? history.slice(0, Math.max(0, history.length - 2)) : history;
        return this.toConversationTurns(visibleHistory);
    });
    protected readonly hasTranscript = computed(() => this.previousTurns().length > 0 || this.currentQuestion().trim().length > 0 || this.answerBlocks().length > 0 || this.toolProgress().length > 0 || this.isLoading());
    protected readonly hasSession = computed(() => this.history().length > 0 || this.currentQuestion().trim().length > 0 || this.partialAnswer().trim().length > 0 || !!this.response() || !!this.errorKey() || this.isLoading());
    protected readonly canStartNewChat = computed(() => this.query().trim().length > 0 || this.hasSession());

    constructor() {
        effect(() => {
            const siteId = this.activeSiteId();
            if (siteId === this.lastSiteId) return;
            this.lastSiteId = siteId;
            this.resetSessionState();
        });
        this.destroyRef.onDestroy(() => {
            this.stopDictation(true);
            this.clearFocusTimer();
        });
    }

    protected submit(event?: Event): void {
        event?.preventDefault();
        if (!this.canSubmit()) {
            return;
        }

        const site = this.activeSite();
        const query = this.query().trim();
        if (!site || query === '') {
            return;
        }
        this.stopDictation(true);
        this.cancelActiveRequest();
        const requestId = ++this.requestSequence;
        const history = this.history();
        const request: AskAIRequest = {
            query,
            route: this.router.url,
            history,
            ...this.explicitDateRangeFromRoute()
        };

        this.opened.emit();
        this.drawerVisible.set(true);
        this.currentQuestion.set(query);
        this.query.set('');
        this.response.set(null);
        this.partialAnswer.set('');
        this.errorKey.set(null);
        this.progressMessageKey.set('askAi.progress.accepted');
        this.toolProgress.set([]);
        this.toolProgressSequence = 0;
        this.feedbackStatus.set(null);
        this.isLoading.set(true);

        let subscription: Subscription | null = null;
        let sawTerminalEvent = false;
        subscription = this.askAI
            .askStream(site.id, request)
            .pipe(
                finalize(() => {
                    if (subscription && this.activeRequest === subscription) {
                        this.activeRequest = null;
                    }
                    if (this.isCurrentRequest(requestId, site.id)) {
                        this.isLoading.set(false);
                    }
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (event) => {
                    if (!this.isCurrentRequest(requestId, site.id)) {
                        return;
                    }
                    if (event.type === 'final' || event.type === 'error') {
                        sawTerminalEvent = true;
                    }
                    this.applyStreamEvent(event, query);
                },
                error: (error) => {
                    if (this.isCurrentRequest(requestId, site.id)) {
                        this.partialAnswer.set('');
                        this.finishRunningToolProgress('failed');
                        this.errorKey.set(this.streamErrorKey(error));
                    }
                },
                complete: () => {
                    if (this.isCurrentRequest(requestId, site.id) && !sawTerminalEvent) {
                        this.partialAnswer.set('');
                        this.progressMessageKey.set('askAi.loading');
                        this.finishRunningToolProgress('failed');
                        this.errorKey.set('askAi.errors.request');
                    }
                }
            });
        this.activeRequest = subscription;
    }

    protected openDrawer(): void {
        if (!this.shouldRender()) {
            return;
        }
        this.opened.emit();
        this.drawerVisible.set(true);
        if (this.status()?.available) {
            this.schedulePromptFocus();
        }
    }

    protected onDrawerVisibleChange(visible: boolean): void {
        this.drawerVisible.set(visible);
        if (!visible) {
            this.stopDictation(true);
            this.clearFocusTimer();
        }
    }

    protected startNewChat(): void {
        if (!this.canStartNewChat()) {
            return;
        }
        this.stopDictation(true);
        this.cancelActiveRequest();
        this.requestSequence++;
        this.query.set('');
        this.currentQuestion.set('');
        this.isLoading.set(false);
        this.response.set(null);
        this.partialAnswer.set('');
        this.errorKey.set(null);
        this.progressMessageKey.set('askAi.loading');
        this.toolProgress.set([]);
        this.toolProgressSequence = 0;
        this.feedbackStatus.set(null);
        this.runningActionKey.set('');
        this.history.set([]);
        this.drawerVisible.set(true);
        this.schedulePromptFocus();
    }

    protected stopGenerating(): void {
        if (!this.isLoading()) {
            return;
        }
        this.cancelActiveRequest();
        this.requestSequence++;
        this.isLoading.set(false);
        this.progressMessageKey.set('askAi.loading');
        this.finishRunningToolProgress('failed');
        this.feedbackStatus.set({ severity: 'info', key: 'askAi.stopped' });
    }

    protected runAction(action: AskAIAction): void {
        if (action.type === 'navigate') {
            if (!this.isSafeNavigationTarget(action.target)) {
                this.feedbackStatus.set({
                    severity: 'error',
                    key: 'askAi.errors.action'
                });
                return;
            }
            this.router.navigateByUrl(action.target);
            this.drawerVisible.set(false);
            this.clearFocusTimer();
            return;
        }

        if (this.runningActionKey()) {
            return;
        }
        if (action.type !== 'download_export' || !this.isSafeExportTarget(action.target)) {
            this.feedbackStatus.set({
                severity: 'error',
                key: 'askAi.errors.export'
            });
            return;
        }

        this.feedbackStatus.set(null);
        const actionKey = this.actionKey(action);
        this.runningActionKey.set(actionKey);
        this.takeout
            .downloadFromUrl(action.target, this.exportFilename(action))
            .pipe(
                finalize(() => {
                    if (this.runningActionKey() === actionKey) {
                        this.runningActionKey.set('');
                    }
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: () =>
                    this.feedbackStatus.set({
                        severity: 'success',
                        key: 'askAi.exportSuccess'
                    }),
                error: () =>
                    this.feedbackStatus.set({
                        severity: 'error',
                        key: 'askAi.errors.export'
                    })
            });
    }

    protected actionIcon(action: AskAIAction): string {
        return action.type === 'download_export' ? 'pi pi-download' : 'pi pi-arrow-right';
    }

    protected actionKey(action: AskAIAction): string {
        return `${action.type}:${action.target}:${action.label}`;
    }

    protected isActionRunning(action: AskAIAction): boolean {
        return this.runningActionKey() === this.actionKey(action);
    }

    protected useSuggestion(suggestion: PromptSuggestion): void {
        if (!this.status()?.available) {
            return;
        }
        this.stopDictation(true);
        this.query.set(this.transloco.translate(suggestion.labelKey));
        this.schedulePromptFocus();
    }

    protected updateQueryFromInput(event: Event): void {
        this.query.set((event.target as HTMLInputElement | null)?.value ?? '');
    }

    protected supportsDictation(): boolean {
        return !!this.speechRecognitionConstructor();
    }

    protected canUseDictation(): boolean {
        return this.supportsDictation() && !!this.status()?.available && !this.isLoading();
    }

    protected dictationAriaKey(): string {
        return this.isDictating() ? 'askAi.dictation.stopAria' : 'askAi.dictation.startAria';
    }

    protected toggleDictation(): void {
        if (this.isDictating()) {
            this.stopDictation(false);
            this.schedulePromptFocus();
            return;
        }
        if (!this.canUseDictation()) {
            return;
        }
        const Recognition = this.speechRecognitionConstructor();
        if (!Recognition) {
            return;
        }

        const recognition = new Recognition();
        recognition.continuous = true;
        recognition.interimResults = true;
        recognition.lang = this.transloco.getActiveLang();
        recognition.onresult = (event) => this.applyDictationResult(event);
        recognition.onerror = () => this.finishDictation(recognition);
        recognition.onend = () => this.finishDictation(recognition);

        this.speechRecognition = recognition;
        this.dictationBaseQuery = this.query().trim();
        this.isDictating.set(true);
        try {
            recognition.start();
        } catch {
            this.finishDictation(recognition);
        }
    }

    protected isTable(chart: AskAIChart): boolean {
        return chart.type === 'table';
    }

    protected tableColumns(chart: AskAIChart): string[] {
        const columns = new Set<string>();
        for (const row of chart.rows ?? []) {
            for (const key of Object.keys(row)) {
                columns.add(key);
            }
        }
        return Array.from(columns).slice(0, 12);
    }

    protected formatColumnHeader(key: string): string {
        const value = key.trim();
        if (!value) {
            return '—';
        }
        return value
            .replace(/[_-]+/g, ' ')
            .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
            .replace(/\s+/g, ' ')
            .replace(/\b\w/g, (letter) => letter.toUpperCase());
    }

    protected chartOptions(chart: AskAIChart): EChartsCoreOption {
        this.activeLanguage();
        const xKey = chart.x_key ?? '';
        const rows = chart.rows ?? [];
        const design = this.chartDesign(chart);
        return buildHitkeepChartOptions({
            ariaLabel: chart.title,
            design,
            labels: rows.map((row) => this.formatCell(row[xKey] ?? '')),
            locale: this.transloco.getActiveLang(),
            series: this.askAIChartSeries(chart, rows, design),
            theme: hitkeepChartTheme(this.prefs.isDarkMode()),
            yAxisTicks: 5
        });
    }

    protected chartMergeOptions(chart: AskAIChart): EChartsCoreOption {
        this.activeLanguage();
        const xKey = chart.x_key ?? '';
        const rows = chart.rows ?? [];
        const design = this.chartDesign(chart);
        return buildHitkeepChartMergeOptions({
            ariaLabel: chart.title,
            design,
            labels: rows.map((row) => this.formatCell(row[xKey] ?? '')),
            locale: this.transloco.getActiveLang(),
            series: this.askAIChartSeries(chart, rows, design),
            theme: hitkeepChartTheme(this.prefs.isDarkMode()),
            yAxisTicks: 5
        });
    }

    protected formatCell(value: string | number | boolean | null | undefined): string {
        if (value === null || value === undefined || value === '') {
            return '—';
        }
        if (typeof value === 'number') {
            return new Intl.NumberFormat(this.transloco.getActiveLang()).format(value);
        }
        if (typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value)) {
            const date = new Date(`${value}T00:00:00Z`);
            if (!Number.isNaN(date.getTime())) {
                return new Intl.DateTimeFormat(this.transloco.getActiveLang(), {
                    dateStyle: 'medium',
                    timeZone: 'UTC'
                }).format(date);
            }
        }
        return String(value);
    }

    private chartDesign(chart: AskAIChart): HitkeepChartDesign {
        return chart.type === 'bar' ? 'bar' : 'area';
    }

    private askAIChartSeries(chart: AskAIChart, rows: AskAIChart['rows'], design: HitkeepChartDesign): HitkeepChartSeries[] {
        return (chart.series ?? []).map((item, index) => {
            const color = AskAIControl.palette[index % AskAIControl.palette.length];
            return {
                id: item.key,
                label: item.label,
                data: rows.map((row) => this.numericCell(row[item.key])),
                color,
                gradientFrom: withChartAlpha(color, 0.14),
                gradientTo: withChartAlpha(color, 0),
                design
            };
        });
    }

    private numericCell(value: string | number | boolean | null | undefined): number {
        const numberValue = Number(value ?? 0);
        return Number.isFinite(numberValue) ? numberValue : 0;
    }

    protected turnTrack(turn: ConversationTurn): string {
        return turn.id;
    }

    protected toolProgressTrack(progress: ToolCallProgress): string {
        return progress.key;
    }

    protected toolProgressIcon(progress: ToolCallProgress): string {
        if (progress.state === 'running') {
            return 'pi pi-spin pi-spinner';
        }
        return progress.state === 'failed' ? 'pi pi-exclamation-circle' : 'pi pi-check';
    }

    protected blockTrack(block: AnswerBlock): string {
        return block.type === 'list' ? block.items.join('|') : block.text;
    }

    private toConversationTurns(messages: AskAIMessage[]): ConversationTurn[] {
        const turns: ConversationTurn[] = [];
        for (let index = 0; index < messages.length; index += 2) {
            const user = messages[index];
            const assistant = messages[index + 1];
            if (user?.role === 'user' && assistant?.role === 'assistant') {
                turns.push({
                    id: `${index}-${user.content}`,
                    user: user.content,
                    assistant: assistant.content
                });
            }
        }
        return turns.slice(-3);
    }

    private markdownBlocks(markdown: string): AnswerBlock[] {
        return markdown
            .split(/\n{2,}/)
            .map((block) => block.trim())
            .filter(Boolean)
            .map((block): AnswerBlock => {
                const lines = block.split('\n').map((line) => line.trim());
                const heading = lines.length === 1 ? lines[0]?.replace(/^#{1,3}\s+/, '').trim() : '';
                if (heading && heading !== lines[0]) {
                    return { type: 'heading', text: this.cleanMarkdownText(heading) };
                }
                const listLines = lines.filter((line) => /^([-*]|\d+[.)])\s+/.test(line));
                const listItems = listLines.map((line) => this.cleanMarkdownText(line.replace(/^([-*]|\d+[.)])\s+/, '').trim())).filter(Boolean);
                if (listItems.length === lines.length) {
                    return {
                        type: 'list',
                        items: listItems,
                        ordered: lines.every((line) => /^\d+[.)]\s+/.test(line))
                    };
                }
                return {
                    type: 'paragraph',
                    text: this.cleanMarkdownText(lines.join('\n'))
                };
            });
    }

    private cleanMarkdownText(value: string): string {
        return value
            .replace(/!\[([^\]]*)]\([^)]+\)/g, '$1')
            .replace(/\[([^\]]+)]\([^)]+\)/g, '$1')
            .replace(/\*\*([^*]+)\*\*/g, '$1')
            .replace(/__([^_]+)__/g, '$1')
            .replace(/\*([^*]+)\*/g, '$1')
            .replace(/_([^_]+)_/g, '$1')
            .replace(/`([^`]+)`/g, '$1')
            .trim();
    }

    private isSafeExportTarget(target: string): boolean {
        const site = this.activeSite();
        return !!site && target.startsWith(`/api/sites/${site.id}/takeout?`);
    }

    private isSafeNavigationTarget(target: string): boolean {
        return target.startsWith('/') && !target.startsWith('//') && !target.startsWith('/api/') && !target.includes('\\');
    }

    private resetSessionState(): void {
        this.stopDictation(true);
        this.cancelActiveRequest();
        this.clearFocusTimer();
        this.requestSequence++;
        this.query.set('');
        this.currentQuestion.set('');
        this.drawerVisible.set(false);
        this.isLoading.set(false);
        this.response.set(null);
        this.partialAnswer.set('');
        this.errorKey.set(null);
        this.progressMessageKey.set('askAi.loading');
        this.toolProgress.set([]);
        this.toolProgressSequence = 0;
        this.feedbackStatus.set(null);
        this.history.set([]);
        this.runningActionKey.set('');
    }

    private applyStreamEvent(event: AskAIStreamEvent, query: string): void {
        if (event.type === 'progress') {
            this.progressMessageKey.set(event.message_key || 'askAi.loading');
            this.applyToolProgress(event);
            return;
        }
        if (event.type === 'delta') {
            this.progressMessageKey.set('askAi.progress.composing');
            if (event.delta_markdown) {
                this.partialAnswer.update((answer) => answer + event.delta_markdown);
            }
            return;
        }
        if (event.type === 'error') {
            this.progressMessageKey.set('askAi.loading');
            this.partialAnswer.set('');
            this.finishRunningToolProgress('failed');
            this.errorKey.set(event.message_key || 'askAi.errors.request');
            return;
        }
        if (event.type === 'final' && event.response) {
            const response = event.response;
            this.progressMessageKey.set('askAi.loading');
            this.partialAnswer.set('');
            this.finishRunningToolProgress('done');
            this.response.set(response);
            this.history.update((history) => [...history, { role: 'user' as const, content: query }, { role: 'assistant' as const, content: response.answer_markdown }].slice(-8));
        }
    }

    private applyToolProgress(event: AskAIStreamEvent): void {
        if (event.status !== 'tool_call_start' && event.status !== 'tool_call_finish') {
            return;
        }
        const toolName = event.tool_name?.trim();
        if (!toolName) {
            return;
        }
        if (event.status === 'tool_call_start') {
            const key = event.tool_call_id?.trim() || `${toolName}:${++this.toolProgressSequence}`;
            const next: ToolCallProgress = {
                key,
                toolName,
                labelKey: this.toolLabelKey(toolName),
                state: 'running'
            };
            this.toolProgress.update((progress) => {
                const existingIndex = progress.findIndex((item) => item.key === key);
                if (existingIndex === -1) {
                    return [...progress, next];
                }
                return progress.map((item, index) => (index === existingIndex ? { ...item, state: 'running' } : item));
            });
            return;
        }

        this.toolProgress.update((progress) => {
            const index = this.findToolProgressIndex(progress, event);
            if (index === -1) {
                return [
                    ...progress,
                    {
                        key: event.tool_call_id?.trim() || `${toolName}:${++this.toolProgressSequence}`,
                        toolName,
                        labelKey: this.toolLabelKey(toolName),
                        state: 'done'
                    }
                ];
            }
            return progress.map((item, itemIndex) => (itemIndex === index ? { ...item, state: 'done' } : item));
        });
    }

    private findToolProgressIndex(progress: ToolCallProgress[], event: AskAIStreamEvent): number {
        const toolCallID = event.tool_call_id?.trim();
        if (toolCallID) {
            const byID = progress.findIndex((item) => item.key === toolCallID);
            if (byID !== -1) {
                return byID;
            }
        }
        const toolName = event.tool_name?.trim();
        if (!toolName) {
            return -1;
        }
        for (let index = progress.length - 1; index >= 0; index--) {
            const item = progress[index];
            if (item?.toolName === toolName && item.state === 'running') {
                return index;
            }
        }
        return -1;
    }

    private finishRunningToolProgress(state: Exclude<ToolCallProgressState, 'running'>): void {
        this.toolProgress.update((progress) => progress.map((item) => (item.state === 'running' ? { ...item, state } : item)));
    }

    private toolLabelKey(toolName: string): string {
        return AskAIControl.toolLabelKeys[toolName] ?? 'askAi.tools.analytics';
    }

    private streamErrorKey(error: unknown): string {
        if (error instanceof AskAIStreamStatusError) {
            const status = error.askAIStatus?.status;
            if (status === 'budget_exhausted' || error.statusCode === 429) {
                return 'askAi.errors.budget';
            }
            if (status === 'not_configured' || status === 'disabled') {
                return 'askAi.errors.notConfigured';
            }
        }
        return 'askAi.errors.request';
    }

    private cancelActiveRequest(): void {
        this.activeRequest?.unsubscribe();
        this.activeRequest = null;
    }

    private applyDictationResult(event: SpeechRecognitionResultEventLike): void {
        const transcript = this.dictationTranscript(event);
        if (!transcript) {
            return;
        }
        this.query.set([this.dictationBaseQuery, transcript].filter(Boolean).join(' '));
    }

    private dictationTranscript(event: SpeechRecognitionResultEventLike): string {
        const parts: string[] = [];
        const results = Array.from({ length: event.results.length }, (_, index) => event.results[index]);
        for (const result of results) {
            const transcript = result?.[0]?.transcript?.trim();
            if (transcript) {
                parts.push(transcript);
            }
        }
        return parts.join(' ').replace(/\s+/g, ' ').trim();
    }

    private finishDictation(recognition: SpeechRecognitionLike): void {
        if (this.speechRecognition !== recognition) {
            return;
        }
        this.speechRecognition = null;
        this.isDictating.set(false);
    }

    private stopDictation(abort: boolean): void {
        const recognition = this.speechRecognition;
        if (!recognition) {
            this.isDictating.set(false);
            return;
        }
        if (abort) {
            recognition.onend = null;
            recognition.onerror = null;
            recognition.onresult = null;
            this.speechRecognition = null;
            this.isDictating.set(false);
            try {
                recognition.abort();
            } catch {
                // Native speech recognition can throw if it already ended.
            }
            return;
        }
        try {
            recognition.stop();
        } catch {
            this.finishDictation(recognition);
        }
    }

    private speechRecognitionConstructor(): SpeechRecognitionConstructor | null {
        const win = this.document.defaultView as SpeechRecognitionWindow | null;
        return win?.SpeechRecognition ?? win?.webkitSpeechRecognition ?? null;
    }

    private schedulePromptFocus(): void {
        this.clearFocusTimer();
        this.focusTimer = setTimeout(() => {
            this.focusTimer = null;
            this.promptInput()?.nativeElement.focus();
        }, 0);
    }

    private clearFocusTimer(): void {
        if (this.focusTimer === null) {
            return;
        }
        clearTimeout(this.focusTimer);
        this.focusTimer = null;
    }

    private isCurrentRequest(requestId: number, siteId: string): boolean {
        return requestId === this.requestSequence && this.activeSiteId() === siteId;
    }

    private explicitDateRangeFromRoute(): Pick<AskAIRequest, 'from' | 'to'> {
        try {
            const url = new URL(this.router.url, 'https://hitkeep.local');
            const from = this.explicitDateParam(url.searchParams, ['from', 'date_from', 'start']);
            const to = this.explicitDateParam(url.searchParams, ['to', 'date_to', 'end']);
            return from && to ? { from, to } : {};
        } catch {
            return {};
        }
    }

    private explicitDateParam(params: URLSearchParams, keys: string[]): string {
        for (const key of keys) {
            const value = params.get(key)?.trim() ?? '';
            if (/^\d{4}-\d{2}-\d{2}$/.test(value)) {
                return value;
            }
        }
        return '';
    }

    private exportFilename(action: AskAIAction): string {
        const site = this.activeSite();
        const safeDomain = (site?.domain || 'site')
            .toLowerCase()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/(^-|-$)/g, '');
        const format = action.format || 'xlsx';
        return `${safeDomain || 'site'}-ask-ai-${new Date().toISOString().slice(0, 10)}.${format}`;
    }
}
