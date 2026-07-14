import { TestBed } from '@angular/core/testing';
import { NavigationStart, Router } from '@angular/router';
import { Subject } from 'rxjs';

import { NavigationNoticeService } from './navigation-notice.service';

describe('NavigationNoticeService', () => {
    const routerEvents = new Subject<NavigationStart>();

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [{ provide: Router, useValue: { events: routerEvents } }]
        });
    });

    it('delivers a navigation notice once', () => {
        const service = TestBed.inject(NavigationNoticeService);

        service.show('sites.settings.notices.siteUnavailable');

        expect(service.key()).toBe('sites.settings.notices.siteUnavailable');
        expect(service.consume()).toBe('sites.settings.notices.siteUnavailable');
        expect(service.consume()).toBeNull();
    });

    it('clears the previous notice when a new navigation starts', () => {
        const service = TestBed.inject(NavigationNoticeService);
        service.show('sites.settings.notices.siteUnavailable');

        routerEvents.next(new NavigationStart(1, '/dashboard'));

        expect(service.key()).toBeNull();
    });

    it('keeps a guard notice through its redirect navigation', () => {
        const service = TestBed.inject(NavigationNoticeService);
        service.show('sites.settings.notices.siteUnavailable', { preserveNextNavigation: true });

        routerEvents.next(new NavigationStart(1, '/overview'));
        expect(service.key()).toBe('sites.settings.notices.siteUnavailable');

        routerEvents.next(new NavigationStart(2, '/dashboard'));
        expect(service.key()).toBeNull();
    });
});
