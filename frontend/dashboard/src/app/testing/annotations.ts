import { HttpTestingController } from '@angular/common/http/testing';
import type { Annotation } from '@models/analytics.types';

/**
 * Answers the notes lookup every chart shares once a site is active. Safe to
 * call when no lookup happened, so `afterEach` can drain it before `verify()`.
 */
export function flushAnnotations(httpMock: HttpTestingController, annotations: Annotation[] = []): void {
    for (const request of httpMock.match((req) => /^\/api\/sites\/[^/]+\/annotations$/.test(req.url) && req.method === 'GET')) {
        request.flush(annotations);
    }
}
