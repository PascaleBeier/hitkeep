import { TestBed } from '@angular/core/testing';
import { Route } from '@angular/router';
import { firstValueFrom, of } from 'rxjs';
import { vi } from 'vitest';

import { SelectivePreloadingStrategy } from './selective-preloading-strategy';

describe('SelectivePreloadingStrategy', () => {
    let service: SelectivePreloadingStrategy;

    beforeEach(() => {
        TestBed.configureTestingModule({});
        service = TestBed.inject(SelectivePreloadingStrategy);
    });

    it('does not preload routes unless they explicitly opt in', async () => {
        const load = vi.fn(() => of('loaded'));

        const result = await firstValueFrom(service.preload({ path: 'events' }, load));

        expect(service).toBeTruthy();
        expect(result).toBeNull();
        expect(load).not.toHaveBeenCalled();
    });

    it('preloads an opted-in route', async () => {
        const load = vi.fn(() => of('loaded'));
        const route: Route = { path: 'dashboard', data: { preload: true } };

        const result = await firstValueFrom(service.preload(route, load));

        expect(result).toBe('loaded');
        expect(load).toHaveBeenCalledTimes(1);
    });
});
