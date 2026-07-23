import { NgOptimizedImage } from '@angular/common';
import { ChangeDetectionStrategy, Component, effect, inject, signal } from '@angular/core';
import { FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { TranslocoPipe } from '@jsverse/transloco';
import { ButtonModule } from '@openng/optimus-ui/button';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { MessageModule } from '@openng/optimus-ui/message';

import { SettingsCard } from '@features/settings/components/settings-card';
import { TeamService } from '@services/team.service';

@Component({
    selector: 'app-team-branding',
    imports: [TranslocoPipe, ReactiveFormsModule, InputTextModule, ButtonModule, MessageModule, NgOptimizedImage, SettingsCard],
    templateUrl: './team-branding.html',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamBrandingPage {
    protected readonly teamService = inject(TeamService);
    protected readonly team = this.teamService.activeTeam;

    protected readonly isSaving = signal(false);
    protected readonly successKey = signal('');
    protected readonly errorKey = signal('');
    protected readonly form = new FormGroup({
        name: new FormControl('', { nonNullable: true, validators: [Validators.required, Validators.maxLength(120)] }),
        logo_url: new FormControl('', { nonNullable: true, validators: [Validators.maxLength(2048)] })
    });

    constructor() {
        effect(() => {
            const t = this.team();
            if (t) {
                this.form.patchValue({ name: t.name, logo_url: t.logo_url ?? '' }, { emitEvent: false });
            }
        });
    }

    protected saveSettings(): void {
        if (this.form.invalid || this.isSaving()) {
            return;
        }

        const t = this.team();
        if (!t) {
            return;
        }

        this.successKey.set('');
        this.errorKey.set('');
        this.isSaving.set(true);

        const { name, logo_url } = this.form.getRawValue();
        this.teamService.updateTeam(t.id, { name, logo_url }).subscribe({
            next: () => {
                this.isSaving.set(false);
                this.successKey.set('admin.team.settings.saveSuccess');
            },
            error: () => {
                this.isSaving.set(false);
                this.errorKey.set('admin.team.settings.saveError');
            }
        });
    }
}
