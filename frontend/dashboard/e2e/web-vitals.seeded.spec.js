const { test, expect } = require("playwright/test");
const { E2E_SHARE_TOKEN, login } = require("./support/auth");

const TABLE_SETTLE_MS = 1000;
const PRIMARY_SEEDED_SITE_DOMAIN = "acme-analytics.io";
const SEEDED_CITY_RE = /Mountain View|New York|Seattle|Berlin|Munich|London|Paris|Amsterdam/;
const SEEDED_PROVIDER_RE = /Google LLC|Verizon Business|Comcast Cable|Deutsche Telekom AG|Vodafone GmbH|BT|Orange|KPN/;
const SEEDED_ASN_RE = /AS15169|AS701|AS7922|AS3320|AS3209|AS2856|AS3215|AS1136/;

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
    await page.waitForTimeout(TABLE_SETTLE_MS);
}

async function expectBreakdownRow(page, tabName, valuePattern) {
    await page.getByRole("tab", { name: tabName }).click();
    const panel = page.getByRole("tabpanel", { name: tabName });
    await expect(panel).toBeVisible();
    await expect(panel.locator(".web-vitals-table tbody tr").filter({ hasText: valuePattern }).first()).toBeVisible();
}

test("web vitals dashboard renders seeded data and filters", async ({ page }) => {
    await login(page, "/web-vitals");
    await selectSeededSite(page);

    await expect(page.getByRole("button", { name: "Inspect LCP over time" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Inspect INP over time" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Inspect CLS over time" })).toBeVisible();
    await expect(page.getByText("Path: /")).toBeVisible();
    await expect(page.getByRole("heading", { name: "LCP p75 trend" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Rating mix" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Page breakdown" })).toBeVisible();
    await expect(page.locator(".web-vitals-table tbody tr").filter({ hasText: "/" }).first()).toBeVisible();

    await page.getByRole("button", { name: "Inspect INP over time" }).click();
    await expect(page.getByRole("heading", { name: "INP p75 trend" })).toBeVisible();

    await page.getByRole("button", { name: /^Good\s+\d+/ }).click();
    await expect(page.getByText("Rating: Good")).toBeVisible();

    await page.locator(".web-vitals-table tbody tr").filter({ hasText: "/pricing" }).first().click();
    await expect(page.getByText("Path: /pricing")).toBeVisible();
    await expect(page.locator(".web-vitals-table tbody tr").filter({ hasText: "/pricing" }).first()).toBeVisible();
    await page.getByRole("tab", { name: "Browsers" }).click();
    const browsersPanel = page.getByRole("tabpanel", { name: "Browsers" });
    await expect(browsersPanel).toBeVisible();
    await expect(
        browsersPanel
            .locator(".web-vitals-table tbody tr")
            .filter({ hasText: /Chrome|Safari|Firefox|Edge/ })
            .first()
    ).toBeVisible();

    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("Path: /")).toBeVisible();
    await expect(page.getByText("Rating: Good")).toHaveCount(0);

    await expectBreakdownRow(page, "Cities", SEEDED_CITY_RE);
    await expectBreakdownRow(page, "Providers", SEEDED_PROVIDER_RE);
    await expectBreakdownRow(page, "ASNs", SEEDED_ASN_RE);
});

test("shared dashboard exposes seeded Web Vitals", async ({ page }) => {
    await page.goto(`/share/${E2E_SHARE_TOKEN}/web-vitals`, { waitUntil: "domcontentloaded" });
    await page.waitForTimeout(TABLE_SETTLE_MS);

    await expect(page).toHaveURL(new RegExp(`/share/${E2E_SHARE_TOKEN}/web-vitals`));
    await expect(page.getByRole("button", { name: "Inspect LCP over time" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "LCP p75 trend" })).toBeVisible();
    await expect(page.locator(".web-vitals-table tbody tr").filter({ hasText: "/" }).first()).toBeVisible();
    await expectBreakdownRow(page, "Cities", SEEDED_CITY_RE);
    await expectBreakdownRow(page, "Providers", SEEDED_PROVIDER_RE);
    await expectBreakdownRow(page, "ASNs", SEEDED_ASN_RE);
    await expect(page.getByRole("button", { name: /share dashboard/i })).toHaveCount(0);
});
