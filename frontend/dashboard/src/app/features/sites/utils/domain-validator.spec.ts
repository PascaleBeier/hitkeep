import { FormControl } from '@angular/forms';

import { domainValidator, sanitizeDomainInput } from './domain-validator';

describe('domainValidator', () => {
    it('accepts valid multi-label hostnames', () => {
        const values = ['example.com', 'sub.example-app.com.br', 'app-staging.example-service.co.uk', 'a.example.com', 'api2.service-3.example.com', 'example.xn--p1ai', 'www2.example.com', 'blog.www.example.com'];

        for (const value of values) {
            expect(domainValidator(new FormControl(value))).toBeNull();
        }
    });

    it('rejects values outside the hostname contract', () => {
        const values = [
            { value: 'http://example.com', error: { containsProtocol: true } },
            { value: 'https://example.com', error: { containsProtocol: true } },
            { value: 'example.com/path', error: { pattern: true } },
            { value: 'example.com:443', error: { pattern: true } },
            { value: 'example.com?query=1', error: { pattern: true } },
            { value: 'example.com#section', error: { pattern: true } },
            { value: '*.example.com', error: { pattern: true } },
            { value: '192.0.2.1', error: { pattern: true } },
            { value: '2001:db8::1', error: { pattern: true } },
            { value: 'localhost', error: { pattern: true } },
            { value: 'münich.de', error: { pattern: true } },
            { value: '.example.com', error: { pattern: true } },
            { value: 'example.com.', error: { pattern: true } },
            { value: 'example..com', error: { pattern: true } },
            { value: '-example.com', error: { pattern: true } },
            { value: 'example-.com', error: { pattern: true } },
            { value: 'inva lid.com', error: { pattern: true } },
            { value: 'a'.repeat(64) + '.example', error: { pattern: true } },
            { value: 'a.'.repeat(126) + 'a.com', error: { pattern: true } }
        ];

        for (const { value, error } of values) {
            expect(domainValidator(new FormControl(value))).toEqual(error);
        }
    });

    it('reports protocol and www prefix errors case-insensitively', () => {
        expect(domainValidator(new FormControl('HTTPS://example.com'))).toEqual({ containsProtocol: true });
        expect(domainValidator(new FormControl('WWW.example.com'))).toEqual({ containsWww: true });
    });

    it('keeps the existing input sanitizer behavior', () => {
        expect(sanitizeDomainInput('  HTTPS://Example.COM/  ')).toBe('example.com');
        expect(sanitizeDomainInput('Example.COM/')).toBe('example.com');
    });
});
