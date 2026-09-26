import { DOCUMENT } from '@angular/common';
import { ChangeDetectionStrategy, Component, DestroyRef, ElementRef, afterRenderEffect, computed, effect, inject, output, signal, viewChild } from '@angular/core';
import { Router } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { Subscription, finalize } from 'rxjs';
import { ButtonModule } from '@openng/optimus-ui/button';
import { DrawerModule } from '@openng/optimus-ui/drawer';
import { TextareaModule } from '@openng/optimus-ui/textarea';
import { MessageModule } from '@openng/optimus-ui/message';
import { injectActiveLang } from '@core/i18n/active-lang';
import { buildTakeoutExportFilename } from '@core/export/export-formats';
import { AskAIAction, AskAIRequest, AskAIStatus } from '@models/analytics.types';
import { DOCS_LINKS } from '@core/config/docs-links';
import { INSTANCE_CAPABILITIES, TEAM_CAPABILITIES } from '@core/access/capabilities';
import { AskAIService } from '@services/ask-ai.service';
import { AskAISession, type AskAIConversationTurn, type ToolCallProgress } from './ask-ai-session';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { AccessService } from '@services/access.service';
import { TeamService } from '@services/team.service';
import { ShareService } from '@services/share.service';
import { TakeoutDownloadService } from '@services/takeout-download.service';
import { SiteService } from '@features/sites/services/site.service';
import { AskAIAnswer } from './ask-ai-answer';

interface PromptSuggestion {
    key: string;
    labelKey: string;
    icon: string;
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
    imports: [AskAIAnswer, ButtonModule, DrawerModule, MessageModule, TextareaModule, TranslocoPipe],
    providers: [AskAISession],
    templateUrl: './ask-ai-control.html',
    styleUrl: './ask-ai-control.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AskAIControl {
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

    private readonly session = inject(AskAISession);
    private readonly askAI = inject(AskAIService);
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly access = inject(AccessService);
    private readonly teams = inject(TeamService);
    private readonly siteService = inject(SiteService);
    private readonly share = inject(ShareService);
    private readonly router = inject(Router);
    private readonly takeout = inject(TakeoutDownloadService);
    private readonly document = inject(DOCUMENT);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = injectActiveLang();
    private readonly destroyRef = inject(DestroyRef);
    private readonly promptInput = viewChild<ElementRef<HTMLTextAreaElement>>('promptInput');
    private readonly transcriptScroll = viewChild<ElementRef<HTMLElement>>('transcriptScroll');
    private followTranscript = true;
    private lastSiteId: string | null = null;
    private statusRequest: Subscription | null = null;
    private actionRequest: Subscription | null = null;
    private actionSequence = 0;
    private speechRecognition: SpeechRecognitionLike | null = null;
    private dictationBaseQuery = '';

    readonly opened = output<void>();
    protected readonly mcpGuideUrl = DOCS_LINKS.mcp;
    protected readonly aiConfigurationGuideUrl = DOCS_LINKS.aiModelConfiguration;
    protected readonly query = signal('');
    protected readonly siteStatus = signal<AskAIStatus | null>(null);
    protected readonly drawerVisible = signal(false);
    protected readonly isLoading = this.session.isLoading;
    protected readonly response = this.session.response;
    protected readonly partialAnswer = this.session.partialAnswer;
    protected readonly errorKey = this.session.errorKey;
    protected readonly progressMessageKey = this.session.progressMessageKey;
    protected readonly toolProgress = this.session.toolProgress;
    protected readonly feedbackStatus = signal<{
        severity: 'info' | 'success' | 'error';
        key: string;
    } | null>(null);
    protected readonly history = this.session.history;
    protected readonly completedTurns = this.session.completedTurns;
    protected readonly currentQuestion = this.session.currentQuestion;
    protected readonly runningActionKey = signal('');
    protected readonly isDictating = signal(false);

    protected readonly activeSite = computed(() => this.siteService.activeSite());
    protected readonly activeSiteId = computed(() => this.activeSite()?.id ?? null);
    protected readonly status = computed(() => this.siteStatus() ?? this.bootstrap.status()?.ask_ai ?? null);
    protected readonly quotaRemaining = computed(() => {
        if (!this.bootstrap.cloudHosted()) return null;
        const status = this.siteStatus();
        return status?.daily_remaining === undefined || status.daily_limit === undefined ? null : { remaining: status.daily_remaining, limit: status.daily_limit };
    });
    protected readonly cloudQuotaExhausted = computed(() => this.bootstrap.cloudHosted() && this.status()?.status === 'daily_limit_exhausted');
    protected readonly selfHostedBudgetExhausted = computed(() => !this.bootstrap.cloudHosted() && this.status()?.status === 'budget_exhausted');
    protected readonly showProUpgrade = computed(() => this.quotaRemaining() !== null && this.teams.activeTeam()?.plan?.code === 'free' && (this.status()?.available || this.cloudQuotaExhausted()));
    protected readonly canManageTeam = computed(() => this.access.canActiveTeam(TEAM_CAPABILITIES.manageSettings));
    protected readonly canViewSystemStatus = computed(() => this.access.hasInstance(INSTANCE_CAPABILITIES.viewSystem));
    protected readonly quotaReset = computed(() => {
        this.activeLanguage();
        if (!this.bootstrap.cloudHosted()) return null;
        const value = this.siteStatus()?.daily_reset_at;
        if (!value) return null;
        const date = new Date(value);
        if (Number.isNaN(date.getTime())) return null;
        return `${new Intl.DateTimeFormat(this.transloco.getActiveLang(), { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' }).format(date)} UTC`;
    });
    protected readonly shouldRender = computed(() => {
        const status = this.status();
        return !this.share.isShareMode() && !!this.activeSite() && !!status?.enabled;
    });
    protected readonly canSubmit = computed(() => this.shouldRender() && !!this.status()?.available && this.query().trim().length > 0 && !this.isLoading());
    protected readonly statusLabelKey = computed(() => {
        const status = this.status();
        if (status?.available) return 'askAi.status.ready';
        if (this.cloudQuotaExhausted()) return 'askAi.quota.exhausted';
        if (this.selfHostedBudgetExhausted()) return 'askAi.budget.title';
        if (status?.status === 'budget_exhausted') return 'askAi.disabled.budget';
        if (status?.status === 'not_configured') return 'askAi.disabled.notConfigured';
        return 'askAi.disabled.unavailable';
    });
    protected readonly placeholderKey = computed(() => {
        const status = this.status();
        if (!status?.available) {
            if (this.cloudQuotaExhausted()) return 'askAi.quota.exhausted';
            if (this.selfHostedBudgetExhausted()) return 'askAi.budget.trigger';
            if (status?.status === 'budget_exhausted') return 'askAi.disabled.budget';
            if (status?.status === 'not_configured') return 'askAi.disabled.notConfigured';
            return 'askAi.disabled.unavailable';
        }
        return this.history().length > 0 ? 'askAi.followUpTriggerPlaceholder' : 'askAi.triggerPlaceholder';
    });
    protected readonly promptPlaceholderKey = computed(() => (this.history().length > 0 ? 'askAi.followUpPlaceholder' : 'askAi.promptPlaceholder'));
    protected readonly previousTurns = computed(() => (this.response() ? this.completedTurns().slice(0, -1) : this.completedTurns()));
    protected readonly hasTranscript = computed(() => this.previousTurns().length > 0 || this.currentQuestion().trim().length > 0 || this.partialAnswer().trim().length > 0 || !!this.response() || this.toolProgress().length > 0 || this.isLoading());
    protected readonly hasSession = computed(() => this.history().length > 0 || this.currentQuestion().trim().length > 0 || this.partialAnswer().trim().length > 0 || !!this.response() || !!this.errorKey() || this.isLoading());
    protected readonly canStartNewChat = computed(() => this.query().trim().length > 0 || this.hasSession());

    constructor() {
        effect(() => {
            const siteId = this.activeSiteId();
            if (siteId === this.lastSiteId) return;
            this.lastSiteId = siteId;
            this.resetSessionState();
        });
        afterRenderEffect({
            earlyRead: () => {
                this.drawerVisible();
                this.currentQuestion();
                this.partialAnswer();
                this.response();
                this.toolProgress();
                this.completedTurns();
                const element = this.transcriptScroll()?.nativeElement;
                return this.drawerVisible() && this.followTranscript && element ? { element, height: element.scrollHeight } : null;
            },
            write: (target) => {
                const next = target();
                if (next) next.element.scrollTop = next.height;
            }
        });
        this.destroyRef.onDestroy(() => {
            this.statusRequest?.unsubscribe();
            this.stopDictation(true);
        });
    }

    protected submit(event?: Event): void {
        event?.preventDefault();
        if (!this.canSubmit()) return;
        const site = this.activeSite();
        const query = this.query().trim();
        if (!site || !query) return;
        this.stopDictation(true);
        this.followTranscript = true;
        this.opened.emit();
        this.drawerVisible.set(true);
        this.query.set('');
        this.feedbackStatus.set(null);
        this.session.submit(
            site.id,
            query,
            this.router.url,
            this.explicitDateRangeFromRoute(),
            () => this.refreshStatus(site.id),
            (status) => this.siteStatus.set(status)
        );
    }

    protected openDrawer(): void {
        if (!this.shouldRender()) {
            return;
        }
        this.followTranscript = true;
        this.opened.emit();
        this.drawerVisible.set(true);
        const siteId = this.activeSiteId();
        if (siteId) this.refreshStatus(siteId);
    }

    protected onDrawerVisibleChange(visible: boolean): void {
        this.drawerVisible.set(visible);
        if (!visible) {
            this.stopDictation(true);
        }
    }

    protected startNewChat(): void {
        if (!this.canStartNewChat()) {
            return;
        }
        this.stopDictation(true);
        this.followTranscript = true;
        this.session.reset();
        this.resetAction();
        this.query.set('');
        this.feedbackStatus.set(null);
        this.runningActionKey.set('');
        this.drawerVisible.set(true);
        this.focusPrompt();
    }

    protected onTranscriptScroll(event: Event): void {
        const element = event.target as HTMLElement;
        this.followTranscript = element.scrollHeight - element.scrollTop - element.clientHeight < 64;
    }

    protected stopGenerating(): void {
        if (!this.isLoading()) {
            return;
        }
        this.session.stop();
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
        const siteId = this.activeSiteId();
        const actionId = ++this.actionSequence;
        this.actionRequest = this.takeout
            .downloadFromUrl(action.target, this.exportFilename(action))
            .pipe(
                finalize(() => {
                    if (this.runningActionKey() === actionKey && this.activeSiteId() === siteId && this.actionSequence === actionId) {
                        this.runningActionKey.set('');
                    }
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: () => {
                    if (this.activeSiteId() === siteId && this.actionSequence === actionId) this.feedbackStatus.set({ severity: 'success', key: 'askAi.exportSuccess' });
                },
                error: () => {
                    if (this.activeSiteId() === siteId && this.actionSequence === actionId) this.feedbackStatus.set({ severity: 'error', key: 'askAi.errors.export' });
                }
            });
    }

    protected actionKey(action: AskAIAction): string {
        return `${action.type}:${action.target}:${action.label}`;
    }

    protected useSuggestion(suggestion: PromptSuggestion): void {
        if (!this.status()?.available) {
            return;
        }
        this.stopDictation(true);
        this.query.set(this.transloco.translate(suggestion.labelKey));
        this.focusPrompt();
    }

    protected updateQueryFromInput(event: Event): void {
        this.query.set((event.target as HTMLTextAreaElement | null)?.value ?? '');
    }

    protected onPromptKeydown(event: KeyboardEvent): void {
        if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
            this.submit(event);
        }
    }

    protected focusPrompt(): void {
        if (this.status()?.available) this.promptInput()?.nativeElement.focus();
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
            this.focusPrompt();
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
        recognition.onresult = (event) => {
            if (this.speechRecognition === recognition) this.applyDictationResult(event);
        };
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

    protected turnTrack(turn: AskAIConversationTurn): string {
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

    private isSafeExportTarget(target: string): boolean {
        const site = this.activeSite();
        return !!site && target.startsWith(`/api/sites/${site.id}/takeout?`);
    }

    private isSafeNavigationTarget(target: string): boolean {
        return target.startsWith('/') && !target.startsWith('//') && !target.startsWith('/api/') && !target.includes('\\');
    }

    private resetSessionState(): void {
        this.statusRequest?.unsubscribe();
        this.statusRequest = null;
        this.siteStatus.set(null);
        this.stopDictation(true);
        this.followTranscript = true;
        this.session.reset();
        this.resetAction();
        this.query.set('');
        this.drawerVisible.set(false);
        this.feedbackStatus.set(null);
        this.runningActionKey.set('');
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

    protected openSystemStatus(): void {
        if (!this.selfHostedBudgetExhausted() || !this.canViewSystemStatus()) return;
        this.drawerVisible.set(false);
        void this.router.navigateByUrl('/admin/status');
    }

    protected openPlanComparison(): void {
        if (!this.showProUpgrade() || !this.canManageTeam()) return;
        this.drawerVisible.set(false);
        void this.router.navigateByUrl('/admin/team/overview');
    }

    protected refreshQuota(): void {
        const siteId = this.activeSiteId();
        if (siteId) this.refreshStatus(siteId);
    }

    private resetAction(): void {
        this.actionSequence++;
        this.actionRequest?.unsubscribe();
        this.actionRequest = null;
        this.runningActionKey.set('');
    }

    private refreshStatus(siteId: string): void {
        this.statusRequest?.unsubscribe();
        this.statusRequest = this.askAI
            .getStatus(siteId)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (status) => {
                    if (this.activeSiteId() === siteId) this.siteStatus.set(status);
                },
                error: () => {
                    if (this.activeSiteId() === siteId) this.siteStatus.set(null);
                }
            });
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
        return buildTakeoutExportFilename(this.activeSite()?.domain, 'ask-ai', action.format || 'xlsx');
    }
}
