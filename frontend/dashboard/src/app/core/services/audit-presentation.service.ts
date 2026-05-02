import { Injectable, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { TranslocoService } from '@jsverse/transloco';

import { AuditTableOption, AuditTableRow, AuditTableSeverity } from '@components/audit-table/audit-table.types';

type AuditScope = 'system' | 'team';

const SYSTEM_ACTIONS = ['role.updated', 'mfa.disabled', 'backup.triggered', 'backup.completed', 'backup.failed', 'spam_filter.refresh', 'mail.test', 'diagnostics.export', 'access.denied'];

const SHARED_ACTIONS = [
    'auth.login_succeeded',
    'auth.login_failed',
    'auth.mfa_required',
    'auth.mfa_succeeded',
    'auth.logout',
    'auth.session_extended',
    'team.created',
    'team.updated',
    'team.archived',
    'team.active_changed',
    'member.invited',
    'member.invite_resent',
    'member.invite_revoked',
    'member.invite_accepted',
    'member.added',
    'member.role_updated',
    'member.removed',
    'member.left',
    'ownership.transferred',
    'site.created',
    'site.deleted',
    'site.retention_updated',
    'site.transferred_in',
    'site.transferred_out',
    'site.exclusion_created',
    'site.exclusion_deleted',
    'permission.site_member_granted',
    'permission.site_member_role_updated',
    'permission.site_member_revoked',
    'import.upload_created',
    'import.file_uploaded',
    'import.validation_started',
    'import.validated',
    'import.validation_failed',
    'import.queued',
    'import.requeued',
    'import.started',
    'import.failed',
    'import.completed',
    'import.deleted',
    'import.data_cleared',
    'import.data_written'
];

const ACTION_KEYS: Record<string, string> = {
    'role.updated': 'auditTable.actions.roleUpdated',
    'mfa.disabled': 'auditTable.actions.mfaDisabled',
    'backup.triggered': 'auditTable.actions.backupTriggered',
    'backup.completed': 'auditTable.actions.backupCompleted',
    'backup.failed': 'auditTable.actions.backupFailed',
    'spam_filter.refresh': 'auditTable.actions.spamRefreshed',
    'mail.test': 'auditTable.actions.mailTested',
    'diagnostics.export': 'auditTable.actions.diagnosticsExported',
    'access.denied': 'auditTable.actions.accessDenied',
    'auth.login_succeeded': 'auditTable.actions.authLoginSucceeded',
    'auth.login_failed': 'auditTable.actions.authLoginFailed',
    'auth.mfa_required': 'auditTable.actions.authMfaRequired',
    'auth.mfa_succeeded': 'auditTable.actions.authMfaSucceeded',
    'auth.logout': 'auditTable.actions.authLogout',
    'auth.session_extended': 'auditTable.actions.authSessionExtended',
    'team.created': 'auditTable.actions.teamCreated',
    'team.updated': 'auditTable.actions.teamUpdated',
    'team.archived': 'auditTable.actions.teamArchived',
    'team.active_changed': 'auditTable.actions.teamActiveChanged',
    'member.invited': 'auditTable.actions.memberInvited',
    'member.invite_accepted': 'auditTable.actions.memberInviteAccepted',
    'member.invite_resent': 'auditTable.actions.memberInviteResent',
    'member.invite_revoked': 'auditTable.actions.memberInviteRevoked',
    'member.added': 'auditTable.actions.memberAdded',
    'member.role_updated': 'auditTable.actions.memberRoleUpdated',
    'member.removed': 'auditTable.actions.memberRemoved',
    'member.left': 'auditTable.actions.memberLeft',
    'ownership.transferred': 'auditTable.actions.ownershipTransferred',
    'site.created': 'auditTable.actions.siteCreated',
    'site.deleted': 'auditTable.actions.siteDeleted',
    'site.retention_updated': 'auditTable.actions.siteRetentionUpdated',
    'site.transferred_in': 'auditTable.actions.siteTransferredIn',
    'site.transferred_out': 'auditTable.actions.siteTransferredOut',
    'site.exclusion_created': 'auditTable.actions.siteExclusionCreated',
    'site.exclusion_deleted': 'auditTable.actions.siteExclusionDeleted',
    'permission.site_member_granted': 'auditTable.actions.permissionSiteMemberGranted',
    'permission.site_member_role_updated': 'auditTable.actions.permissionSiteMemberRoleUpdated',
    'permission.site_member_revoked': 'auditTable.actions.permissionSiteMemberRevoked',
    'import.upload_created': 'auditTable.actions.importUploadCreated',
    'import.file_uploaded': 'auditTable.actions.importFileUploaded',
    'import.validation_started': 'auditTable.actions.importValidationStarted',
    'import.validated': 'auditTable.actions.importValidated',
    'import.validation_failed': 'auditTable.actions.importValidationFailed',
    'import.queued': 'auditTable.actions.importQueued',
    'import.requeued': 'auditTable.actions.importRequeued',
    'import.started': 'auditTable.actions.importStarted',
    'import.failed': 'auditTable.actions.importFailed',
    'import.completed': 'auditTable.actions.importCompleted',
    'import.deleted': 'auditTable.actions.importDeleted',
    'import.data_cleared': 'auditTable.actions.importDataCleared',
    'import.data_written': 'auditTable.actions.importDataWritten'
};

const TARGET_TYPE_KEYS: Record<string, string> = {
    system: 'auditTable.targetTypes.system',
    mail: 'auditTable.targetTypes.mail',
    user: 'auditTable.targetTypes.user',
    team: 'auditTable.targetTypes.team',
    site: 'auditTable.targetTypes.site',
    import: 'auditTable.targetTypes.import',
    site_exclusion: 'auditTable.targetTypes.siteExclusion',
    api_client: 'auditTable.targetTypes.apiClient',
    backup: 'auditTable.targetTypes.backup',
    spam_filter: 'auditTable.targetTypes.spamFilter',
    diagnostics: 'auditTable.targetTypes.diagnostics',
    permission: 'auditTable.targetTypes.permission'
};

const TARGET_TYPES = Object.keys(TARGET_TYPE_KEYS);

@Injectable({ providedIn: 'root' })
export class AuditPresentationService {
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    actionOptions(scope: AuditScope): AuditTableOption[] {
        this.activeLanguage();
        const actionValues = scope === 'system' ? [...SYSTEM_ACTIONS, ...SHARED_ACTIONS] : SHARED_ACTIONS;
        return [{ label: this.transloco.translate('auditTable.filters.allActions'), value: '' }, ...actionValues.map((value) => ({ value, label: this.actionLabel(value) }))];
    }

    targetTypeOptions(): AuditTableOption[] {
        this.activeLanguage();
        return [{ label: this.transloco.translate('auditTable.filters.allTargets'), value: '' }, ...TARGET_TYPES.map((value) => ({ value, label: this.targetTypeLabel(value) }))];
    }

    outcomeOptions(): AuditTableOption[] {
        this.activeLanguage();
        return [
            { label: this.transloco.translate('auditTable.filters.allOutcomes'), value: '' },
            { label: this.outcomeLabel('success'), value: 'success' },
            { label: this.outcomeLabel('failure'), value: 'failure' },
            { label: this.outcomeLabel('denied'), value: 'denied' }
        ];
    }

    actionLabel(action: string | null | undefined): string {
        this.activeLanguage();
        const value = action ?? '';
        return ACTION_KEYS[value] ? this.transloco.translate(ACTION_KEYS[value]) : this.humanizeAuditValue(value);
    }

    targetTypeLabel(targetType: string | null | undefined): string {
        this.activeLanguage();
        const value = targetType ?? '';
        return TARGET_TYPE_KEYS[value] ? this.transloco.translate(TARGET_TYPE_KEYS[value]) : this.humanizeAuditValue(value);
    }

    outcomeLabel(outcome: string | null | undefined): string {
        this.activeLanguage();
        const value = outcome ?? '';
        const labels: Record<string, string> = {
            success: 'auditTable.outcomes.success',
            failure: 'auditTable.outcomes.failure',
            denied: 'auditTable.outcomes.denied'
        };
        return labels[value] ? this.transloco.translate(labels[value]) : this.humanizeAuditValue(value);
    }

    roleLabel(role: string | null | undefined): string {
        this.activeLanguage();
        const value = role ?? '';
        const labels: Record<string, string> = {
            owner: 'auditTable.roles.owner',
            admin: 'auditTable.roles.admin',
            member: 'auditTable.roles.member',
            user: 'auditTable.roles.user'
        };
        return labels[value] ? this.transloco.translate(labels[value]) : this.humanizeAuditValue(value);
    }

    targetLabel(row: AuditTableRow): string {
        return row.target_label || row.target_email || row.target_id || '-';
    }

    actorLabel(row: AuditTableRow): string {
        return row.actor_email_snapshot || row.actor_email || this.transloco.translate('common.unknown');
    }

    actionSeverity(action: string | null | undefined): AuditTableSeverity {
        const value = action ?? '';
        if (
            value === 'access.denied' ||
            value.endsWith('.failed') ||
            value.endsWith('.revoked') ||
            value.endsWith('.removed') ||
            value.endsWith('.deleted') ||
            value === 'member.left' ||
            value === 'team.archived' ||
            value === 'permission.site_member_revoked'
        ) {
            return 'danger';
        }
        if (value.includes('role') || value.includes('ownership') || value.startsWith('auth.') || value.startsWith('permission.')) {
            return 'info';
        }
        if (value.includes('created') || value.includes('added') || value.includes('accepted') || value.includes('completed') || value.includes('written') || value.includes('granted')) {
            return 'success';
        }
        return 'secondary';
    }

    outcomeSeverity(outcome: string | null | undefined): AuditTableSeverity {
        switch (outcome) {
            case 'success':
                return 'success';
            case 'failure':
                return 'danger';
            case 'denied':
                return 'warn';
            default:
                return 'secondary';
        }
    }

    humanizeAuditValue(value: string | null | undefined): string {
        if (!value) {
            return '-';
        }
        return value.replace(/[._-]+/g, ' ').replace(/\b\w/g, (char) => char.toUpperCase());
    }
}
