const DOCS_BASE = 'https://hitkeep.com';

/**
 * Public documentation URLs for the tracker install surfaces. Most other pages still
 * declare their own guide URL inline; move them here as they are touched.
 *
 * Note there is no `/guides/tracking/installation/` page: `/guides/installation/` is the
 * self-hosting guide and must not be linked as a tracker install guide.
 */
export const DOCS_LINKS = {
    trackerArchitecture: `${DOCS_BASE}/guides/tracking/tracker-architecture/`,
    npmPackage: `${DOCS_BASE}/guides/tracking/npm-package/`,
    serverSideTracking: `${DOCS_BASE}/guides/tracking/server-side-tracking/`,
    wordpress: `${DOCS_BASE}/guides/integrations/wordpress/`,
    apiClients: `${DOCS_BASE}/guides/security/api-clients/`
} as const;

/** Public WordPress plugin directory listing for the first-party HitKeep plugin. */
export const WORDPRESS_PLUGIN_URL = 'https://wordpress.org/plugins/hitkeep/';

/** Public npm registry page for the browser SDK. */
export const NPM_PACKAGE_URL = 'https://www.npmjs.com/package/@hitkeep/tracker';

/** Package name of the browser SDK, as installed from npm. */
export const NPM_PACKAGE_NAME = '@hitkeep/tracker';
