import { ChangeDetectionStrategy, Component, computed, effect, inject, input, output, signal } from '@angular/core';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { compatForm } from '@angular/forms/signals/compat';
import { TranslocoPipe, TranslocoService } from '@jsverse/transloco';
import { SelectModule } from '@openng/optimus-ui/select';
import { SkeletonModule } from '@openng/optimus-ui/skeleton';
import { Team } from '@models/analytics.types';

@Component({
    selector: 'app-team-switcher',
    imports: [ReactiveFormsModule, SelectModule, SkeletonModule, TranslocoPipe],
    templateUrl: './team-switcher.html',
    styleUrl: './team-switcher.css',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class TeamSwitcher {
    private readonly transloco = inject(TranslocoService);
    private readonly teamFormModel = signal({
        selectedTeam: new FormControl<Team | null>(null)
    });

    teams = input.required<Team[]>();
    currentTeamId = input<string>('');
    loading = input<boolean>(false);
    switching = input<boolean>(false);
    showBrand = input<boolean>(true);
    showAdd = input<boolean>(false);
    compact = input<boolean>(false);
    beforeSwitch = input<((nextTeam: Team) => boolean | Promise<boolean>) | undefined>(undefined);

    teamSelected = output<Team>();
    addClicked = output<void>();

    protected readonly teamForm = compatForm(this.teamFormModel);
    protected readonly isCheckingSwitch = signal(false);
    protected readonly activeTeam = computed(() => this.teams().find((team) => team.id === this.currentTeamId()) ?? this.teams()[0] ?? null);
    protected readonly activeTeamId = computed(() => this.activeTeam()?.id ?? '');
    protected readonly activeTeamName = computed(() => this.activeTeam()?.name ?? this.transloco.translate('teams.switcher.placeholder'));
    protected readonly interactionDisabled = computed(() => this.switching() || this.isCheckingSwitch());

    constructor() {
        effect(() => {
            this.teamForm.selectedTeam().control().setValue(this.activeTeam(), { emitEvent: false });
        });

        effect(() => {
            const control = this.teamForm.selectedTeam().control();
            if (this.interactionDisabled()) {
                control.disable({ emitEvent: false });
                return;
            }
            control.enable({ emitEvent: false });
        });
    }

    protected async onTeamChange(team: Team | null) {
        if (!team || this.interactionDisabled() || team.id === this.activeTeamId()) {
            return;
        }

        this.isCheckingSwitch.set(true);
        try {
            const proceed = await this.canSwitchTeam(team);
            if (!proceed) {
                this.teamForm.selectedTeam().control().setValue(this.activeTeam(), { emitEvent: false });
                return;
            }

            this.teamSelected.emit(team);
        } finally {
            this.isCheckingSwitch.set(false);
        }
    }

    protected onCreateTeam() {
        if (!this.showAdd() || this.interactionDisabled()) {
            return;
        }
        this.addClicked.emit();
    }

    protected teamLogoUrl(team: Team | null): string {
        if (!team) {
            return this.generateInitialsAvatar({ id: 'team', name: 'Team', logo_url: '', role: 'member', created_at: '' });
        }
        if (team.logo_url) {
            return team.logo_url;
        }
        return this.generateInitialsAvatar(team);
    }

    protected roleLabel(role: string): string {
        return this.transloco.translate(`teams.roles.${role}`);
    }

    private async canSwitchTeam(nextTeam: Team): Promise<boolean> {
        const checker = this.beforeSwitch();
        if (!checker) {
            return true;
        }
        const allowed = checker(nextTeam);
        return typeof (allowed as Promise<boolean>)?.then === 'function' ? await allowed : Boolean(allowed);
    }

    private generateInitialsAvatar(team: Team): string {
        const initials = this.teamInitials(team.name);
        const palette = this.paletteFromSeed(team.id || team.name || 'team');
        const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 28 28" role="img" aria-label="${this.escapeAttr(team.name || 'Team')}"><rect width="28" height="28" rx="6" fill="${palette.background}"/><text x="14" y="19" text-anchor="middle" font-family="Inter,Segoe UI,Arial,sans-serif" font-size="11" font-weight="700" fill="${palette.foreground}">${this.escapeText(initials)}</text></svg>`;
        return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
    }

    private teamInitials(name: string): string {
        const trimmed = (name || '').trim();
        if (!trimmed) {
            return 'T';
        }
        const parts = trimmed.split(/\s+/).filter(Boolean);
        if (parts.length === 1) {
            return parts[0].slice(0, 2).toUpperCase();
        }
        return `${parts[0].charAt(0)}${parts[1].charAt(0)}`.toUpperCase();
    }

    private paletteFromSeed(seed: string): { background: string; foreground: string } {
        const palettes: { background: string; foreground: string }[] = [
            { background: '#DCFCE7', foreground: '#065F46' },
            { background: '#DBEAFE', foreground: '#1E3A8A' },
            { background: '#FCE7F3', foreground: '#9D174D' },
            { background: '#FEF3C7', foreground: '#92400E' },
            { background: '#E0E7FF', foreground: '#3730A3' }
        ];
        let hash = 0;
        for (let i = 0; i < seed.length; i++) {
            hash = (hash << 5) - hash + seed.charCodeAt(i);
            hash |= 0;
        }
        const index = Math.abs(hash) % palettes.length;
        return palettes[index];
    }

    private escapeText(value: string): string {
        return value.replace(/[&<>"']/g, (ch) => {
            switch (ch) {
                case '&':
                    return '&amp;';
                case '<':
                    return '&lt;';
                case '>':
                    return '&gt;';
                case '"':
                    return '&quot;';
                case "'":
                    return '&#39;';
                default:
                    return ch;
            }
        });
    }

    private escapeAttr(value: string): string {
        return this.escapeText(value);
    }
}
