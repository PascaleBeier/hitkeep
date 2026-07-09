import { AbstractControl, ValidationErrors } from '@angular/forms';

export function domainValidator(control: AbstractControl): ValidationErrors | null {
    const value = control.value as string;
    if (!value) return null;

    if (value.startsWith('http://') || value.startsWith('https://')) {
        return { containsProtocol: true };
    }

    if (value.startsWith('www.')) {
        return { containsWww: true };
    }

    const domainRegex = /^[a-zA-Z0-9][a-zA-Z0-9-]{1,61}[a-zA-Z0-9](?:\.[a-zA-Z]{2,})+$/;
    if (!domainRegex.test(value)) {
        return { pattern: true };
    }

    return null;
}

export function sanitizeDomainInput(value: string): string {
    return value
        .toLowerCase()
        .trim()
        .replace(/^https?:\/\//, '')
        .replace(/\/$/, '');
}
