import { dialogCancelButton, dialogDangerButton, dialogPrimaryButton, dialogWarnButton } from './dialog-actions';

describe('dialog action helpers', () => {
    it('uses secondary outlined cancel actions for OptimusUI dialogs', () => {
        expect(dialogCancelButton('Cancel')).toEqual({
            label: 'Cancel',
            severity: 'secondary',
            outlined: true,
            type: 'button'
        });
    });

    it('uses explicit primary, warning, and danger accept actions', () => {
        expect(dialogPrimaryButton('Save changes')).toEqual({
            label: 'Save changes',
            type: 'button'
        });
        expect(dialogWarnButton('Disable 2FA')).toEqual({
            label: 'Disable 2FA',
            type: 'button',
            severity: 'warn'
        });
        expect(dialogDangerButton('Delete')).toEqual({
            label: 'Delete',
            type: 'button',
            severity: 'danger'
        });
    });
});
