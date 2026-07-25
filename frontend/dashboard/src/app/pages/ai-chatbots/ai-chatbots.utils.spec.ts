import { computeComparisonPeriod, createEmptySeries, mergeChatbotChartSeries, totalFor } from '@pages/ai-chatbots/ai-chatbots.utils';

describe('AI chatbot utils', () => {
    it('creates an empty series state', () => {
        expect(createEmptySeries()).toEqual({
            started: [],
            sent: [],
            rendered: [],
            clicked: [],
            handoff: [],
            assisted: []
        });
    });

    it('computes the previous comparison window', () => {
        expect(computeComparisonPeriod('2026-03-10T00:00:00.000Z', '2026-03-20T00:00:00.000Z')).toEqual({
            from: '2026-02-27T23:59:59.999Z',
            to: '2026-03-09T23:59:59.999Z'
        });
    });

    it('totals a metric series', () => {
        const state = createEmptySeries();
        state.started = [
            { time: '2026-03-18T00:00:00Z', count: 3 },
            { time: '2026-03-19T00:00:00Z', count: 4 }
        ];

        expect(totalFor('started', state)).toBe(7);
    });

    describe('mergeChatbotChartSeries', () => {
        it('merges charted metrics by timestamp', () => {
            const state = createEmptySeries();
            state.started = [{ time: '2026-03-18T00:00:00Z', count: 5 }];
            state.rendered = [{ time: '2026-03-18T00:00:00Z', count: 4 }];
            state.handoff = [{ time: '2026-03-18T00:00:00Z', count: 1 }];
            state.assisted = [{ time: '2026-03-18T00:00:00Z', count: 2 }];

            expect(mergeChatbotChartSeries(state)).toEqual([
                {
                    time: '2026-03-18T00:00:00Z',
                    started: 5,
                    rendered: 4,
                    handoff: 1,
                    assisted: 2
                }
            ]);
        });

        it('defaults missing metrics to zero and sorts points chronologically', () => {
            const state = createEmptySeries();
            state.started = [
                { time: '2026-03-19T00:00:00Z', count: 7 },
                { time: '2026-03-17T00:00:00Z', count: 3 }
            ];
            state.rendered = [{ time: '2026-03-18T00:00:00Z', count: 9 }];

            expect(mergeChatbotChartSeries(state)).toEqual([
                { time: '2026-03-17T00:00:00Z', started: 3, rendered: 0, handoff: 0, assisted: 0 },
                { time: '2026-03-18T00:00:00Z', started: 0, rendered: 9, handoff: 0, assisted: 0 },
                { time: '2026-03-19T00:00:00Z', started: 7, rendered: 0, handoff: 0, assisted: 0 }
            ]);
        });

        it('ignores metrics that are not charted', () => {
            const state = createEmptySeries();
            state.sent = [{ time: '2026-03-18T00:00:00Z', count: 12 }];
            state.clicked = [{ time: '2026-03-18T00:00:00Z', count: 6 }];

            expect(mergeChatbotChartSeries(state)).toEqual([]);
        });

        it('returns an empty list for an empty series state', () => {
            expect(mergeChatbotChartSeries(createEmptySeries())).toEqual([]);
        });
    });
});
