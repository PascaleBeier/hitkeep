import { TestBed } from '@angular/core/testing';
import { firstValueFrom, toArray } from 'rxjs';
import { vi } from 'vitest';
import { AskAIService, AskAIStreamStatusError } from './ask-ai.service';

describe('AskAIService', () => {
    afterEach(() => {
        vi.unstubAllGlobals();
    });

    it('streams Ask AI progress and final response from the SSE endpoint', async () => {
        TestBed.configureTestingModule({});
        const encoder = new TextEncoder();
        const body = new ReadableStream<Uint8Array>({
            start(controller) {
                controller.enqueue(encoder.encode('event: progress\ndata: {"type":"progress","status":"accepted","message_key":"askAi.progress.accepted"}\n\n'));
                controller.enqueue(encoder.encode('event: delta\ndata: {"type":"delta","status":"streaming","delta_markdown":"Traffic "}\n\n'));
                controller.enqueue(encoder.encode('event: delta\ndata: {"type":"delta","status":"streaming","delta_markdown":"increased."}\n\n'));
                controller.enqueue(encoder.encode('event: final\ndata: {"type":"final","status":"success","response":{"run_id":"run_123","answer_markdown":"Traffic increased.","citations":[],"charts":[],"actions":[]}}\n\n'));
                controller.close();
            }
        });
        const fetchMock = vi.fn().mockResolvedValue(
            new Response(body, {
                status: 200,
                headers: { 'Content-Type': 'text/event-stream' }
            })
        );
        vi.stubGlobal('fetch', fetchMock);

        const service = TestBed.inject(AskAIService);
        const events = await firstValueFrom(
            service
                .askStream('site_123', {
                    query: 'What changed?',
                    from: '2026-06-01',
                    to: '2026-06-25'
                })
                .pipe(toArray())
        );

        const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
        expect(url).toBe('/api/sites/site_123/ask-ai/events');
        expect(init.method).toBe('POST');
        expect(init.credentials).toBe('same-origin');
        expect(init.body).toBe(
            JSON.stringify({
                query: 'What changed?',
                from: '2026-06-01',
                to: '2026-06-25'
            })
        );
        expect(events).toEqual([
            {
                type: 'progress',
                status: 'accepted',
                message_key: 'askAi.progress.accepted'
            },
            { type: 'delta', status: 'streaming', delta_markdown: 'Traffic ' },
            { type: 'delta', status: 'streaming', delta_markdown: 'increased.' },
            {
                type: 'final',
                status: 'success',
                response: {
                    run_id: 'run_123',
                    answer_markdown: 'Traffic increased.',
                    citations: [],
                    charts: [],
                    actions: []
                }
            }
        ]);
    });

    it('parses CRLF-delimited stream events from intermediaries', async () => {
        TestBed.configureTestingModule({});
        const encoder = new TextEncoder();
        const body = new ReadableStream<Uint8Array>({
            start(controller) {
                controller.enqueue(encoder.encode('event: progress\r\ndata: {"type":"progress","status":"accepted"}\r\n\r\n'));
                controller.enqueue(encoder.encode('event: final\r\ndata: {"type":"final","status":"success","response":{"run_id":"run_456","answer_markdown":"Done.","citations":[],"charts":[],"actions":[]}}\r\n\r\n'));
                controller.close();
            }
        });
        vi.stubGlobal(
            'fetch',
            vi.fn().mockResolvedValue(
                new Response(body, {
                    status: 200,
                    headers: { 'Content-Type': 'text/event-stream' }
                })
            )
        );

        const events = await firstValueFrom(TestBed.inject(AskAIService).askStream('site_123', { query: 'What changed?' }).pipe(toArray()));

        expect(events.map((event) => event.type)).toEqual(['progress', 'final']);
        expect(events[1]?.response?.answer_markdown).toBe('Done.');
    });

    it('fails the stream when the SSE response closes without a terminal event', async () => {
        TestBed.configureTestingModule({});
        const encoder = new TextEncoder();
        const body = new ReadableStream<Uint8Array>({
            start(controller) {
                controller.enqueue(encoder.encode('event: progress\ndata: {"type":"progress","status":"generating"}\n\n'));
                controller.close();
            }
        });
        vi.stubGlobal(
            'fetch',
            vi.fn().mockResolvedValue(
                new Response(body, {
                    status: 200,
                    headers: { 'Content-Type': 'text/event-stream' }
                })
            )
        );

        try {
            await firstValueFrom(TestBed.inject(AskAIService).askStream('site_123', { query: 'What changed?' }).pipe(toArray()));
            throw new Error('expected Ask AI stream to fail');
        } catch (error) {
            expect(String(error)).toContain('terminal event');
        }
    });

    it('throws a typed Ask AI status error when stream preflight returns JSON status', async () => {
        TestBed.configureTestingModule({});
        vi.stubGlobal(
            'fetch',
            vi.fn().mockResolvedValue(
                new Response(
                    JSON.stringify({
                        enabled: true,
                        available: false,
                        status: 'budget_exhausted',
                        budget_exhausted: true
                    }),
                    {
                        status: 429,
                        headers: { 'Content-Type': 'application/json' }
                    }
                )
            )
        );

        try {
            await firstValueFrom(
                TestBed.inject(AskAIService).askStream('site_123', {
                    query: 'What changed?'
                })
            );
            throw new Error('expected Ask AI stream to fail');
        } catch (error) {
            expect(error).toBeInstanceOf(AskAIStreamStatusError);
            expect((error as AskAIStreamStatusError).statusCode).toBe(429);
            expect((error as AskAIStreamStatusError).askAIStatus?.status).toBe('budget_exhausted');
        }
    });
});
