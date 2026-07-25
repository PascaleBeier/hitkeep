# @hitkeep/tracker

[![npm version](https://img.shields.io/npm/v/@hitkeep/tracker.svg)](https://www.npmjs.com/package/@hitkeep/tracker)
[![license](https://img.shields.io/npm/l/@hitkeep/tracker.svg)](https://github.com/PascaleBeier/hitkeep/blob/main/LICENSE)
[![zero dependencies](https://img.shields.io/badge/dependencies-0-brightgreen.svg)](https://www.npmjs.com/package/@hitkeep/tracker?activeTab=dependencies)
[![types included](https://img.shields.io/badge/types-included-blue.svg)](https://www.npmjs.com/package/@hitkeep/tracker)

Typed, cookieless web analytics tracking for **React, Vue, Angular, Astro**, and any other bundler-based frontend — a privacy-friendly Google Analytics alternative that stores no cookies and needs no consent banner for basic measurement.

This is the official browser SDK for [HitKeep](https://hitkeep.com), a self-hostable, GDPR-friendly analytics platform that runs as a single Go binary. The package bundles the same tracker that ships as HitKeep's `hk.js` snippet — compiled from the same source — as a fully typed ES module.

- **Cookieless by default** — no cookies, no cross-site identifiers, no consent banner for basic analytics
- **Framework-native** — no `<script>` tag, no `window.hk` casts, full TypeScript types
- **Automatic tracking** — pageviews, SPA route changes, outbound links, file downloads, form submits
- **Typed e-commerce events** — `view_item`, `add_to_cart`, `begin_checkout`, `purchase`
- **Tiny and self-contained** — ~3.7 kB gzipped, zero runtime dependencies, ESM + CJS

## Install

```sh
npm install @hitkeep/tracker
```

## Quick start

```ts
import { init, track } from '@hitkeep/tracker';

init({ host: 'https://your-hitkeep.example.com' });

track('signup_clicked', { plan: 'pro' });
```

`host` is the origin (or path prefix) your HitKeep instance serves the tracker from — the same URL the `hk.js` snippet would load from, including [custom tracking domains](https://hitkeep.com/guides/tracking/custom-tracking-domains/). Pageviews are captured automatically, including history-based SPA navigations, so no router wiring is required.

## Framework examples

### React and Next.js

```tsx
'use client';

import { useEffect } from 'react';
import { cleanup, init } from '@hitkeep/tracker';

export function Analytics() {
    useEffect(() => {
        init({ host: 'https://your-hitkeep.example.com' });
        return cleanup;
    }, []);

    return null;
}
```

### Vue and Nuxt

```ts
import { createApp } from 'vue';
import { init } from '@hitkeep/tracker';
import App from './App.vue';

init({ host: 'https://your-hitkeep.example.com' });

createApp(App).mount('#app');
```

### Angular

```ts
import { ApplicationConfig, provideAppInitializer } from '@angular/core';
import { init } from '@hitkeep/tracker';

export const appConfig: ApplicationConfig = {
    providers: [provideAppInitializer(() => init({ host: 'https://your-hitkeep.example.com' }))]
};
```

### Astro

```astro
<script>
    import { init } from '@hitkeep/tracker';

    init({ host: 'https://your-hitkeep.example.com' });
</script>
```

Full walkthroughs for each framework, including Next.js App Router and Astro view transitions, are in the [npm package guide](https://hitkeep.com/guides/tracking/npm-package/).

## API

| Export | Purpose |
| :--- | :--- |
| `init(config)` | Start the tracker and return a handle. Idempotent. |
| `track(name, properties?)` | Record a custom event. Calls before `init` are queued. |
| `trackPageview()` | Send a manual pageview. |
| `cleanup()` | Remove listeners and allow re-initialization. |
| `blockTrackingForMe()` / `enableTrackingForMe()` / `isTrackingEnabled()` | Visitor opt-out controls. |
| `trackViewItem` / `trackAddToCart` / `trackBeginCheckout` / `trackPurchase` | Typed [e-commerce events](https://hitkeep.com/guides/analytics/ecommerce/). |

### Configuration

```ts
init({
    host: 'https://your-hitkeep.example.com', // required
    autoCapturePageviews: true, // send a pageview on init
    autoTrackSpaNavigation: true, // pageviews on pushState/replaceState/popstate
    outboundLinks: true, // outbound_click events
    fileDownloads: true, // file_download events
    formSubmissions: true, // form_submit events
    webVitals: false, // load the Web Vitals bundle from `${host}/hk-vitals.js`
    useBeacon: true, // prefer navigator.sendBeacon
    respectDoNotTrack: true, // drop tracking when DNT is enabled
    captureOnLocalhost: false, // localhost is blocked by default
    bindToWindow: true // expose window.hk.event for snippet-compatible callers
});
```

Each option maps to a `hk.js` data attribute where one exists — see the [tracker architecture guide](https://hitkeep.com/guides/tracking/tracker-architecture/) for the delivery, retry, and storage behavior shared by both.

### E-commerce

```ts
import { trackPurchase } from '@hitkeep/tracker';

trackPurchase({
    transaction_id: 'tx-1042',
    value: 49.9,
    currency: 'EUR',
    items: [{ item_id: 'sku-1', item_name: 'Starter Plan', quantity: 1, price: 49.9 }]
});
```

### Visitor opt-out

This package bundles the tracker into your application, so content blockers do not filter it. Offer visitors an explicit opt-out:

```ts
import { blockTrackingForMe, enableTrackingForMe, isTrackingEnabled } from '@hitkeep/tracker';
```

The opt-out persists in `localStorage` under `hk_ignore` and is honored by both this package and the `hk.js` snippet.

## What you need to run it

The package sends data to a HitKeep instance. You can [self-host HitKeep](https://hitkeep.com/guides/installation/) as a single binary, Docker image, or Helm chart, or use [HitKeep Cloud](https://cloud.hitkeep.eu/signup?plan=free&billing=monthly&utm_source=npm&utm_medium=referral&utm_campaign=tracker_package). HitKeep is open source under the MIT license — the source for this package lives in [PascaleBeier/hitkeep](https://github.com/PascaleBeier/hitkeep).

## Documentation

- [NPM package guide](https://hitkeep.com/guides/tracking/npm-package/) — framework integration, full API reference, differences from the snippet
- [Tracker architecture](https://hitkeep.com/guides/tracking/tracker-architecture/) — delivery, retries, SPA handling, storage boundaries
- [Custom events](https://hitkeep.com/guides/tracking/custom-events/) — event naming and property conventions
- [Automatic events](https://hitkeep.com/guides/tracking/automatic-events/) — outbound clicks, downloads, form submits
- [E-commerce analytics](https://hitkeep.com/guides/analytics/ecommerce/) — purchase funnel tracking
- [Self-hosting HitKeep](https://hitkeep.com/guides/installation/) — binary, Docker Compose, Kubernetes

## Versioning

This package is versioned in lockstep with HitKeep itself and published with every HitKeep release. Keep the major version aligned with your HitKeep instance.

## License

MIT © Pascale Beier — see [LICENSE](https://github.com/PascaleBeier/hitkeep/blob/main/LICENSE).
