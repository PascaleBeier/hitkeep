import { AbstractControl, ValidationErrors } from '@angular/forms';

export function domainValidator(control: AbstractControl): ValidationErrors | null {
    const value = control.value as string;
    if (!value) return null;
    if (typeof value !== 'string') return { pattern: true };

    const normalizedValue = value.toLowerCase();

    if (normalizedValue.includes('://')) {
        return { containsProtocol: true };
    }

    if (normalizedValue.startsWith('www.')) {
        return { containsWww: true };
    }

    const domainRegex = /^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$/;
    if (value.length > 253 || isIPv4Address(normalizedValue) || !domainRegex.test(value)) {
        return { pattern: true };
    }

    return null;
}

function isIPv4Address(value: string): boolean {
    const parts = value.split('.');
    return parts.length === 4 && parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) <= 255);
}

export function sanitizeDomainInput(value: string): string {
    return value
        .toLowerCase()
        .trim()
        .replace(/^https?:\/\//, '')
        .replace(/\/$/, '');
}
