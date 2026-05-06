import { countryFlagUrl, languageFlagUrl, localeFlagUrl } from './flag-utils';

describe('flag-utils', () => {
    it('should resolve country flags directly', () => {
        expect(countryFlagUrl('DE')).toBe('/flags/de.svg');
    });

    it('should resolve representative flags for language codes', () => {
        expect(languageFlagUrl('en')).toBe('/flags/gb.svg');
        expect(languageFlagUrl('cs')).toBe('/flags/cz.svg');
        expect(languageFlagUrl('nl')).toBe('/flags/nl.svg');
    });

    it('should use dedicated language assets when available', () => {
        expect(languageFlagUrl('nb')).toBe('/flags/language/non.svg');
        expect(languageFlagUrl('nn')).toBe('/flags/language/non.svg');
    });

    it('should prefer explicit locale regions over language fallbacks', () => {
        expect(localeFlagUrl('en-US')).toBe('/flags/us.svg');
        expect(localeFlagUrl('pt-BR')).toBe('/flags/br.svg');
    });

    it('should fall back to earth for invalid values', () => {
        expect(countryFlagUrl('')).toBe('/flags/other/earth.svg');
        expect(languageFlagUrl('???')).toBe('/flags/other/earth.svg');
        expect(localeFlagUrl('')).toBe('/flags/other/earth.svg');
    });
});
