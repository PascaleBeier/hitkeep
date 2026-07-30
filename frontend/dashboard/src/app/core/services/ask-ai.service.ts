import { Service } from '@angular/core';
import { Observable, Subscriber } from 'rxjs';
import { AskAIRequest, AskAIStatus, AskAIStreamEvent } from '@models/analytics.types';

export class AskAIStreamStatusError extends Error {
    constructor(
        readonly statusCode: number,
        readonly askAIStatus: AskAIStatus | null = null
    ) {
        super(`Ask AI stream failed with status ${statusCode}`);
        this.name = 'AskAIStreamStatusError';
    }
}

@Service()
export class AskAIService {
    askStream(siteId: string, request: AskAIRequest): Observable<AskAIStreamEvent> {
        return new Observable<AskAIStreamEvent>((subscriber) => {
            const controller = new AbortController();
            void this.fetchAskAIStream(siteId, request, controller, subscriber);
            return () => controller.abort();
        });
    }

    private async fetchAskAIStream(siteId: string, request: AskAIRequest, controller: AbortController, subscriber: Subscriber<AskAIStreamEvent>): Promise<void> {
        try {
            const response = await fetch(`/api/sites/${encodeURIComponent(siteId)}/ask-ai/events`, {
                method: 'POST',
                credentials: 'same-origin',
                headers: {
                    Accept: 'text/event-stream',
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(request),
                signal: controller.signal
            });
            if (!response.ok) {
                throw await this.streamStatusError(response);
            }
            if (!response.body) {
                throw new AskAIStreamStatusError(response.status);
            }
            const sawTerminalEvent = await this.readStreamEvents(response.body, subscriber);
            if (!sawTerminalEvent) {
                throw new Error('Ask AI stream ended without a terminal event');
            }
            if (!subscriber.closed) {
                subscriber.complete();
            }
        } catch (error) {
            if (!controller.signal.aborted && !subscriber.closed) {
                subscriber.error(error);
            }
        }
    }

    private async streamStatusError(response: Response): Promise<AskAIStreamStatusError> {
        let status: AskAIStatus | null = null;
        const contentType = response.headers.get('Content-Type') ?? '';
        if (contentType.toLowerCase().includes('application/json')) {
            try {
                status = (await response.json()) as AskAIStatus;
            } catch {
                status = null;
            }
        }
        return new AskAIStreamStatusError(response.status, status);
    }

    private async readStreamEvents(body: ReadableStream<Uint8Array>, subscriber: Subscriber<AskAIStreamEvent>): Promise<boolean> {
        const reader = body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        let sawTerminalEvent = false;

        try {
            for (;;) {
                const { value, done } = await reader.read();
                if (done) break;
                buffer += decoder.decode(value, { stream: true });
                buffer = buffer.replace(/\r\n/g, '\n');
                const emitted = this.emitBufferedEvents(buffer, subscriber);
                buffer = emitted.buffer;
                sawTerminalEvent ||= emitted.terminal;
                if (subscriber.closed) break;
            }
            buffer += decoder.decode();
            buffer = buffer.replace(/\r\n/g, '\n');
            const emitted = this.emitBufferedEvents(buffer, subscriber, true);
            sawTerminalEvent ||= emitted.terminal;
        } finally {
            reader.releaseLock();
        }
        return sawTerminalEvent;
    }

    private emitBufferedEvents(buffer: string, subscriber: Subscriber<AskAIStreamEvent>, flush = false): { buffer: string; terminal: boolean } {
        let cursor = 0;
        let terminal = false;
        for (;;) {
            const next = buffer.indexOf('\n\n', cursor);
            if (next === -1) break;
            terminal ||= this.emitStreamEvent(buffer.slice(cursor, next), subscriber);
            cursor = next + 2;
            if (subscriber.closed) {
                return { buffer: '', terminal };
            }
        }
        const remaining = buffer.slice(cursor);
        if (flush && remaining.trim()) {
            terminal ||= this.emitStreamEvent(remaining, subscriber);
            return { buffer: '', terminal };
        }
        return { buffer: remaining, terminal };
    }

    private emitStreamEvent(block: string, subscriber: Subscriber<AskAIStreamEvent>): boolean {
        const data = block
            .split(/\r?\n/)
            .filter((line) => line.startsWith('data:'))
            .map((line) => line.slice(5).trimStart())
            .join('\n');
        if (!data) return false;
        const event = JSON.parse(data) as AskAIStreamEvent;
        subscriber.next(event);
        return event.type === 'final' || event.type === 'error';
    }
}
