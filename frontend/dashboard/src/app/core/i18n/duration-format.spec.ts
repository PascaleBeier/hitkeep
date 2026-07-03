import { formatDurationInterval, localeForLanguage } from './duration-format';

describe('duration-format', () => {
    it('maps Dutch to the Netherlands locale', () => {
        expect(localeForLanguage('nl')).toBe('nl-NL');
    });

    it('maps Portuguese to the Brazilian Portuguese locale', () => {
        expect(localeForLanguage('pt')).toBe('pt-BR');
    });

    it('formats Dutch duration labels through Intl', () => {
        expect(formatDurationInterval(120, 'nl', 'short')).toContain('min');
    });
});
