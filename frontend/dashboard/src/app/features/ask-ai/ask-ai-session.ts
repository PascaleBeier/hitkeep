import { DestroyRef, Injectable, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { Subscription, finalize } from 'rxjs';
import { AskAIMessage, AskAIRequest, AskAIResponse, AskAIStatus, AskAIStreamEvent } from '@models/analytics.types';
import { AskAIService, AskAIStreamStatusError } from '@services/ask-ai.service';
import { SiteService } from '@features/sites/services/site.service';

export interface AskAIConversationTurn {
    id: string;
    user: string;
    response: AskAIResponse;
}

export type ToolCallProgressState = 'running' | 'done' | 'failed';

export interface ToolCallProgress {
    key: string;
    toolName: string;
    labelKey: string;
    state: ToolCallProgressState;
}

@Injectable()
export class AskAISession {
    private static readonly toolLabelKeys: Record<string, string> = {
        hitkeep_get_site_overview: 'askAi.tools.siteOverview',
        hitkeep_get_event_names: 'askAi.tools.eventNames',
        hitkeep_get_event_breakdown: 'askAi.tools.eventBreakdown',
        hitkeep_get_ecommerce: 'askAi.tools.ecommerce',
        hitkeep_get_web_vitals: 'askAi.tools.webVitals',
        hitkeep_get_ai_visibility: 'askAi.tools.aiVisibility',
        hitkeep_get_annotations: 'askAi.tools.annotations'
    };

    private readonly askAI = inject(AskAIService);
    private readonly siteService = inject(SiteService);
    private readonly destroyRef = inject(DestroyRef);
    private activeRequest: Subscription | null = null;
    private requestSequence = 0;
    private toolProgressSequence = 0;

    readonly isLoading = signal(false);
    readonly response = signal<AskAIResponse | null>(null);
    readonly partialAnswer = signal('');
    readonly errorKey = signal<string | null>(null);
    readonly progressMessageKey = signal('askAi.loading');
    readonly toolProgress = signal<ToolCallProgress[]>([]);
    readonly history = signal<AskAIMessage[]>([]);
    readonly completedTurns = signal<AskAIConversationTurn[]>([]);
    readonly currentQuestion = signal('');

    get version(): number {
        return this.requestSequence;
    }

    submit(siteId: string, query: string, route: string, dates: Pick<AskAIRequest, 'from' | 'to'>, onSettled?: () => void, onStatusError?: (status: AskAIStatus) => void): void {
        this.cancelActiveRequest();
        const requestId = ++this.requestSequence;
        const request: AskAIRequest = { query, route, history: this.history(), ...dates };
        this.currentQuestion.set(query);
        this.response.set(null);
        this.partialAnswer.set('');
        this.errorKey.set(null);
        this.progressMessageKey.set('askAi.progress.accepted');
        this.toolProgress.set([]);
        this.toolProgressSequence = 0;
        this.isLoading.set(true);

        let subscription: Subscription | null = null;
        let sawTerminalEvent = false;
        subscription = this.askAI
            .askStream(siteId, request)
            .pipe(
                finalize(() => {
                    if (subscription && this.activeRequest === subscription) this.activeRequest = null;
                    if (this.isCurrentRequest(requestId, siteId)) {
                        this.isLoading.set(false);
                        onSettled?.();
                    }
                }),
                takeUntilDestroyed(this.destroyRef)
            )
            .subscribe({
                next: (event) => {
                    if (!this.isCurrentRequest(requestId, siteId)) return;
                    if (event.type === 'final' || event.type === 'error') {
                        sawTerminalEvent = true;
                        this.isLoading.set(false);
                    }
                    this.applyStreamEvent(event, query);
                },
                error: (error) => {
                    if (!this.isCurrentRequest(requestId, siteId)) return;
                    this.partialAnswer.set('');
                    this.finishRunningToolProgress('failed');
                    if (error instanceof AskAIStreamStatusError && error.askAIStatus) onStatusError?.(error.askAIStatus);
                    this.errorKey.set(this.streamErrorKey(error));
                },
                complete: () => {
                    if (!this.isCurrentRequest(requestId, siteId) || sawTerminalEvent) return;
                    this.partialAnswer.set('');
                    this.progressMessageKey.set('askAi.loading');
                    this.finishRunningToolProgress('failed');
                    this.errorKey.set('askAi.errors.request');
                }
            });
        this.activeRequest = subscription;
    }

    stop(): void {
        if (!this.isLoading()) return;
        this.cancelActiveRequest();
        this.requestSequence++;
        this.isLoading.set(false);
        this.progressMessageKey.set('askAi.loading');
        this.finishRunningToolProgress('failed');
    }

    reset(): void {
        this.cancelActiveRequest();
        this.requestSequence++;
        this.currentQuestion.set('');
        this.isLoading.set(false);
        this.response.set(null);
        this.partialAnswer.set('');
        this.errorKey.set(null);
        this.progressMessageKey.set('askAi.loading');
        this.toolProgress.set([]);
        this.toolProgressSequence = 0;
        this.history.set([]);
        this.completedTurns.set([]);
    }

    private applyStreamEvent(event: AskAIStreamEvent, query: string): void {
        if (event.type === 'progress') {
            this.progressMessageKey.set(event.message_key || 'askAi.loading');
            this.applyToolProgress(event);
            return;
        }
        if (event.type === 'delta') {
            this.progressMessageKey.set('askAi.progress.composing');
            if (event.delta_markdown) this.partialAnswer.update((answer) => answer + event.delta_markdown);
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
            this.completedTurns.update((turns) => [...turns, { id: response.run_id, user: query, response }].slice(-4));
            this.history.update((history) => [...history, { role: 'user' as const, content: query }, { role: 'assistant' as const, content: response.answer_markdown }].slice(-8));
        }
    }

    private applyToolProgress(event: AskAIStreamEvent): void {
        if (event.status !== 'tool_call_start' && event.status !== 'tool_call_finish') return;
        const toolName = event.tool_name?.trim();
        if (!toolName) return;
        if (event.status === 'tool_call_start') {
            const key = event.tool_call_id?.trim() || `${toolName}:${++this.toolProgressSequence}`;
            const next: ToolCallProgress = { key, toolName, labelKey: this.toolLabelKey(toolName), state: 'running' };
            this.toolProgress.update((progress) => {
                const existingIndex = progress.findIndex((item) => item.key === key);
                if (existingIndex === -1) return [...progress, next];
                return progress.map((item, index) => (index === existingIndex ? { ...item, state: 'running' } : item));
            });
            return;
        }
        this.toolProgress.update((progress) => {
            const index = this.findToolProgressIndex(progress, event);
            if (index === -1) return [...progress, { key: event.tool_call_id?.trim() || `${toolName}:${++this.toolProgressSequence}`, toolName, labelKey: this.toolLabelKey(toolName), state: 'done' }];
            return progress.map((item, itemIndex) => (itemIndex === index ? { ...item, state: 'done' } : item));
        });
    }

    private findToolProgressIndex(progress: ToolCallProgress[], event: AskAIStreamEvent): number {
        const toolCallID = event.tool_call_id?.trim();
        if (toolCallID) {
            const byID = progress.findIndex((item) => item.key === toolCallID);
            if (byID !== -1) return byID;
        }
        const toolName = event.tool_name?.trim();
        if (!toolName) return -1;
        for (let index = progress.length - 1; index >= 0; index--) {
            const item = progress[index];
            if (item?.toolName === toolName && item.state === 'running') return index;
        }
        return -1;
    }

    private finishRunningToolProgress(state: Exclude<ToolCallProgressState, 'running'>): void {
        this.toolProgress.update((progress) => progress.map((item) => (item.state === 'running' ? { ...item, state } : item)));
    }

    private toolLabelKey(toolName: string): string {
        return AskAISession.toolLabelKeys[toolName] ?? 'askAi.tools.analytics';
    }

    private streamErrorKey(error: unknown): string {
        if (error instanceof AskAIStreamStatusError) {
            const status = error.askAIStatus?.status;
            if (status === 'daily_limit_exhausted') return 'askAi.quota.exhausted';
            if (status === 'budget_exhausted' || error.statusCode === 429) return 'askAi.errors.budget';
            if (status === 'not_configured' || status === 'disabled') return 'askAi.errors.notConfigured';
        }
        return 'askAi.errors.request';
    }

    private cancelActiveRequest(): void {
        this.activeRequest?.unsubscribe();
        this.activeRequest = null;
    }

    private isCurrentRequest(requestId: number, siteId: string): boolean {
        return requestId === this.requestSequence && this.siteService.activeSite()?.id === siteId;
    }
}
