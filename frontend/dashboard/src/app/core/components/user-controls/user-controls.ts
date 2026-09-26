import { ChangeDetectionStrategy, Component, inject, input, signal } from '@angular/core';
import { AvatarModule } from '@openng/optimus-ui/avatar';
import { MenuModule } from '@openng/optimus-ui/menu';
import { TranslocoPipe } from '@jsverse/transloco';
import { PreferencesService } from '@services/preferences.service';
import { UserMenuService } from '@services/user-menu.service';
import { UserProfileService } from '@services/user-profile.service';
import { ThemeDialog } from '@components/theme-dialog/theme-dialog';

@Component({
    selector: 'app-user-controls',
    standalone: true,
    imports: [AvatarModule, MenuModule, TranslocoPipe, ThemeDialog],
    changeDetection: ChangeDetectionStrategy.OnPush,
    template: `
        <div class="flex items-center gap-1">
            <button
                type="button"
                (click)="themeDialogVisible.set(true)"
                class="cursor-pointer rounded-full p-2 text-muted-color hover:bg-surface-100 focus:outline-none focus:ring-2 focus:ring-primary-500 dark:hover:bg-surface-800"
                [attr.aria-label]="'theme.openDialogAria' | transloco"
            >
                <i class="pi pi-palette" aria-hidden="true"></i>
            </button>
            <button
                type="button"
                (click)="prefs.toggleTheme()"
                class="cursor-pointer rounded-full p-2 text-muted-color hover:bg-surface-100 focus:outline-none focus:ring-2 focus:ring-primary-500 dark:hover:bg-surface-800"
                [attr.aria-label]="prefs.isDarkMode() ? ('common.switchToLightModeAria' | transloco) : ('common.switchToDarkModeAria' | transloco)"
            >
                <i class="pi" [class]="prefs.isDarkMode() ? 'pi-moon' : 'pi-sun'" aria-hidden="true"></i>
            </button>

            @if (showMenu()) {
                <button
                    type="button"
                    (click)="profileMenu.toggle($event)"
                    class="cursor-pointer flex items-center gap-2 rounded-full py-1.5 pl-2 pr-1 hover:bg-surface-100 focus:outline-none focus:ring-2 focus:ring-primary-500 dark:hover:bg-surface-800"
                    aria-haspopup="true"
                    [attr.aria-label]="'common.openUserMenuAria' | transloco"
                >
                    @if (profile.avatarUrl()) {
                        <p-avatar [image]="profile.avatarUrl()" shape="circle" styleClass="w-7 h-7 bg-surface-200 dark:bg-surface-700" aria-hidden="true" />
                    } @else {
                        <p-avatar icon="pi pi-user" shape="circle" styleClass="w-7 h-7 bg-surface-200 dark:bg-surface-700" aria-hidden="true" />
                    }
                    <span class="hidden pr-1 text-sm font-medium text-[var(--p-text-color)] xl:inline">{{ profile.displayName() }}</span>
                    <i class="pi pi-chevron-down pr-1 text-xs text-muted-color" aria-hidden="true"></i>
                </button>
                <p-menu #profileMenu [model]="userMenu.menuItems()" [popup]="true" appendTo="body" />
            }
        </div>
        <app-theme-dialog [(visible)]="themeDialogVisible" />
    `
})
export class UserControls {
    showMenu = input<boolean>(true);

    protected themeDialogVisible = signal(false);
    protected prefs = inject(PreferencesService);
    protected userMenu = inject(UserMenuService);
    protected profile = inject(UserProfileService);
}
