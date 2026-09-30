import { TestBed } from '@angular/core/testing';
import { Subject } from 'rxjs';
import { AskAIStreamEvent } from '@models/analytics.types';
import { AskAIService } from '@services/ask-ai.service';
import { SiteService } from '@features/sites/services/site.service';
import { AskAISession } from './ask-ai-session';

describe('AskAISession', () => {
    it('drops preamble text when an analytics tool starts and labels the tool', () => {
        const events = new Subject<AskAIStreamEvent>();
        TestBed.configureTestingModule({
            providers: [AskAISession, { provide: AskAIService, useValue: { askStream: () => events } }, { provide: SiteService, useValue: { activeSite: () => ({ id: 'site-1' }) } }]
        });
        const session = TestBed.inject(AskAISession);
        session.submit('site-1', 'Which page led?', '/dashboard', {});

        events.next({ type: 'delta', delta_markdown: 'Let me check. ' } as AskAIStreamEvent);
        expect(session.partialAnswer()).toBe('Let me check. ');

        events.next({ type: 'progress', status: 'tool_call_start', tool_name: 'hitkeep_get_breakdown', tool_call_id: 'call-1' } as AskAIStreamEvent);
        expect(session.partialAnswer()).toBe('');
        expect(session.toolProgress()[0]?.labelKey).toBe('askAi.tools.breakdown');

        events.next({ type: 'delta', delta_markdown: '/pricing led.' } as AskAIStreamEvent);
        expect(session.partialAnswer()).toBe('/pricing led.');
    });
});
