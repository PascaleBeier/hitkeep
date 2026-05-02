const fs = require("node:fs");
const path = require("node:path");
const { execFileSync } = require("node:child_process");
const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

const DEFAULT_FIXTURE_DIR = path.resolve(__dirname, "../../../tests/fixtures/imports/plausible");
const PRIMARY_SEEDED_SITE_DOMAIN = "acme-analytics.io";
const IMPORT_RANGE_FROM = "2026-04-02T00:00:00Z";
const IMPORT_RANGE_TO = "2026-05-01T00:00:00Z";
const DESKTOP_VIEWPORT = { width: 1440, height: 1024 };
const MOBILE_VIEWPORT = { width: 390, height: 844 };
const FIXTURE_CSV_COUNT = 11;
const FIXTURE_ROWS_ACCEPTED = 16;
const FIXTURE_EVENT_TOTAL = 8;
const FIXTURE_EVENT_PROPERTY_ROWS = 2;
const FIXTURE_OUTBOUND_TOTAL = 3;
const FIXTURE_ENGAGEMENT_TOTAL = 5;
const FIXTURE_OUTBOUND_URL = "https://example.test/out";
const PERSISTED_IMPORT_SCREENSHOTS = new Set(["imports-plausible-provider", "imports-plausible-zip-review", "imports-plausible-events-report"]);

test.describe.configure({ mode: "serial", timeout: 300_000 });

test("imports the provided Plausible ZIP fixture through the dashboard", async ({ page }, testInfo) => {
    const fixtureZip = plausibleFixtureZipPath(testInfo);

    await runPlausibleImportFlow(page, [fixtureZip], "zip", testInfo);
});

test("imports the provided Plausible CSV fixture set through the dashboard", async ({ page }, testInfo) => {
    const csvFiles = collectFixtureCSVs(plausibleFixtureDirPath());
    expect(csvFiles).toHaveLength(FIXTURE_CSV_COUNT);

    await runPlausibleImportFlow(page, rotateFiles(csvFiles), "csv", testInfo);
});

async function runPlausibleImportFlow(page, files, mode, testInfo) {
    await page.setViewportSize(DESKTOP_VIEWPORT);
    await login(page, "/imports");
    const targetSite = await createImportTargetSite(page, mode);
    await page.goto("/imports", { waitUntil: "domcontentloaded" });
    await selectSeededSite(page, targetSite.domain);
    await expect(page.getByRole("heading", { name: "Site import" })).toBeVisible();

    const siteId = targetSite.id;
    const before = await listImports(page, siteId);
    const beforeIds = new Set(before.map((job) => job.id));
    const outboundTotalBefore = 0;
    const engagementTotalBefore = 0;
    let importId = "";

    try {
        if (mode === "zip") {
            await captureImportScreenshot(page, "imports-plausible-provider", testInfo);
            await captureDarkImportScreenshot(page, "imports-plausible-provider-dark", testInfo);
        }
        await page.getByRole("button", { name: /select plausible importer/i }).click();
        await expect(page.getByText("Drop files here")).toBeVisible();
        await captureImportScreenshot(page, "imports-plausible-upload", testInfo);

        await dropImportFiles(page, files);
        await expect(page.locator(".imports-fileupload__summary")).toContainText(String(files.length));
        await captureImportScreenshot(page, `imports-plausible-${mode}-selected`, testInfo);

        await page.getByRole("button", { name: /upload and validate/i }).click();
        await expect(page.getByText("Validation manifest")).toBeVisible({ timeout: 30_000 });
        await expect(page.getByText("Rows accepted")).toBeVisible();
        await expect(page.locator(".imports-manifest-grid")).toContainText(String(FIXTURE_ROWS_ACCEPTED));
        await expect(page.locator(".imports-event-panel").first()).toContainText(String(FIXTURE_EVENT_TOTAL));
        await expect(page.getByText("Unattributed rows")).toBeVisible();
        await expect(page.locator(".imports-event-panel").filter({ hasText: "Event properties" })).toContainText(String(FIXTURE_EVENT_PROPERTY_ROWS));
        await expect(page.locator(".imports-event-panel").filter({ hasText: "Event dimensions" })).toContainText("browser");
        await expect(page.getByText("outbound_click")).toBeVisible();
        await expect(page.getByText("engagement")).toBeVisible();
        await captureImportScreenshot(page, `imports-plausible-${mode}-review`, testInfo);

        const validated = await waitForNewImport(page, siteId, beforeIds, (job) => job.status === "validated");
        importId = validated.id;
        assertFixtureManifest(validated.manifest);

        if (mode === "zip") {
            await captureDarkImportScreenshot(page, "imports-plausible-zip-review-dark", testInfo);
            await page.setViewportSize(MOBILE_VIEWPORT);
            await captureImportScreenshot(page, "imports-plausible-review-mobile", testInfo);
            await page.setViewportSize(DESKTOP_VIEWPORT);
        }

        await page.getByRole("button", { name: /start import/i }).click();
        const completed = await waitForImportStatus(page, siteId, importId, "completed");
        expect(completed.rows_imported).toBeGreaterThanOrEqual(completed.manifest.rows_accepted);
        assertFixtureManifest(completed.manifest);
        await expect(page.getByText("Import completed.")).toBeVisible({ timeout: 30_000 });
        await captureImportScreenshot(page, `imports-plausible-${mode}-completed`, testInfo);

        const eventNames = await eventNamesForImportRange(page, siteId);
        expect(eventNames).toEqual(expect.arrayContaining(["outbound_click", "engagement"]));

        const outboundTotalAfter = await eventTimeseriesTotalForImportRange(page, siteId, "outbound_click");
        const engagementTotalAfter = await eventTimeseriesTotalForImportRange(page, siteId, "engagement");
        expect(outboundTotalAfter - outboundTotalBefore).toBe(FIXTURE_OUTBOUND_TOTAL);
        expect(engagementTotalAfter - engagementTotalBefore).toBe(FIXTURE_ENGAGEMENT_TOTAL);

        const propertyKeys = await eventPropertyKeysForImportRange(page, siteId, "outbound_click");
        expect(propertyKeys).toContain("url");

        const propertyBreakdown = await eventPropertyBreakdownForImportRange(page, siteId, "outbound_click", "url");
        expect(propertyBreakdown).toEqual(expect.arrayContaining([expect.objectContaining({ name: FIXTURE_OUTBOUND_URL, value: FIXTURE_OUTBOUND_TOTAL })]));

        if (mode === "zip") {
            await openImportedEventReport(page, targetSite.domain);
            await captureImportScreenshot(page, "imports-plausible-events-report", testInfo);
        }
    } finally {
        await cleanupCreatedImports(page, siteId, beforeIds, importId);
        await deleteSite(page, siteId);
    }
}

async function createImportTargetSite(page, mode) {
    const domain = `plausible-import-${mode}-${Date.now()}.example.test`;
    const response = await page.request.post("/api/sites", {
        headers: originHeaders(page),
        data: { domain }
    });
    expect(response.ok()).toBeTruthy();
    return response.json();
}

function plausibleFixtureZipPath(testInfo) {
    const configured = process.env.HITKEEP_PLAUSIBLE_FIXTURE_ZIP;
    if (configured) {
        expect(fs.existsSync(configured)).toBeTruthy();
        return configured;
    }
    const zipPath = testInfo.outputPath("plausible-export.zip");
    execFileSync("zip", ["-q", zipPath, ...collectFixtureCSVs(plausibleFixtureDirPath()).map((filePath) => path.relative(plausibleFixtureDirPath(), filePath))], {
        cwd: plausibleFixtureDirPath()
    });
    return zipPath;
}

function plausibleFixtureDirPath() {
    const configured = process.env.HITKEEP_PLAUSIBLE_FIXTURE_CSV_DIR;
    const fixtureDir = configured || DEFAULT_FIXTURE_DIR;
    expect(fs.existsSync(fixtureDir)).toBeTruthy();
    return fixtureDir;
}

function collectFixtureCSVs(root) {
    const files = [];
    const visit = (dir) => {
        for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
            const fullPath = path.join(dir, entry.name);
            if (entry.isDirectory()) {
                visit(fullPath);
            } else if (entry.isFile() && entry.name.endsWith(".csv")) {
                files.push(fullPath);
            }
        }
    };
    visit(root);
    return files.sort();
}

function rotateFiles(files) {
    const offset = Math.floor(files.length / 2);
    return [...files.slice(offset), ...files.slice(0, offset)];
}

async function dropImportFiles(page, files) {
    const payloads = files.map((filePath) => {
        const lowerName = path.basename(filePath).toLowerCase();
        return {
            name: path.basename(filePath),
            mimeType: lowerName.endsWith(".zip") ? "application/zip" : "text/csv",
            base64: fs.readFileSync(filePath).toString("base64")
        };
    });
    const dataTransfer = await page.evaluateHandle((items) => {
        const dt = new DataTransfer();
        for (const item of items) {
            const binary = window.atob(item.base64);
            const bytes = new Uint8Array(binary.length);
            for (let i = 0; i < binary.length; i += 1) {
                bytes[i] = binary.charCodeAt(i);
            }
            dt.items.add(new File([bytes], item.name, { type: item.mimeType }));
        }
        return dt;
    }, payloads);

    const uploadContent = page.locator(".p-fileupload-content");
    await uploadContent.dispatchEvent("dragenter", { dataTransfer });
    await uploadContent.dispatchEvent("dragover", { dataTransfer });
    await expect(uploadContent).toHaveClass(/p-fileupload-highlight/);
    await uploadContent.dispatchEvent("drop", { dataTransfer });
    await dataTransfer.dispose();
}

async function selectSeededSite(page, domain = PRIMARY_SEEDED_SITE_DOMAIN) {
    const combobox = page.locator('[role="combobox"]:visible').first();
    await expect(combobox).toBeVisible();

    const currentSite = ((await combobox.textContent()) || "").trim();
    if (currentSite.includes(domain)) {
        return;
    }

    await page.locator('[aria-label="Select a site to view stats"]:visible').first().click();
    const option = page.locator('[role="option"]:visible').filter({ hasText: domain }).first();
    await expect(option).toBeVisible();
    await option.click();
    await expect(combobox).toContainText(domain);
}

async function seededSiteId(page, domain = PRIMARY_SEEDED_SITE_DOMAIN) {
    const sites = await apiJSON(page, "/api/sites");
    const site = sites.find((candidate) => candidate.domain === domain);
    expect(site, `seeded site ${domain} should exist`).toBeTruthy();
    return site.id;
}

async function listImports(page, siteId) {
    const response = await apiJSON(page, `/api/sites/${siteId}/imports`);
    return response.imports || [];
}

async function waitForNewImport(page, siteId, beforeIds, predicate) {
    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
        const imports = await listImports(page, siteId);
        const created = imports.find((job) => !beforeIds.has(job.id) && job.provider === "plausible" && predicate(job));
        if (created) {
            return created;
        }
        await page.waitForTimeout(500);
    }
    throw new Error("Timed out waiting for Plausible import to validate");
}

async function waitForImportStatus(page, siteId, importId, expectedStatus) {
    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
        const job = await apiJSON(page, `/api/sites/${siteId}/imports/${importId}`);
        if (job.status === expectedStatus) {
            return job;
        }
        if (job.status === "failed" || job.status === "validation_failed") {
            throw new Error(`Import ${importId} failed: ${job.error || job.status}`);
        }
        await page.waitForTimeout(500);
    }
    throw new Error(`Timed out waiting for Plausible import ${importId} to reach ${expectedStatus}`);
}

async function eventNamesForImportRange(page, siteId) {
    return apiJSON(page, `/api/sites/${siteId}/events/names?from=${encodeURIComponent(IMPORT_RANGE_FROM)}&to=${encodeURIComponent(IMPORT_RANGE_TO)}`);
}

async function eventPropertyKeysForImportRange(page, siteId, eventName) {
    return apiJSON(page, `/api/sites/${siteId}/events/properties?from=${encodeURIComponent(IMPORT_RANGE_FROM)}&to=${encodeURIComponent(IMPORT_RANGE_TO)}&event_name=${encodeURIComponent(eventName)}`);
}

async function eventTimeseriesTotalForImportRange(page, siteId, eventName) {
    const series = await apiJSON(page, `/api/sites/${siteId}/events/timeseries?from=${encodeURIComponent(IMPORT_RANGE_FROM)}&to=${encodeURIComponent(IMPORT_RANGE_TO)}&event_name=${encodeURIComponent(eventName)}`);
    return series.reduce((sum, point) => sum + point.count, 0);
}

async function eventPropertyBreakdownForImportRange(page, siteId, eventName, propertyKey) {
    return apiJSON(page, `/api/sites/${siteId}/events/breakdown?from=${encodeURIComponent(IMPORT_RANGE_FROM)}&to=${encodeURIComponent(IMPORT_RANGE_TO)}&event_name=${encodeURIComponent(eventName)}&property_key=${encodeURIComponent(propertyKey)}`);
}

async function openImportedEventReport(page, domain) {
    await page.goto("/events", { waitUntil: "domcontentloaded" });
    await selectSeededSite(page, domain);
    const yearRangeButton = page.getByRole("button", { name: "Last year" });
    await yearRangeButton.click();
    await expect(yearRangeButton).toHaveAttribute("aria-pressed", "true");

    await selectVisibleOptionMatching(page, "#event-name-select", /outbound_click/);
    await expect(page.locator("#event-name-select")).toContainText("outbound_click");

    const eventActivity = page.locator("p-card").filter({ hasText: "Event activity" }).first();
    await expect(eventActivity).toContainText("Total events");
    await expect(eventActivity).not.toContainText("No event data");
    await expect(page.getByText("Select a property to view the breakdown.")).toBeVisible();
}

async function deleteSite(page, siteId) {
    const response = await page.request.delete(`/api/sites/${siteId}`, {
        headers: originHeaders(page)
    });
    expect([200, 404]).toContain(response.status());
}

async function selectVisibleOptionMatching(page, selector, pattern) {
    const select = page.locator(`${selector}:visible`).first();
    await expect(select).toBeVisible({ timeout: 15_000 });
    await expect(select).toBeEnabled();
    const selectHost = select.locator("xpath=ancestor::p-select[1]");
    const selectTrigger = selectHost.locator('[role="button"][aria-label="dropdown trigger"], .p-select-dropdown').first();

    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
        await selectTrigger.click().catch(async () => selectHost.click());
        await page.waitForSelector('[role="option"]:visible', { timeout: 5_000 }).catch(() => undefined);

        const options = page.locator('[role="option"]:visible, .p-select-option:visible');
        const count = await options.count();
        for (let i = 0; i < count; i += 1) {
            const option = options.nth(i);
            const optionText = ((await option.textContent()) || "").trim();
            if (!pattern.test(optionText)) continue;
            if (await clickOption(option)) {
                return;
            }
        }

        await page.keyboard.press("Escape");
        await page.waitForTimeout(500);
    }

    throw new Error(`No visible option matching ${pattern} for ${selector}`);
}

async function clickOption(option) {
    try {
        await option.click({ timeout: 3_000 });
        return true;
    } catch {
        try {
            await option.click({ force: true, timeout: 3_000 });
            return true;
        } catch {
            return false;
        }
    }
}

async function cleanupCreatedImports(page, siteId, beforeIds, primaryImportId) {
    const imports = await listImports(page, siteId);
    const createdIds = imports.filter((job) => !beforeIds.has(job.id) && job.provider === "plausible").map((job) => job.id);
    if (primaryImportId && !createdIds.includes(primaryImportId)) {
        createdIds.push(primaryImportId);
    }

    for (const importId of createdIds) {
        await waitUntilImportDeletable(page, siteId, importId);
        const response = await page.request.delete(`/api/sites/${siteId}/imports/${importId}`, {
            headers: originHeaders(page)
        });
        expect([200, 404]).toContain(response.status());
    }
}

async function waitUntilImportDeletable(page, siteId, importId) {
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
        const response = await page.request.get(`/api/sites/${siteId}/imports/${importId}`);
        if (response.status() === 404) {
            return;
        }
        expect(response.ok()).toBeTruthy();
        const job = await response.json();
        if (!["queued", "running", "validating"].includes(job.status)) {
            return;
        }
        await page.waitForTimeout(500);
    }
}

function assertFixtureManifest(manifest) {
    expect(manifest).toBeTruthy();
    expect(manifest.rows_scanned).toBe(FIXTURE_ROWS_ACCEPTED);
    expect(manifest.rows_accepted).toBe(FIXTURE_ROWS_ACCEPTED);
    expect(manifest.rows_skipped).toBe(0);
    expect(manifest.files).toHaveLength(FIXTURE_CSV_COUNT);
    expect(manifest.ignored_files).toHaveLength(0);
    expect(manifest.missing_files).toHaveLength(0);
    expect(manifest.event_coverage.rows_scanned).toBe(2);
    expect(manifest.event_coverage.rows_accepted).toBe(2);
    expect(manifest.event_coverage.events).toBe(FIXTURE_EVENT_TOTAL);
    expect(manifest.event_coverage.visitors).toBe(6);
    expect(manifest.event_coverage.event_names).toEqual(["engagement", "outbound_click"]);
    expect(manifest.event_coverage.property_keys).toEqual(["path", "url"]);
    expect(manifest.event_property_coverage.unattributed_rows).toBe(FIXTURE_EVENT_PROPERTY_ROWS);
    expect(manifest.event_property_coverage.unattributed_events).toBe(5);
    expect(manifest.event_dimension_coverage.available).toEqual(["date", "event_name", "url", "path"]);
    expect(manifest.event_dimension_coverage.unavailable).toContain("browser");
    expect(manifest.overlap.policy).toBe("skip_native_day");
}

async function apiJSON(page, url) {
    const response = await page.request.get(url, { timeout: 15_000 });
    const body = await response.text();
    expect(response.ok(), `${url} returned ${response.status()}: ${body}`).toBeTruthy();
    return JSON.parse(body);
}

async function captureImportScreenshot(page, slug, testInfo) {
    const dirs = importScreenshotDirs();
    if (dirs.length === 0 && process.env.HITKEEP_IMPORT_ATTACH_SCREENSHOTS !== "1") {
        return;
    }

    await page.evaluate(() => window.scrollTo(0, 0));
    await page.mouse.move(1, 1);
    await page.waitForTimeout(100);

    const artifactPath = testInfo.outputPath(`${slug}.png`);
    await page.screenshot({ path: artifactPath, fullPage: true, animations: "disabled" });
    await testInfo.attach(slug, { path: artifactPath, contentType: "image/png" });

    if (shouldPersistImportScreenshot(slug)) {
        for (const dir of dirs) {
            fs.mkdirSync(dir, { recursive: true });
            fs.copyFileSync(artifactPath, path.join(dir, `${slug}.png`));
        }
    }
}

async function captureDarkImportScreenshot(page, slug, testInfo) {
    await setDarkMode(page, true);
    await captureImportScreenshot(page, slug, testInfo);
    await setDarkMode(page, false);
}

async function setDarkMode(page, enabled) {
    const html = page.locator("html");
    const isDark = async () => ((await html.getAttribute("class")) || "").includes("p-dark");
    if ((await isDark()) === enabled) {
        return;
    }

    const label = enabled ? "Switch to dark mode" : "Switch to light mode";
    await page.locator(`button[aria-label="${label}"]:visible`).first().click();
    await expect.poll(isDark).toBe(enabled);
}

function shouldPersistImportScreenshot(slug) {
    return process.env.HITKEEP_IMPORT_PERSIST_ALL_SCREENSHOTS === "1" || PERSISTED_IMPORT_SCREENSHOTS.has(slug);
}

function importScreenshotDirs() {
    const raw = process.env.HITKEEP_IMPORT_SCREENSHOT_DIRS || process.env.HITKEEP_IMPORT_SCREENSHOT_DIR || "";
    return raw
        .split(path.delimiter)
        .map((entry) => entry.trim())
        .filter(Boolean);
}

function originHeaders(page) {
    return { Origin: new URL(page.url()).origin };
}
