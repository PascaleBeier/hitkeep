import { ButtonProps } from '@openng/optimus-ui/button';

export function dialogCancelButton(label: string): ButtonProps {
    return {
        label,
        severity: 'secondary',
        outlined: true,
        type: 'button'
    };
}

export function dialogPrimaryButton(label: string, props: Partial<ButtonProps> = {}): ButtonProps {
    return {
        label,
        type: 'button',
        ...props
    };
}

export function dialogDangerButton(label: string): ButtonProps {
    return dialogPrimaryButton(label, { severity: 'danger' });
}

export function dialogWarnButton(label: string): ButtonProps {
    return dialogPrimaryButton(label, { severity: 'warn' });
}
