import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { RouteProgressBar } from '@layout/route-progress-bar';

@Component({
    selector: 'app-root',
    imports: [RouterOutlet, RouteProgressBar],
    templateUrl: './app.html',
    changeDetection: ChangeDetectionStrategy.OnPush,
    styleUrl: './app.css'
})
export class App {
    protected readonly title = signal('dashboard');
}
