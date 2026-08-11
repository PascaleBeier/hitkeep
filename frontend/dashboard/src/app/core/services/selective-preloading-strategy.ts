import { Service } from '@angular/core';
import { PreloadingStrategy, Route } from '@angular/router';
import { Observable, of } from 'rxjs';

@Service()
export class SelectivePreloadingStrategy implements PreloadingStrategy {
    preload(route: Route, load: () => Observable<unknown>): Observable<unknown> {
        return route.data?.['preload'] === true ? load() : of(null);
    }
}
