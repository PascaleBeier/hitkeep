import { TestBed } from '@angular/core/testing';
import { TranslocoTestingModule } from '@jsverse/transloco';

import { AuditPresentationService } from './audit-presentation.service';

describe('AuditPresentationService', () => {
    let service: AuditPresentationService;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [
                TranslocoTestingModule.forRoot({
                    langs: {
                        en: {
                            auditTable: {
                                filters: {
                                    allActions: 'All actions',
                                    allTargets: 'All targets',
                                    allOutcomes: 'All outcomes'
                                },
                                actions: {
                                    askAIRequested: 'Ask AI requested',
                                    askAIResponded: 'Ask AI responded',
                                    askAIHistoryViewed: 'Ask AI history viewed'
                                },
                                targetTypes: {},
                                outcomes: {
                                    success: 'Success',
                                    failure: 'Failure',
                                    denied: 'Denied'
                                },
                                roles: {}
                            },
                            common: {
                                unknown: 'Unknown'
                            }
                        }
                    },
                    translocoConfig: {
                        availableLangs: ['en'],
                        defaultLang: 'en'
                    },
                    preloadLangs: true
                })
            ]
        }).compileComponents();

        service = TestBed.inject(AuditPresentationService);
    });

    it('presents Ask AI audit actions as first-class localized team filters', () => {
        expect(service.actionLabel('ask_ai.requested')).toBe('Ask AI requested');
        expect(service.actionLabel('ask_ai.responded')).toBe('Ask AI responded');
        expect(service.actionLabel('ask_ai.history_viewed')).toBe('Ask AI history viewed');

        const teamOptions = service.actionOptions('team');
        expect(teamOptions.some((option) => option.value === 'ask_ai.requested' && option.label === 'Ask AI requested')).toBe(true);
        expect(teamOptions.some((option) => option.value === 'ask_ai.responded' && option.label === 'Ask AI responded')).toBe(true);
        expect(teamOptions.some((option) => option.value === 'ask_ai.history_viewed' && option.label === 'Ask AI history viewed')).toBe(true);

        const systemOptions = service.actionOptions('system');
        expect(systemOptions.some((option) => option.value === 'ask_ai.requested' && option.label === 'Ask AI requested')).toBe(true);
        expect(systemOptions.some((option) => option.value === 'ask_ai.responded' && option.label === 'Ask AI responded')).toBe(true);
        expect(systemOptions.some((option) => option.value === 'ask_ai.history_viewed' && option.label === 'Ask AI history viewed')).toBe(true);
    });

    it('uses info severity for Ask AI audit actions', () => {
        expect(service.actionSeverity('ask_ai.requested')).toBe('info');
        expect(service.actionSeverity('ask_ai.responded')).toBe('info');
        expect(service.actionSeverity('ask_ai.history_viewed')).toBe('info');
    });
});
