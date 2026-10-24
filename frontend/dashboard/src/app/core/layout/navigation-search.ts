import { ChangeDetectionStrategy, Component, computed, ElementRef, inject, input, signal, viewChild } from '@angular/core';
import { FormControl, ReactiveFormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { TranslocoPipe } from '@jsverse/transloco';
import { AutoCompleteModule } from '@openng/optimus-ui/autocomplete';
import { DialogModule } from '@openng/optimus-ui/dialog';
import { MenuItem } from '@openng/optimus-ui/api';
import { SidebarMenuService } from './sidebar-menu.service';

@Component({
    selector: 'app-navigation-search',
    imports: [ReactiveFormsModule, RouterLink, TranslocoPipe, AutoCompleteModule, DialogModule],
    templateUrl: './navigation-search.html',
    changeDetection: ChangeDetectionStrategy.OnPush,
    host: { '(document:keydown)': 'handleKeyboard($event)' }
})
export class NavigationSearch {
    readonly disabled = input(false);
    private readonly menu = inject(SidebarMenuService);
    private readonly router = inject(Router);
    private readonly trigger = viewChild<ElementRef<HTMLButtonElement>>('trigger');
    protected readonly visible = signal(false);
    protected readonly query = signal('');
    protected readonly control = new FormControl<string | MenuItem>('');
    protected readonly results = computed(() => {
        const query = this.query().trim().toLocaleLowerCase();
        return !query || this.disabled() ? [] : this.menu.searchableItems().filter((item) => item.label?.toLocaleLowerCase().includes(query));
    });

    open(): void {
        if (this.disabled() || this.visible() || document.querySelector('[role="dialog"][aria-modal="true"]')) return;
        this.visible.set(true);
    }

    close(): void {
        this.visible.set(false);
        this.query.set('');
        this.control.setValue('', { emitEvent: false });
    }

    protected restoreFocus(): void {
        this.trigger()?.nativeElement.focus();
    }

    activate(item: MenuItem): void {
        if (this.disabled() || !this.menu.searchableItems().includes(item)) return;
        this.close();
        if (item.routerLink) void this.router.navigateByUrl(item.routerLink);
        else if (item.url) window.open(item.url, item.target ?? '_self', 'noopener,noreferrer');
    }

    protected handleKeyboard(event: KeyboardEvent): void {
        if (event.defaultPrevented || event.isComposing || event.altKey || event.shiftKey || !(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 'k' || this.disabled()) return;
        if (this.visible()) {
            event.preventDefault();
            return;
        }
        if (document.querySelector('[role="dialog"][aria-modal="true"]')) return;
        event.preventDefault();
        this.open();
    }
}
