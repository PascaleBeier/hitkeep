import path from 'node:path';

import { defineConfig, devices } from '@playwright/test';

const dashboardDir = __dirname;
const repoRoot = path.resolve(dashboardDir, '../..');
const port = Number(process.env.HITKEEP_E2E_PORT || 8098);
const publicPath = normalizePublicPath(process.env.HITKEEP_E2E_PUBLIC_PATH || '/');
const rootBaseURL = process.env.HITKEEP_BASE_URL || `http://127.0.0.1:${port}`;
const baseURL = trimTrailingSlash(joinPublicURL(rootBaseURL, publicPath));
const workers = process.env.CI ? 1 : parsePositiveInteger(process.env.HITKEEP_E2E_WORKERS, 1);

export default defineConfig({
    testDir: './e2e',
    outputDir: process.env.HITKEEP_E2E_OUTPUT_DIR || 'test-results',
    timeout: 60_000,
    expect: {
        timeout: 10_000
    },
    fullyParallel: false,
    forbidOnly: !!process.env.CI,
    retries: process.env.CI ? 1 : 0,
    workers,
    reporter: [
        ['list'],
        [
            'html',
            {
                open: 'never',
                outputFolder: process.env.HITKEEP_E2E_HTML_REPORT || 'playwright-report'
            }
        ]
    ],
    use: {
        baseURL,
        trace: 'on-first-retry',
        screenshot: 'only-on-failure',
        video: 'on-first-retry'
    },
    projects: [
        {
            name: 'chromium',
            use: {
                ...devices['Desktop Chrome']
            }
        }
    ],
    webServer: process.env.HITKEEP_BASE_URL
        ? undefined
        : {
              command: 'bash tests/e2e/serve-dashboard.sh',
              cwd: repoRoot,
              url: `${baseURL}/healthz`,
              timeout: 300_000,
              reuseExistingServer: process.env.HITKEEP_E2E_REUSE_SERVER === '1',
              gracefulShutdown: { signal: 'SIGTERM', timeout: 5_000 },
              stdout: 'pipe',
              stderr: 'pipe'
          }
});

function normalizePublicPath(value: string) {
    const trimmed = (value || '/').trim();
    if (!trimmed || trimmed === '/') {
        return '/';
    }
    const withLeadingSlash = trimmed.startsWith('/') ? trimmed : `/${trimmed}`;
    return withLeadingSlash.endsWith('/') ? withLeadingSlash : `${withLeadingSlash}/`;
}

function joinPublicURL(base: string, pathPrefix: string) {
    if (pathPrefix === '/') {
        return base;
    }

    const url = new URL(base);
    const basePath = normalizePublicPath(url.pathname);
    const nextPath = pathPrefix.replace(/\/$/, '');

    if (basePath !== '/' && nextPath === basePath.replace(/\/$/, '')) {
        return url.toString();
    }

    url.pathname = `${basePath === '/' ? '' : basePath.replace(/\/$/, '')}${nextPath}`;
    return url.toString();
}

function trimTrailingSlash(url: string) {
    return url.endsWith('/') ? url.slice(0, -1) : url;
}

function parsePositiveInteger(value: string | undefined, fallback: number) {
    if (!value) {
        return fallback;
    }

    const parsed = Number(value);
    return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}
