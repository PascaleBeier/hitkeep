import { formatBytes, formatResponseMs } from '@pages/ai-agents/ai-agents-page.utils';

describe('AI agents page utils', () => {
    describe('formatBytes', () => {
        it('renders zero and negative sizes as plain bytes', () => {
            expect(formatBytes(0, 'en-US')).toBe('0 B');
            expect(formatBytes(-12, 'en-US')).toBe('0 B');
        });

        it('scales into the largest fitting unit', () => {
            expect(formatBytes(512, 'en-US')).toBe('512 B');
            expect(formatBytes(1024, 'en-US')).toBe('1 KB');
            expect(formatBytes(1536, 'en-US')).toBe('1.5 KB');
            expect(formatBytes(1024 * 1024 * 2.5, 'en-US')).toBe('2.5 MB');
        });

        it('drops the fraction once the value passes ten units', () => {
            expect(formatBytes(1024 * 12.4, 'en-US')).toBe('12 KB');
        });

        it('formats the number for the requested locale', () => {
            expect(formatBytes(1536, 'de-DE')).toBe('1,5 KB');
        });
    });

    describe('formatResponseMs', () => {
        it('renders zero and negative durations as zero milliseconds', () => {
            expect(formatResponseMs(0, 'en-US')).toBe('0 ms');
            expect(formatResponseMs(-5, 'en-US')).toBe('0 ms');
        });

        it('rounds to whole milliseconds and groups for the locale', () => {
            expect(formatResponseMs(842.6, 'en-US')).toBe('843 ms');
            expect(formatResponseMs(1842, 'en-US')).toBe('1,842 ms');
            expect(formatResponseMs(1842, 'de-DE')).toBe('1.842 ms');
        });
    });
});
