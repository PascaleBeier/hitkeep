import { Injectable, computed, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { Router } from '@angular/router';
import { MenuItem } from 'primeng/api';
import { TranslocoService } from '@jsverse/transloco';
import { formatDurationInterval } from '@core/i18n/duration-format';
import { PermissionService } from '@services/permission.service';
import { AuthService } from '@services/auth.service';
import { catchError, finalize, of } from 'rxjs';

@Injectable({ providedIn: 'root' })
export class UserMenuService {
    private router = inject(Router);
    private perms = inject(PermissionService);
    private auth = inject(AuthService);
    private transloco = inject(TranslocoService);
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

        return [
            {
                label: this.transloco.translate('userMenu.administration'),
                icon: 'pi pi-shield',
                visible: this.perms.isInstanceAdmin(),
                command: () => this.router.navigate(['/admin/system'])
            },
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

    signOut() {
        if (this.isSigningOut) return;
        this.isSigningOut = true;
        this.auth
            .logout()
            .pipe(
                catchError(() => of(null)),
                finalize(() => {
                    this.isSigningOut = false;
                    this.router.navigate(['/login']);
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
