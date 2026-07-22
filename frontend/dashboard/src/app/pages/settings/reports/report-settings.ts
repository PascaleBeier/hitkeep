import { ChangeDetectionStrategy, Component, ElementRef, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { finalize } from 'rxjs';
import { ConfirmationService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { ConfirmDialogModule } from 'primeng/confirmdialog';
import { IconFieldModule } from 'primeng/iconfield';
import { InputIconModule } from 'primeng/inputicon';
import { InputTextModule } from 'primeng/inputtext';
import { MessageModule } from 'primeng/message';
import { MultiSelectModule } from 'primeng/multiselect';
import { SelectModule } from 'primeng/select';
import { TableModule } from 'primeng/table';
import { TagModule } from 'primeng/tag';
import { CrudDialog } from '@components/crud-dialog/crud-dialog';
import { dialogCancelButton, dialogDangerButton } from '@components/dialog-actions/dialog-actions';
import { DialogShell } from '@components/dialog-shell/dialog-shell';
import { PageBreadcrumb, PageBreadcrumbItem } from '@components/page-breadcrumb/page-breadcrumb';
import { PageHeader, PageHeaderLeft, PageHeaderRight } from '@components/page-header/page-header';
import { TableRowActionItem, TableRowActions } from '@components/table-row-actions/table-row-actions';
import { SiteSelectOption } from '@features/sites/components/site-select-option';
import { SiteService } from '@features/sites/services/site.service';
import { injectActiveLang } from '@core/i18n/active-lang';
import { ReportDefinition, ReportDefinitionInput, ReportDelivery, ReportFrequency, ReportPreset, ReportPreview, ReportRecipient, ReportRun, ReportScope, ReportStatus, TeamMember } from '@models/analytics.types';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { ReportDefinitionsService } from '@services/report-definitions.service';
import { TeamService } from '@services/team.service';
import { UserProfileService } from '@services/user-profile.service';

interface ReportFeedback {
    key: string;
    severity: 'error' | 'success';
}

interface ReportTableRow {
    report: ReportDefinition;
    name: string;
    preset: string;
    scope: string;
    sites: string;
    recipients: string;
    schedule: string;
    status: string;
    nextRun: string;
    lastOutcome: string;
}

@Component({
    selector: 'app-report-settings',
    imports: [
        FormsModule,
        RouterLink,
        ButtonModule,
        ConfirmDialogModule,
        IconFieldModule,
        InputIconModule,
        InputTextModule,
        MessageModule,
        MultiSelectModule,
        SelectModule,
        TableModule,
        TagModule,
        CrudDialog,
        DialogShell,
        PageHeader,
        PageHeaderLeft,
        PageHeaderRight,
        PageBreadcrumb,
        SiteSelectOption,
        TableRowActions,
        TranslocoPipe
    ],
    providers: [ConfirmationService],
    templateUrl: './report-settings.html',
    styleUrl: './report-settings.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class ReportSettings {
    private readonly service = inject(ReportDefinitionsService);
    private readonly bootstrap = inject(DashboardBootstrapService);
    private readonly teamService = inject(TeamService);
    private readonly profileService = inject(UserProfileService);
    private readonly siteService = inject(SiteService);
    private readonly transloco = inject(TranslocoService);
    private readonly activeLanguage = injectActiveLang();
    private readonly route = inject(ActivatedRoute);
    private readonly elementRef = inject<ElementRef<HTMLElement>>(ElementRef);
    private readonly confirmationService = inject(ConfirmationService);

    protected readonly reports = this.service.reports;
    protected readonly isLoading = this.service.isLoading;
    protected readonly sites = this.siteService.sites;
    protected readonly mailAvailable = computed(() => this.bootstrap.status()?.mail_delivery?.available === true);
    protected readonly currentUserID = computed(() => this.profileService.profile()?.id ?? '');
    protected readonly canCreateTeamReport = computed(() => ['owner', 'admin'].includes(this.teamService.activeTeam()?.role ?? ''));
    protected readonly externalRecipientsLocked = computed(() => this.externalRecipientsLockedForTeam(this.draft().scope === 'team' ? this.draft().tenant_id : undefined));
    protected readonly members = signal<TeamMember[]>([]);
    protected readonly memberOptions = computed(() => this.members().map((member) => ({ label: member.email, value: member.user_id })));
    protected readonly editorVisible = signal(false);
    protected readonly historyVisible = signal(false);
    protected readonly editingReportID = signal<string | null>(null);
    protected readonly preview = signal<ReportPreview | null>(null);
    protected readonly runs = signal<ReportRun[]>([]);
    protected readonly historyReport = signal<ReportDefinition | null>(null);
    protected readonly saveState = signal<'idle' | 'saving'>('idle');
    protected readonly reportActionID = signal<string | null>(null);
    protected readonly historyActionID = signal<string | null>(null);
    protected readonly historyLoading = signal(false);
    protected readonly listFeedback = signal<ReportFeedback | null>(null);
    protected readonly dialogFeedback = signal<ReportFeedback | null>(null);
    protected readonly historyFeedback = signal<ReportFeedback | null>(null);
    protected readonly draft = signal<ReportDefinitionInput>(this.emptyDraft());
    protected readonly externalEmailInput = signal('');
    protected readonly externalEmailError = signal('');
    protected readonly focusedReportID = signal<string | null>(null);

    protected readonly breadcrumbItems = computed<PageBreadcrumbItem[]>(() => {
        this.activeLanguage();
        return [{ label: this.transloco.translate('settings.reports.breadcrumb'), isCurrent: true }];
    });
    protected readonly reportTableRows = computed<ReportTableRow[]>(() => {
        this.activeLanguage();
        return this.reports().map((report) => ({
            report,
            name: report.name,
            preset: this.presetLabel(report.preset),
            scope: this.transloco.translate(`settings.reports.scope.${report.scope}`),
            sites: this.siteLabel(report),
            recipients: this.recipientLabel(report),
            schedule: `${this.cadenceLabel(report)} ${report.schedule.local_time} ${report.schedule.timezone}`,
            status: this.transloco.translate(`settings.reports.status.${report.status}`),
            nextRun: this.formatDate(report.next_run_at),
            lastOutcome: report.last_outcome ? this.transloco.translate(`settings.reports.runStatus.${report.last_outcome.status}`) : this.transloco.translate('settings.reports.notAvailable')
        }));
    });
    protected readonly reportStatusOptions = computed(() => {
        this.activeLanguage();
        return (['draft', 'active', 'paused'] as const).map((value) => ({ label: this.transloco.translate(`settings.reports.status.${value}`), value }));
    });
    protected readonly timeOptions = Array.from({ length: 96 }, (_, index) => {
        const hour = Math.floor(index / 4)
            .toString()
            .padStart(2, '0');
        const minute = ((index % 4) * 15).toString().padStart(2, '0');
        const value = `${hour}:${minute}`;
        return { label: value, value };
    });
    protected readonly timezoneOptions = this.buildTimezoneOptions();
    protected readonly weekdayOptions = computed(() => {
        this.activeLanguage();
        return Array.from({ length: 7 }, (_, value) => ({ label: this.transloco.translate(`settings.reports.weekdays.${value}`), value }));
    });
    protected readonly monthDayOptions = Array.from({ length: 28 }, (_, index) => ({ label: String(index + 1), value: index + 1 }));

    constructor() {
        this.loadReports(true);
    }

    protected refresh(): void {
        this.loadReports(false);
    }

    protected openCreate(): void {
        this.editingReportID.set(null);
        this.draft.set(this.emptyDraft());
        this.resetEditorState();
        this.loadMembers(this.teamService.activeTeamId());
        this.editorVisible.set(true);
    }

    protected openEdit(report: ReportDefinition): void {
        this.editingReportID.set(report.id);
        this.draft.set({
            name: report.name,
            scope: report.scope,
            tenant_id: report.tenant_id,
            preset: report.preset,
            site_mode: report.site_mode,
            site_ids: report.sites.map((site) => site.id),
            recipient_user_ids: report.recipients.filter((recipient) => recipient.kind === 'member' && !recipient.opted_out_at && !!recipient.user_id).map((recipient) => recipient.user_id!),
            external_recipient_emails: report.recipients.filter((recipient) => recipient.kind === 'external').map((recipient) => recipient.email),
            schedule: { ...report.schedule },
            status: report.status
        });
        this.resetEditorState();
        if (report.tenant_id) this.loadMembers(report.tenant_id);
        this.editorVisible.set(true);
    }

    protected duplicate(report: ReportDefinition): void {
        this.openEdit(report);
        this.editingReportID.set(null);
        this.draft.update((draft) => ({ ...draft, name: this.transloco.translate('settings.reports.copyName', { name: report.name }), status: 'draft' }));
    }

    protected onEditorVisibleChange(visible: boolean): void {
        this.editorVisible.set(visible);
        if (!visible) this.dialogFeedback.set(null);
    }

    protected closeEditor(): void {
        if (this.saveState() === 'saving') return;
        this.editorVisible.set(false);
        this.dialogFeedback.set(null);
    }

    protected onHistoryVisibleChange(visible: boolean): void {
        this.historyVisible.set(visible);
        if (!visible) this.historyFeedback.set(null);
    }

    protected setField<K extends keyof ReportDefinitionInput>(key: K, value: ReportDefinitionInput[K]): void {
        this.draft.update((draft) => ({ ...draft, [key]: value }));
        this.preview.set(null);
    }

    protected setScheduleField(key: keyof ReportDefinitionInput['schedule'], value: string | number | undefined): void {
        this.draft.update((draft) => ({ ...draft, schedule: { ...draft.schedule, [key]: value } }));
        this.preview.set(null);
    }

    protected setScope(scope: ReportScope): void {
        const userID = this.currentUserID();
        const teamID = this.teamService.activeTeamId();
        this.draft.update((draft) => ({
            ...draft,
            scope,
            tenant_id: scope === 'team' ? teamID : undefined,
            site_mode: scope === 'team' ? 'selected' : draft.site_mode,
            recipient_user_ids: userID ? [userID] : [],
            external_recipient_emails: []
        }));
        if (scope === 'team') this.loadMembers(teamID);
        this.preview.set(null);
    }

    protected setPreset(preset: ReportPreset): void {
        this.draft.update((draft) => {
            const siteIDs = draft.site_ids.length ? [draft.site_ids[0]!] : this.sites()[0] ? [this.sites()[0]!.id] : [];
            const schedule = { ...draft.schedule };
            if (preset === 'opportunity_brief' && schedule.frequency === 'monthly') {
                schedule.frequency = 'weekly';
                schedule.weekly_day = 1;
                delete schedule.monthly_day;
            }
            return {
                ...draft,
                preset,
                site_mode: preset === 'portfolio_digest' ? draft.site_mode : 'selected',
                site_ids: preset === 'portfolio_digest' ? draft.site_ids : siteIDs,
                schedule
            };
        });
        this.preview.set(null);
    }

    protected setFrequency(frequency: ReportFrequency): void {
        this.draft.update((draft) => {
            const schedule = { frequency, timezone: draft.schedule.timezone, local_time: draft.schedule.local_time } as ReportDefinitionInput['schedule'];
            if (frequency === 'weekly') schedule.weekly_day = draft.schedule.weekly_day ?? 1;
            if (frequency === 'monthly') schedule.monthly_day = draft.schedule.monthly_day ?? 1;
            return { ...draft, schedule };
        });
        this.preview.set(null);
    }

    protected selectedSiteID(): string {
        return this.draft().site_ids[0] ?? '';
    }

    protected setSelectedSite(siteID: string): void {
        this.setField('site_ids', siteID ? [siteID] : []);
    }

    protected previewReport(): void {
        if (!this.formValid()) return;
        this.dialogFeedback.set(null);
        this.service.preview({ ...this.draft(), status: 'draft' }, this.editingReportID() ?? undefined).subscribe({
            next: (preview) => this.preview.set(preview),
            error: () => this.dialogFeedback.set({ key: 'settings.reports.errors.preview', severity: 'error' })
        });
    }

    protected save(): void {
        const status = this.draft().status;
        if (!this.formValid() || (status === 'active' && !this.mailAvailable())) return;
        this.saveState.set('saving');
        this.dialogFeedback.set(null);
        const request = this.editingReportID() ? this.service.update(this.editingReportID()!, this.draft()) : this.service.create(this.draft());
        request.pipe(finalize(() => this.saveState.set('idle'))).subscribe({
            next: () => {
                this.editorVisible.set(false);
                this.listFeedback.set({ key: 'settings.reports.saved', severity: 'success' });
            },
            error: () => this.dialogFeedback.set({ key: 'settings.reports.errors.save', severity: 'error' })
        });
    }

    protected toggle(report: ReportDefinition): void {
        const status: ReportStatus = report.status === 'active' ? 'paused' : 'active';
        if (status === 'active' && !this.mailAvailable()) return;
        this.beginReportAction(report.id);
        this.service
            .update(report.id, { status })
            .pipe(finalize(() => this.reportActionID.set(null)))
            .subscribe({
                next: () => this.listFeedback.set({ key: 'settings.reports.saved', severity: 'success' }),
                error: () => this.listFeedback.set({ key: 'settings.reports.errors.action', severity: 'error' })
            });
    }

    protected confirmRemove(report: ReportDefinition): void {
        this.confirmationService.confirm({
            message: this.transloco.translate('settings.reports.deleteConfirm', { name: report.name }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('common.actions.delete')),
            accept: () => this.remove(report)
        });
    }

    protected testSend(report: ReportDefinition): void {
        if (!this.mailAvailable()) return;
        this.beginReportAction(report.id);
        this.service
            .testSend(report.id)
            .pipe(finalize(() => this.reportActionID.set(null)))
            .subscribe({
                next: () => this.listFeedback.set({ key: 'settings.reports.testAccepted', severity: 'success' }),
                error: () => this.listFeedback.set({ key: 'settings.reports.errors.test', severity: 'error' })
            });
    }

    protected openHistory(report: ReportDefinition): void {
        this.historyReport.set(report);
        this.runs.set([]);
        this.historyFeedback.set(null);
        this.historyLoading.set(true);
        this.historyVisible.set(true);
        this.service
            .runs(report.id)
            .pipe(finalize(() => this.historyLoading.set(false)))
            .subscribe({
                next: (runs) => this.runs.set(runs),
                error: () => this.historyFeedback.set({ key: 'settings.reports.errors.action', severity: 'error' })
            });
    }

    protected retry(run: ReportRun): void {
        this.historyFeedback.set(null);
        this.historyActionID.set(run.id);
        this.service
            .retry(run.id)
            .pipe(finalize(() => this.historyActionID.set(null)))
            .subscribe({
                next: () => this.historyFeedback.set({ key: 'settings.reports.retryQueued', severity: 'success' }),
                error: () => this.historyFeedback.set({ key: 'settings.reports.errors.action', severity: 'error' })
            });
    }

    protected resubscribe(report: ReportDefinition): void {
        this.beginReportAction(report.id);
        this.service
            .resubscribe(report.id)
            .pipe(finalize(() => this.reportActionID.set(null)))
            .subscribe({
                next: () => this.listFeedback.set({ key: 'settings.reports.resubscribed', severity: 'success' }),
                error: () => this.listFeedback.set({ key: 'settings.reports.errors.action', severity: 'error' })
            });
    }

    protected addExternalRecipient(): void {
        if (this.externalRecipientsLocked()) {
            this.externalEmailError.set(this.transloco.translate('settings.reports.recipients.proRequired'));
            return;
        }
        const email = this.externalEmailInput().trim().toLocaleLowerCase('en-US');
        this.externalEmailError.set('');
        if (!this.validEmail(email)) {
            this.externalEmailError.set(this.transloco.translate('settings.reports.recipients.invalidEmail'));
            return;
        }
        const member = this.members().find((candidate) => candidate.email.trim().toLocaleLowerCase('en-US') === email);
        if (member) {
            this.draft.update((draft) => ({
                ...draft,
                recipient_user_ids: [...new Set([...draft.recipient_user_ids, member.user_id])],
                external_recipient_emails: draft.external_recipient_emails.filter((candidate) => candidate !== email)
            }));
            this.externalEmailInput.set('');
            this.preview.set(null);
            return;
        }
        if (this.draft().external_recipient_emails.includes(email)) {
            this.externalEmailInput.set('');
            return;
        }
        if (this.draft().external_recipient_emails.length >= 25) {
            this.externalEmailError.set(this.transloco.translate('settings.reports.recipients.limit'));
            return;
        }
        this.draft.update((draft) => ({ ...draft, external_recipient_emails: [...draft.external_recipient_emails, email] }));
        this.externalEmailInput.set('');
        this.preview.set(null);
    }

    protected removeExternalDraft(email: string): void {
        this.draft.update((draft) => ({ ...draft, external_recipient_emails: draft.external_recipient_emails.filter((candidate) => candidate !== email) }));
        this.externalEmailError.set('');
        this.preview.set(null);
    }

    protected resendConfirmation(report: ReportDefinition, recipient: ReportRecipient): void {
        if (this.externalRecipientsLockedForTeam(report.tenant_id)) {
            this.listFeedback.set({ key: 'settings.reports.recipients.proRequired', severity: 'error' });
            return;
        }
        this.beginReportAction(report.id);
        this.service
            .resendConfirmation(report.id, recipient.id)
            .pipe(finalize(() => this.reportActionID.set(null)))
            .subscribe({
                next: () => this.listFeedback.set({ key: 'settings.reports.recipients.resent', severity: 'success' }),
                error: () => this.listFeedback.set({ key: 'settings.reports.recipients.resendFailed', severity: 'error' })
            });
    }

    protected confirmRemoveExternalRecipient(report: ReportDefinition, recipient: ReportRecipient): void {
        this.confirmationService.confirm({
            message: this.transloco.translate('settings.reports.recipients.removeConfirm', { email: recipient.email }),
            icon: 'pi pi-exclamation-triangle',
            rejectButtonProps: dialogCancelButton(this.transloco.translate('common.actions.cancel')),
            acceptButtonProps: dialogDangerButton(this.transloco.translate('common.actions.remove')),
            accept: () => this.removeExternalRecipient(report, recipient)
        });
    }

    protected isOptedOut(report: ReportDefinition): boolean {
        return report.recipients.some((recipient) => recipient.user_id === this.currentUserID() && !!recipient.opted_out_at);
    }

    protected canManage(report: ReportDefinition): boolean {
        if (report.scope === 'personal') return report.owner_user_id === this.currentUserID();
        return ['owner', 'admin'].includes(this.teamService.teams().find((team) => team.id === report.tenant_id)?.role ?? '');
    }

    protected formValid(): boolean {
        const draft = this.draft();
        const sitesValid = draft.site_mode === 'all_accessible' || draft.site_ids.length > 0;
        const externalRecipientsValid = !this.externalRecipientsLocked() || draft.external_recipient_emails.length === 0;
        const recipientsValid =
            externalRecipientsValid && draft.recipient_user_ids.length + draft.external_recipient_emails.length > 0 && draft.external_recipient_emails.length <= 25 && draft.external_recipient_emails.every((email) => this.validEmail(email));
        return !!draft.name.trim() && sitesValid && recipientsValid && !!draft.schedule.timezone && /^([01]\d|2[0-3]):(00|15|30|45)$/.test(draft.schedule.local_time);
    }

    protected presetLabel(preset: ReportPreset): string {
        return this.transloco.translate(`settings.reports.presets.${preset}.title`);
    }

    protected cadenceLabel(report: ReportDefinition): string {
        return this.transloco.translate(`settings.reports.frequency.${report.schedule.frequency}.label`);
    }

    protected statusSeverity(status: string): 'success' | 'warn' | 'secondary' | 'danger' {
        if (status === 'active' || status === 'completed' || status === 'accepted') return 'success';
        if (status === 'failed' || status === 'partial') return 'danger';
        if (status === 'paused') return 'warn';
        return 'secondary';
    }

    protected formatDate(value?: string): string {
        if (!value) return this.transloco.translate('settings.reports.notAvailable');
        return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
    }

    protected siteLabel(report: ReportDefinition): string {
        if (report.site_mode === 'all_accessible') return this.transloco.translate('settings.reports.allAccessibleSites');
        return report.sites.map((site) => site.domain).join(', ');
    }

    protected recipientLabel(report: ReportDefinition): string {
        return report.recipients.map((recipient) => recipient.email).join(', ');
    }

    protected visibleRecipients(report: ReportDefinition): ReportRecipient[] {
        return report.recipients.slice(0, 2);
    }

    protected deliveryRecipientLabel(delivery: ReportDelivery): string {
        return delivery.recipient_email || this.historyReport()?.recipients.find((recipient) => recipient.id === delivery.recipient_id)?.email || delivery.recipient_id;
    }

    protected recipientState(recipient: ReportRecipient): string {
        if (recipient.status === 'opted_out') return 'opted_out';
        if (recipient.invitation_state === 'failed') return 'invitation_failed';
        if (recipient.status === 'pending_confirmation' && recipient.confirmation_expires_at && new Date(recipient.confirmation_expires_at).getTime() <= Date.now()) return 'expired';
        return recipient.status;
    }

    protected reportActions(report: ReportDefinition): TableRowActionItem[] {
        const actions: TableRowActionItem[] = [
            {
                label: this.transloco.translate('settings.reports.actions.history'),
                icon: 'pi pi-history',
                command: () => this.openHistory(report)
            }
        ];
        if (this.isOptedOut(report)) {
            actions.push({
                label: this.transloco.translate('settings.reports.resubscribe'),
                icon: 'pi pi-bell',
                command: () => this.resubscribe(report)
            });
        }
        if (!this.canManage(report)) return actions;

        actions.push(
            { separator: true },
            {
                label: this.transloco.translate('settings.reports.actions.test'),
                icon: 'pi pi-send',
                disabled: !this.mailAvailable(),
                command: () => this.testSend(report)
            },
            {
                label: this.transloco.translate('common.actions.edit'),
                icon: 'pi pi-pencil',
                command: () => this.openEdit(report)
            },
            {
                label: this.transloco.translate('settings.reports.actions.duplicate'),
                icon: 'pi pi-copy',
                command: () => this.duplicate(report)
            },
            {
                label: this.transloco.translate(report.status === 'active' ? 'settings.reports.actions.pause' : 'settings.reports.actions.resume'),
                icon: report.status === 'active' ? 'pi pi-pause' : 'pi pi-play',
                disabled: report.status !== 'active' && !this.mailAvailable(),
                command: () => this.toggle(report)
            }
        );

        const externalRecipients = report.recipients.filter((recipient) => recipient.kind === 'external');
        if (externalRecipients.length > 0) {
            actions.push({ separator: true });
            for (const recipient of externalRecipients) {
                if (this.recipientState(recipient) !== 'confirmed') {
                    actions.push({
                        label: `${this.transloco.translate('settings.reports.recipients.resend')} · ${recipient.email}`,
                        icon: 'pi pi-refresh',
                        disabled: this.externalRecipientsLockedForTeam(report.tenant_id),
                        command: () => this.resendConfirmation(report, recipient)
                    });
                }
                actions.push({
                    label: `${this.transloco.translate('common.actions.remove')} · ${recipient.email}`,
                    icon: 'pi pi-times',
                    danger: true,
                    command: () => this.confirmRemoveExternalRecipient(report, recipient)
                });
            }
        }

        actions.push(
            { separator: true },
            {
                label: this.transloco.translate('common.actions.delete'),
                icon: 'pi pi-trash',
                danger: true,
                command: () => this.confirmRemove(report)
            }
        );
        return actions;
    }

    private loadReports(openDeepLink: boolean): void {
        this.listFeedback.set(null);
        this.service.load().subscribe({
            next: () => {
                if (openDeepLink) this.openDeepLinkedReport();
            },
            error: () => this.listFeedback.set({ key: 'settings.reports.errors.load', severity: 'error' })
        });
    }

    private beginReportAction(reportID: string): void {
        this.listFeedback.set(null);
        this.reportActionID.set(reportID);
    }

    private remove(report: ReportDefinition): void {
        this.beginReportAction(report.id);
        this.service
            .delete(report.id)
            .pipe(finalize(() => this.reportActionID.set(null)))
            .subscribe({
                next: () => this.listFeedback.set({ key: 'settings.reports.deleted', severity: 'success' }),
                error: () => this.listFeedback.set({ key: 'settings.reports.errors.action', severity: 'error' })
            });
    }

    private removeExternalRecipient(report: ReportDefinition, recipient: ReportRecipient): void {
        const emails = report.recipients.filter((candidate) => candidate.kind === 'external' && candidate.id !== recipient.id).map((candidate) => candidate.email);
        this.beginReportAction(report.id);
        this.service
            .update(report.id, { external_recipient_emails: emails })
            .pipe(finalize(() => this.reportActionID.set(null)))
            .subscribe({
                next: () => this.listFeedback.set({ key: 'settings.reports.recipients.removed', severity: 'success' }),
                error: () => this.listFeedback.set({ key: 'settings.reports.errors.action', severity: 'error' })
            });
    }

    private resetEditorState(): void {
        this.preview.set(null);
        this.saveState.set('idle');
        this.dialogFeedback.set(null);
        this.externalEmailInput.set('');
        this.externalEmailError.set('');
    }

    private emptyDraft(): ReportDefinitionInput {
        const userID = this.profileService.profile()?.id ?? '';
        const firstSiteID = this.siteService.sites()[0]?.id;
        return {
            name: '',
            scope: 'personal',
            preset: 'site_summary',
            site_mode: 'selected',
            site_ids: firstSiteID ? [firstSiteID] : [],
            recipient_user_ids: userID ? [userID] : [],
            external_recipient_emails: [],
            schedule: {
                frequency: 'daily',
                timezone: this.detectTimezone(),
                local_time: '08:00'
            },
            status: 'draft'
        };
    }

    private loadMembers(teamID: string): void {
        if (!teamID) return;
        this.teamService.listTeamMembers(teamID).subscribe({ next: (members) => this.members.set(members), error: () => this.members.set([]) });
    }

    private externalRecipientsLockedForTeam(teamID?: string): boolean {
        if (!this.bootstrap.cloudHosted() || !teamID) return false;
        return this.teamService.teams().find((team) => team.id === teamID)?.entitlements?.allow_external_report_recipients === false;
    }

    private detectTimezone(): string {
        return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
    }

    private buildTimezoneOptions(): { label: string; value: string }[] {
        const detected = this.detectTimezone();
        const supported = typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : [];
        const zones = [detected, 'UTC', ...supported.filter((timezone) => timezone !== detected && timezone !== 'UTC')];
        return [...new Set(zones)].map((timezone) => ({ label: timezone, value: timezone }));
    }

    private validEmail(email: string): boolean {
        return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email) && email.length <= 254;
    }

    private openDeepLinkedReport(): void {
        const reportID = this.route.snapshot.queryParamMap.get('report');
        if (!reportID) return;
        const report = this.reports().find((candidate) => candidate.id === reportID);
        if (!report) return;
        this.focusedReportID.set(reportID);
        queueMicrotask(() => this.elementRef.nativeElement.querySelector<HTMLElement>(`[data-report-id="${CSS.escape(reportID)}"]`)?.scrollIntoView({ behavior: 'smooth', block: 'center' }));
        if (this.canManage(report)) this.openEdit(report);
    }
}
