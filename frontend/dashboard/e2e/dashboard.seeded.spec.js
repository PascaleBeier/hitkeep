const { test, expect } = require("playwright/test");
const { E2E_SHARE_TOKEN, login } = require("./support/auth");

const CHART_SETTLE_MS = 2500;
const TABLE_SETTLE_MS = 1000;
const PRIMARY_SEEDED_SITE_DOMAIN = "acme-analytics.io";
const SEEDED_CITY_RE = /Mountain View|New York|Seattle|Berlin|Munich|London|Paris|Amsterdam/;
const SEEDED_PROVIDER_RE = /Google LLC|Verizon Business|Comcast Cable|Deutsche Telekom AG|Vodafone GmbH|BT|Orange|KPN/;
const SEEDED_ASN_RE = /AS15169|AS701|AS7922|AS3320|AS3209|AS2856|AS3215|AS1136/;

async function selectFirstVisibleOption(page, selector) {
    const select = page.locator(`${selector}:visible`).first();
    await expect(select).toBeVisible();
    await expect(select).toBeEnabled();

    await select.click();

    const option = page.locator('[role="option"]:visible').first();
    await expect(option).toBeVisible();

    const optionText = ((await option.textContent()) || "").trim();
    await option.click();
    await page.waitForTimeout(TABLE_SETTLE_MS);
    return optionText;
}

async function selectFirstVisibleComboboxOption(page, name) {
    const select = page.getByRole("combobox", { name }).first();
    await expect(select).toBeVisible();
    await expect(select).toBeEnabled();

    await select.click();

    const option = page.locator('[role="option"]:visible').first();
    await expect(option).toBeVisible();

    const optionText = ((await option.textContent()) || "").trim();
    await option.click();
    await page.waitForTimeout(TABLE_SETTLE_MS);
    return optionText;
}

async function selectSeededSite(page, domain = PRIMARY_SEEDED_SITE_DOMAIN) {
    const combobox = page.locator('[role="combobox"]:visible').first();
    await expect(combobox).toBeVisible();

    const currentSite = ((await combobox.textContent()) || "").trim();
    if (currentSite.includes(domain)) {
        return;
    }

    if (
        await page
            .getByText(domain)
            .first()
            .isVisible()
            .catch(() => false)
    ) {
        return;
    }

    await page.locator('[aria-label="Select a site to view stats"]:visible').first().click();

    const option = page.locator('[role="option"]:visible').filter({ hasText: domain }).first();
    await expect(option).toBeVisible();
    await option.click();

    await expect(combobox).toContainText(domain);
    await page.waitForTimeout(TABLE_SETTLE_MS);
}

async function expectSeededGeoNetworkMetrics(page) {
    await revealMetricCard(page, "Cities");
    await expect(page.locator("app-metric-list:visible").filter({ hasText: SEEDED_CITY_RE }).first()).toBeVisible();

    await revealMetricCard(page, "Providers");
    await expect(page.locator("app-metric-list:visible").filter({ hasText: SEEDED_PROVIDER_RE }).first()).toBeVisible();

    await revealMetricCard(page, "ASNs");
    await expect(page.locator("app-metric-list:visible").filter({ hasText: SEEDED_ASN_RE }).first()).toBeVisible();
}

async function revealMetricCard(page, title) {
    const tab = page.getByRole("tab", { name: title, exact: true }).first();
    if (await tab.isVisible().catch(() => false)) {
        await tab.click();
        await page.waitForTimeout(250);
        return;
    }

    await expect(page.getByText(title, { exact: true }).first()).toBeVisible();
}

async function clickSeededMetricRow(page, title, valuePattern) {
    await revealMetricCard(page, title);

    const metricList = page.locator("app-metric-list:visible").filter({ hasText: valuePattern }).first();
    await expect(metricList).toBeVisible();

    const row = metricList.getByRole("button").filter({ hasText: valuePattern }).first();
    await expect(row).toBeVisible();
    await row.click();
    await page.waitForTimeout(TABLE_SETTLE_MS);
}

async function expectMetricCardGroupPolish(page, expectedCardCount = 5) {
    await expect(page.locator(".metric-card-group")).toBeVisible();

    const result = await page.evaluate(() => {
        const cards = [...document.querySelectorAll(".metric-card-group__card")].map((card) => {
            const rect = card.getBoundingClientRect();
            const scrollShells = [...card.querySelectorAll(".metric-list__scroll-shell")];
            const scrollableShells = scrollShells.filter((shell) => shell.classList.contains("metric-list__scroll-shell--scrollable"));
            return {
                title: card.querySelector(".metric-card-group__title")?.textContent?.trim() || "",
                height: Math.round(rect.height),
                width: Math.round(rect.width),
                tabCount: card.querySelectorAll("p-tab").length,
                scrollableCount: scrollableShells.length,
                visibleScrollbarCount: scrollableShells.filter((shell) => {
                    const scrollbar = shell.querySelector(".metric-list__scrollbar");
                    return scrollbar && getComputedStyle(scrollbar).opacity === "1";
                }).length,
                visibleFadeCount: scrollableShells.filter((shell) => {
                    const fade = shell.querySelector(".metric-list__scroll-fade");
                    return fade && getComputedStyle(fade).opacity === "1";
                }).length
            };
        });

        return {
            cardCount: cards.length,
            cards,
            overflowX: document.documentElement.scrollWidth - document.documentElement.clientWidth
        };
    });

    expect(result.overflowX).toBeLessThanOrEqual(1);
    expect(result.cardCount).toBe(expectedCardCount);
    expect(new Set(result.cards.map((card) => card.height)).size).toBe(1);
    expect(Math.min(...result.cards.map((card) => card.width))).toBeGreaterThan(280);
    expect(result.cards.some((card) => card.tabCount > 0)).toBeTruthy();
    expect(result.cards.some((card) => card.scrollableCount > 0 && card.visibleScrollbarCount > 0 && card.visibleFadeCount > 0)).toBeTruthy();
}

test("dashboard renders seeded data and product controls", async ({ page }) => {
    await login(page, "/dashboard");
    await selectSeededSite(page);
    await expect(page).toHaveURL(/\/dashboard(?:\?.*)?$/);
    await expect(page.getByText("Latest Hits")).toBeVisible();
    await expect(page.getByText("Top Sources")).toBeVisible();
    await expect(page.getByText("Cities")).toBeVisible();
    await expect(page.getByText("Providers")).toBeVisible();
    await expect(page.getByText("ASNs")).toBeVisible();
    await expectSeededGeoNetworkMetrics(page);
    const teamSwitcher = page.locator('[data-testid="team-switcher-trigger"]:visible').first();
    await expect(teamSwitcher).toBeVisible();
    await expect(teamSwitcher).toContainText("Acme Analytics");

    await page.getByRole("button", { name: /share dashboard/i }).click();
    await expect(page.getByRole("dialog").getByText("Share dashboard")).toBeVisible();
    await expect(page.getByRole("button", { name: /generate/i })).toBeVisible();
    await page.getByRole("button", { name: /close/i }).click();

    await page.getByRole("button", { name: /site settings/i }).click();
    await expect(page.getByText("Site settings")).toBeVisible();
    await page.getByRole("tab", { name: /tracking/i }).click();
    await expect(page.getByText("Automatic event tracking")).toBeVisible();
    await expect(page.getByText("Track outbound clicks")).toBeVisible();
    await expect(page.getByText("Track file downloads")).toBeVisible();
    await expect(page.getByText("Track form submissions")).toBeVisible();

    await page.getByRole("tab", { name: /team/i }).click();
    await expect(page.getByRole("heading", { name: "Transfer site" })).toBeVisible();
});

test("dashboard filters by seeded geography and network metrics", async ({ page }) => {
    await login(page, "/dashboard");
    await selectSeededSite(page);
    await page.waitForTimeout(TABLE_SETTLE_MS);

    await clickSeededMetricRow(page, "Cities", SEEDED_CITY_RE);
    await expect(page.getByText(/City: (Mountain View|New York|Seattle|Berlin|Munich|London|Paris|Amsterdam)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "Providers", SEEDED_PROVIDER_RE);
    await expect(page.getByText(/Provider: (Google LLC|Verizon Business|Comcast Cable|Deutsche Telekom AG|Vodafone GmbH|BT|Orange|KPN)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "ASNs", SEEDED_ASN_RE);
    await expect(page.getByText(/ASN: (AS15169|AS701|AS7922|AS3320|AS3209|AS2856|AS3215|AS1136)/)).toBeVisible();
});

test("ecommerce page surfaces seeded orders and products", async ({ page }) => {
    await login(page, "/ecommerce");
    await selectSeededSite(page);
    await page.waitForTimeout(CHART_SETTLE_MS);

    await expect(page.getByText("Revenue over time")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Revenue breakdown" })).toBeVisible();
    await expect(page.getByText("Top products")).toBeVisible();
    await expect(page.getByRole("tab", { name: "Revenue sources" })).toBeVisible();
    await expect(page.getByText("Cities")).toBeVisible();
    await expect(page.getByText("Providers")).toBeVisible();
    await expect(page.getByText("ASNs")).toBeVisible();
    await expectSeededGeoNetworkMetrics(page);
    await expect(page.getByText("Pro Plan")).toBeVisible();
});

test("ecommerce page filters by seeded geography and network metrics", async ({ page }) => {
    await login(page, "/ecommerce");
    await selectSeededSite(page);
    await page.waitForTimeout(CHART_SETTLE_MS);

    await clickSeededMetricRow(page, "Cities", SEEDED_CITY_RE);
    await expect(page.getByText(/City: (Mountain View|New York|Seattle|Berlin|Munich|London|Paris|Amsterdam)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "Providers", SEEDED_PROVIDER_RE);
    await expect(page.getByText(/Provider: (Google LLC|Verizon Business|Comcast Cable|Deutsche Telekom AG|Vodafone GmbH|BT|Orange|KPN)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "ASNs", SEEDED_ASN_RE);
    await expect(page.getByText(/ASN: (AS15169|AS701|AS7922|AS3320|AS3209|AS2856|AS3215|AS1136)/)).toBeVisible();
});

test("events page surfaces seeded audience geography and network data", async ({ page }) => {
    await login(page, "/events");
    await selectSeededSite(page);
    await page.waitForTimeout(TABLE_SETTLE_MS);

    const selectedEvent = await selectFirstVisibleComboboxOption(page, "Event");
    expect(selectedEvent).not.toBe("");
    await expect(page.getByRole("heading", { name: "Event activity" })).toBeVisible();
    await expect(page.getByText("Total events")).toBeVisible();
    await expect(page.getByText("Cities")).toBeVisible();
    await expect(page.getByText("Providers")).toBeVisible();
    await expect(page.getByText("ASNs")).toBeVisible();
    await expectSeededGeoNetworkMetrics(page);
});

test("events page filters by seeded audience geography and network metrics", async ({ page }) => {
    await login(page, "/events");
    await selectSeededSite(page);
    await page.waitForTimeout(TABLE_SETTLE_MS);

    const selectedEvent = await selectFirstVisibleComboboxOption(page, "Event");
    expect(selectedEvent).not.toBe("");

    await clickSeededMetricRow(page, "Cities", SEEDED_CITY_RE);
    await expect(page.getByText(/City: (Mountain View|New York|Seattle|Berlin|Munich|London|Paris|Amsterdam)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "Providers", SEEDED_PROVIDER_RE);
    await expect(page.getByText(/Provider: (Google LLC|Verizon Business|Comcast Cable|Deutsche Telekom AG|Vodafone GmbH|BT|Orange|KPN)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "ASNs", SEEDED_ASN_RE);
    await expect(page.getByText(/ASN: (AS15169|AS701|AS7922|AS3320|AS3209|AS2856|AS3215|AS1136)/)).toBeVisible();
});

test("secondary analytics metric cards keep equal-height tabbed surfaces and scroll affordances", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 1000 });
    await login(page, "/events");
    await selectSeededSite(page);
    await page.waitForTimeout(TABLE_SETTLE_MS);
    const selectedEvent = await selectFirstVisibleComboboxOption(page, "Event");
    expect(selectedEvent).not.toBe("");
    await expectMetricCardGroupPolish(page);

    await page.setViewportSize({ width: 390, height: 900 });
    await login(page, "/ai-chatbots");
    await page.waitForTimeout(CHART_SETTLE_MS);
    await expectMetricCardGroupPolish(page);
});

test("goals page surfaces seeded geography and network data", async ({ page }) => {
    await login(page, "/goals");
    await selectSeededSite(page);
    await page.waitForTimeout(TABLE_SETTLE_MS);

    await expect(page.getByRole("heading", { name: "Goals" }).first()).toBeVisible();
    await expect(page.getByText("Conversions", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("Cities")).toBeVisible();
    await expect(page.getByText("Providers")).toBeVisible();
    await expect(page.getByText("ASNs")).toBeVisible();
    await expectSeededGeoNetworkMetrics(page);
});

test("goals page filters by seeded geography and network metrics", async ({ page }) => {
    await login(page, "/goals");
    await selectSeededSite(page);
    await page.waitForTimeout(TABLE_SETTLE_MS);

    await clickSeededMetricRow(page, "Cities", SEEDED_CITY_RE);
    await expect(page.getByText(/City: (Mountain View|New York|Seattle|Berlin|Munich|London|Paris|Amsterdam)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "Providers", SEEDED_PROVIDER_RE);
    await expect(page.getByText(/Provider: (Google LLC|Verizon Business|Comcast Cable|Deutsche Telekom AG|Vodafone GmbH|BT|Orange|KPN)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "ASNs", SEEDED_ASN_RE);
    await expect(page.getByText(/ASN: (AS15169|AS701|AS7922|AS3320|AS3209|AS2856|AS3215|AS1136)/)).toBeVisible();
});

test("funnels page surfaces seeded geography and network data", async ({ page }) => {
    await login(page, "/funnels");
    await selectSeededSite(page);
    await page.waitForTimeout(TABLE_SETTLE_MS);

    await expect(page.getByText("Entries", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("Cities")).toBeVisible();
    await expect(page.getByText("Providers")).toBeVisible();
    await expect(page.getByText("ASNs")).toBeVisible();
    await expectSeededGeoNetworkMetrics(page);
});

test("funnels page filters by seeded geography and network metrics", async ({ page }) => {
    await login(page, "/funnels");
    await selectSeededSite(page);
    await page.waitForTimeout(TABLE_SETTLE_MS);

    await clickSeededMetricRow(page, "Cities", SEEDED_CITY_RE);
    await expect(page.getByText(/City: (Mountain View|New York|Seattle|Berlin|Munich|London|Paris|Amsterdam)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "Providers", SEEDED_PROVIDER_RE);
    await expect(page.getByText(/Provider: (Google LLC|Verizon Business|Comcast Cable|Deutsche Telekom AG|Vodafone GmbH|BT|Orange|KPN)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "ASNs", SEEDED_ASN_RE);
    await expect(page.getByText(/ASN: (AS15169|AS701|AS7922|AS3320|AS3209|AS2856|AS3215|AS1136)/)).toBeVisible();
});

test("ai visibility page shows correlation insights", async ({ page }) => {
    await login(page, "/ai-visibility");
    await selectSeededSite(page);
    await page.waitForTimeout(CHART_SETTLE_MS);

    await expect(page.getByRole("heading", { name: "Fetch volume over time" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Fetch-to-visit correlation" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Citation yield" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Opportunity pages" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Failure hotspots" })).toBeVisible();
    await expect(page.getByText("GPTBot").first()).toBeVisible();
});

test("ai chatbot page surfaces seeded audience geography and network data", async ({ page }) => {
    await login(page, "/ai-chatbots");
    await selectSeededSite(page);
    await page.waitForTimeout(CHART_SETTLE_MS);

    await expect(page.getByRole("heading", { name: "Conversation activity" })).toBeVisible();
    await expect(page.getByText("Conversations", { exact: true }).first()).toBeVisible();
    await expect(page.getByRole("tab", { name: "Cities", exact: true })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Providers", exact: true })).toBeVisible();
    await expect(page.getByRole("tab", { name: "ASNs", exact: true })).toBeVisible();
    await expectSeededGeoNetworkMetrics(page);
});

test("ai chatbot page filters by seeded audience geography and network metrics", async ({ page }) => {
    await login(page, "/ai-chatbots");
    await selectSeededSite(page);
    await page.waitForTimeout(CHART_SETTLE_MS);

    await clickSeededMetricRow(page, "Cities", SEEDED_CITY_RE);
    await expect(page.getByText(/City: (Mountain View|New York|Seattle|Berlin|Munich|London|Paris|Amsterdam)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "Providers", SEEDED_PROVIDER_RE);
    await expect(page.getByText(/Provider: (Google LLC|Verizon Business|Comcast Cable|Deutsche Telekom AG|Vodafone GmbH|BT|Orange|KPN)/)).toBeVisible();
    await page.getByRole("button", { name: "Clear all" }).click();
    await expect(page.getByText("No active filter")).toBeVisible();

    await clickSeededMetricRow(page, "ASNs", SEEDED_ASN_RE);
    await expect(page.getByText(/ASN: (AS15169|AS701|AS7922|AS3320|AS3209|AS2856|AS3215|AS1136)/)).toBeVisible();
});

test("team admin page shows seeded members and settings", async ({ page }) => {
    await login(page, "/admin/team");
    await expect(page.getByRole("heading", { name: "Acme Analytics" })).toBeVisible();

    await page.getByRole("tab", { name: /^Members$/i }).click();
    await expect(page.getByText("bob@devtools.co")).toBeVisible();
    await expect(page.getByText("diana@saaslaunch.com")).toBeVisible();

    await page.getByRole("tab", { name: /^Settings$/i }).click();
    await expect(page.getByText("Team name")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Team API clients" })).toBeVisible();

    await page.getByRole("tab", { name: /^Activity$/i }).click();
    await expect(page.getByRole("search", { name: "Audit filters" })).toBeVisible();
});

test("public share links render seeded analytics without login", async ({ page }) => {
    await page.goto(`/share/${E2E_SHARE_TOKEN}/dashboard`, { waitUntil: "domcontentloaded" });
    await page.waitForTimeout(CHART_SETTLE_MS);

    await expect(page).toHaveURL(new RegExp(`/share/${E2E_SHARE_TOKEN}/dashboard`));
    await expect(page.getByText("Latest Hits")).toBeVisible();
    await expect(page.getByText("Top Sources")).toBeVisible();
    await expect(page.getByText("Cities")).toBeVisible();
    await expect(page.getByText("Providers")).toBeVisible();
    await expect(page.getByText("ASNs")).toBeVisible();
    await expectSeededGeoNetworkMetrics(page);
    await expect(page.getByRole("button", { name: /share dashboard/i })).toHaveCount(0);
});
