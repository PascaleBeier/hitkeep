import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';

import { AIAgentIconsService } from './ai-agent-icons.service';

describe('AIAgentIconsService', () => {
    let service: AIAgentIconsService;
    let httpMock: HttpTestingController;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [provideHttpClient(), provideHttpClientTesting()]
        });
        service = TestBed.inject(AIAgentIconsService);
        httpMock = TestBed.inject(HttpTestingController);
    });

    afterEach(() => {
        httpMock.verify();
    });

    it('loads the catalog once and resolves agent and referrer icon hosts', () => {
        expect(service.agentIconHost('GPTBot')).toBeNull();

        const req = httpMock.expectOne('/api/ai-agents');
        expect(req.request.method).toBe('GET');
        req.flush({
            generated_at: '2026-07-25T00:00:00Z',
            agents: [
                { name: 'GPTBot', family: 'OpenAI', category: 'ai_training_crawler', icon_host: 'openai.com' },
                { name: 'MysteryBot', family: 'Mystery', category: 'other_ai' }
            ],
            ai_referrers: [{ name: 'ChatGPT', icon_host: 'chatgpt.com' }]
        });

        expect(service.agentIconHost('GPTBot')).toBe('openai.com');
        expect(service.agentIconHost('MysteryBot')).toBeNull();
        expect(service.referrerIconHost('ChatGPT')).toBe('chatgpt.com');

        // Further lookups must not refetch the catalog.
        expect(service.agentIconHost('GPTBot')).toBe('openai.com');
        httpMock.expectNone('/api/ai-agents');
    });

    it('stays icon-less and retries later when the catalog request fails', () => {
        expect(service.agentIconHost('GPTBot')).toBeNull();
        httpMock.expectOne('/api/ai-agents').flush(null, { status: 503, statusText: 'Service Unavailable' });

        expect(service.agentIconHost('GPTBot')).toBeNull();
        httpMock.expectOne('/api/ai-agents').flush({
            generated_at: '2026-07-25T00:00:00Z',
            agents: [{ name: 'GPTBot', family: 'OpenAI', category: 'ai_training_crawler', icon_host: 'openai.com' }],
            ai_referrers: []
        });
        expect(service.agentIconHost('GPTBot')).toBe('openai.com');
    });
});
