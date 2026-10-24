const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

test("export format menus download all-sites and per-site takeouts", async ({ page }) => {
    await page.setViewportSize({ width: 1110, height: 866 });
    await login(page, "/import-export/export");
    const originalLocale = await currentLocale(page);
    await setLocale(page, "de");
    const extraSite = await createSite(page);

    try {
        await page.goto("/import-export/export", { waitUntil: "domcontentloaded" });

        await expect(page.getByRole("heading", { name: "Export", level: 1, exact: true })).toBeVisible();
        await expect(page.locator("html")).toHaveAttribute("lang", /de/i);
        await expect(page.getByRole("heading", { name: /^Alle zugänglichen Sites$/ })).toBeVisible();
        await expect(page.getByRole("heading", { name: /^Site-Exporte$/ })).toBeVisible();

        const allSitesMenu = page.locator('[data-testid="all-sites-export-primary"] .p-splitbutton-dropdown').first();
        await expect(allSitesMenu).toBeVisible();
        await allSitesMenu.click();

        const allSitesDownload = page.waitForEvent("download");
        const allSitesResponse = page.waitForResponse((response) => response.url().includes("/api/user/takeout?format=json") && response.ok());
        await clickMenuItem(page, "JSON");
        const [allSitesDownloadEvent] = await Promise.all([allSitesDownload, allSitesResponse]);
        expect(allSitesDownloadEvent.suggestedFilename().toLowerCase()).toContain(".json");

        const siteRow = page.locator(".site-export-row").filter({ hasText: extraSite.domain }).first();
        await expect(siteRow).toBeVisible();

        const siteExportMenu = siteRow.locator(".p-splitbutton-dropdown").first();
        await expect(siteExportMenu).toBeVisible();
        await siteExportMenu.click();

        const siteDownload = page.waitForEvent("download");
        const siteResponse = page.waitForResponse((response) => response.url().includes(`/api/sites/${extraSite.id}/takeout?format=json`) && response.ok());
        await clickMenuItem(page, "JSON");
        const [siteDownloadEvent] = await Promise.all([siteDownload, siteResponse]);
        expect(siteDownloadEvent.suggestedFilename().toLowerCase()).toContain(".json");
    } finally {
        await deleteSite(page, extraSite.id);
        await setLocale(page, originalLocale);
    }
});

async function clickMenuItem(page, name) {
    const item = page.getByRole("menuitem", { name, exact: true });
    await expect(item).toBeVisible();
    await item.click();
}

async function setLocale(page, locale) {
    const response = await page.request.put("/api/user/preferences", {
        headers: originHeaders(page),
        data: { default_locale: locale }
    });
    const body = await response.text();
    expect(response.ok(), `set locale returned ${response.status()}: ${body}`).toBeTruthy();
}

async function currentLocale(page) {
    const response = await page.request.get("/api/user/preferences");
    const body = await response.text();
    expect(response.ok(), `get locale returned ${response.status()}: ${body}`).toBeTruthy();
    return JSON.parse(body).default_locale || "en";
}

async function createSite(page) {
    const domain = `site-export-e2e-${Date.now()}.example.test`;
    const response = await page.request.post("/api/sites", {
        headers: originHeaders(page),
        data: { domain }
    });
    const body = await response.text();
    expect(response.ok(), `create site returned ${response.status()}: ${body}`).toBeTruthy();
    return JSON.parse(body);
}

async function deleteSite(page, siteId) {
    const response = await page.request.delete(`/api/sites/${siteId}`, {
        headers: originHeaders(page)
    });
    expect([200, 404]).toContain(response.status());
}

function originHeaders(page) {
    return { Origin: new URL(page.url()).origin };
}
