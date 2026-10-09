import { computed, inject, Service } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { Router } from '@angular/router';
import { MenuItem } from '@openng/optimus-ui/api';
import { TranslocoService } from '@jsverse/transloco';
import { formatDurationInterval } from '@core/i18n/duration-format';
import { AuthService } from '@services/auth.service';
import { DashboardBootstrapService } from '@services/dashboard-bootstrap.service';
import { ShareService } from '@services/share.service';
import { SiteService } from '@features/sites/services/site.service';
import { AskAIControlService } from '@features/ask-ai/ask-ai-control.service';
import { catchError, finalize, of } from 'rxjs';

@Service()
export class UserMenuService {
    private router = inject(Router);
    private auth = inject(AuthService);
    private transloco = inject(TranslocoService);
    private bootstrap = inject(DashboardBootstrapService);
    private share = inject(ShareService);
    private siteService = inject(SiteService);
    private askAIControl = inject(AskAIControlService);
    private isSigningOut = false;
    private activeLanguage = toSignal(this.transloco.langChanges$, { initialValue: this.transloco.getActiveLang() });

    readonly menuItems = computed<MenuItem[]>(() => {
        const language = this.activeLanguage();
        const session = this.auth.session();
        const remainingSeconds = this.auth.sessionDisplayRemainingSeconds();
        const sessionItems: MenuItem[] = session
            ? [
                  {
                      label: this.transloco.translate(session.remembered ? 'userMenu.rememberedSessionStatus' : 'userMenu.sessionStatus', {
                          remaining: formatDurationInterval(remainingSeconds, language)
                      }),
                      icon: 'pi pi-clock',
                      disabled: true
                  },
                  {
                      label: this.transloco.translate('userMenu.extendSession'),
                      icon: 'pi pi-refresh',
                      disabled: this.auth.sessionExtending() || !session.extendable,
                      command: () => this.extendSession()
                  }
              ]
            : [];

        const askAIItem: MenuItem[] = this.showAskAI()
            ? [
                  {
                      label: this.transloco.translate('askAi.title'),
                      icon: 'pi pi-sparkles',
                      command: () => this.askAIControl.requestOpen()
                  },
                  { separator: true }
              ]
            : [];

        return [
            ...askAIItem,
            {
                label: this.transloco.translate('userMenu.userSettings'),
                icon: 'pi pi-user',
                command: () => this.router.navigate(['/settings'])
            },
            ...sessionItems,
            { separator: true },
            {
                label: this.transloco.translate('userMenu.signOut'),
                icon: 'pi pi-sign-out',
                command: () => this.signOut()
            }
        ];
    });

    private readonly showAskAI = computed(() => {
        if (this.share.isShareMode()) {
            return false;
        }
        const status = this.bootstrap.status();
        return !!this.siteService.activeSite() && !!status?.ask_ai?.enabled;
    });

    signOut() {
        if (this.isSigningOut) return;
        this.isSigningOut = true;
        this.auth
            .logout()
            .pipe(
                catchError(() => of(null)),
                finalize(() => {
                    this.isSigningOut = false;
                })
            )
            .subscribe();
    }

    private extendSession() {
        if (this.auth.sessionExtending()) return;
        this.auth
            .extendSession()
            .pipe(catchError(() => of(null)))
            .subscribe();
    }
}
