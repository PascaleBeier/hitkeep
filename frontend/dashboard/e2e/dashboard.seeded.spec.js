const { test, expect } = require("playwright/test");
const { E2E_SHARE_TOKEN, login } = require("./support/auth");

const PRIMARY_SEEDED_SITE_DOMAIN = "acme-analytics.io";
const SEEDED_EVENT_NAME = "newsletter_signup";
const SEEDED_EVENT_RANGE = "30d";
// Any second range works; the switch itself is what the smooth-reload test checks.
const ALTERNATE_RANGE = "7d";
const SEEDED_CITY_RE = /Mountain View|New York|Seattle|Berlin|Munich|London|Paris|Amsterdam/;
const SEEDED_PROVIDER_RE = /Google LLC|Verizon Business|Comcast Cable|Deutsche Telekom AG|Vodafone GmbH|BT|Orange|KPN/;
const SEEDED_ASN_RE = /AS15169|AS701|AS7922|AS3320|AS3209|AS2856|AS3215|AS1136/;
const SEEDED_AI_SOURCE_RE = /ChatGPT|Claude|Perplexity|Gemini|DeepSeek/;
// Seeded hits carry AI crawler user agents, and GPTBot is the heaviest-weighted
// one, so it is always present in a 30 day window.
const SEEDED_AI_AGENT_RE = /GPTBot/;
// The seeded crawler mix only covers these three categories.
const SEEDED_AI_CATEGORY_RE = /Training crawlers|AI search crawlers|AI assistants/;
// Seeded every day by `seedDefaultRangeAIFetch`, and one of the heaviest
// weighted crawl targets, so it always shows up in the merged pages breakdown.
const SEEDED_AI_PATH = "/docs/getting-started";
// `aiAgents.provenance.hint` renders `tracked {{tracked}} · logs {{fetched}}` for
// rows the merge saw on both sides.
const SEEDED_AI_PROVENANCE_RE = /tracked [\d,.]+ · logs [\d,.]+/;
const AI_AGENTS_PAGE_CHIPS = '[data-testid="ai-agents-page-chips"]';
const AI_AGENTS_TRAFFIC_KPIS = '[data-testid="ai-agent-traffic-kpis"]';
const AI_VISIBILITY_FETCH_DEPTH_STRIPS = '[data-testid="ai-visibility-fetch-depth-strips"]';
const AI_VISIBILITY_CORRELATION_STRIP = '[data-testid="ai-visibility-stat-group-correlation"]';
const AI_VISIBILITY_CORRELATION_TABLES = '[data-testid="metric-card-group-correlation"]';

async function selectComboboxOption(page, name, optionName) {
    const select = page.getByRole("combobox", { name }).first();
    await expect(select).toBeVisible();
    await expect(select).toBeEnabled();

    const currentText = ((await select.textContent()) || "").trim();
    if (currentText.includes(optionName)) {
        return optionName;
    }

    await select.click();

    const option = page.getByRole("option", { name: optionName, exact: true }).first();
    await expect(option).toBeVisible();

    await option.click();
    await expect(page.getByText(optionName, { exact: true }).first()).toBeVisible();
    return optionName;
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
}

async function selectSeededEvent(page) {
    await selectRange(page, SEEDED_EVENT_RANGE);
    return selectComboboxOption(page, "Event", SEEDED_EVENT_NAME);
}

async function selectRange(page, label) {
    const rangeButton = page.getByRole("button", { name: label, exact: true }).first();
    await expect(rangeButton).toBeVisible();

    if ((await rangeButton.getAttribute("aria-pressed")) === "true") {
        return;
    }

    await rangeButton.click();
    await expect(rangeButton).toHaveAttribute("aria-pressed", "true");
}

async function revealMetricCard(page, title) {
    const tab = page.getByRole("tab", { name: title, exact: true }).first();
    await expect(tab).toBeVisible();
    await tab.click();

    const panel = page.getByRole("tabpanel", { name: title, exact: true }).first();
    await expect(panel).toBeVisible();
    return panel;
}

async function expectSeededMetricValue(page, title, valuePattern) {
    const panel = await revealMetricCard(page, title);
    await expect(panel.getByText(valuePattern).first()).toBeVisible();
    return panel;
}

async function expectSeededGeoNetworkMetrics(page) {
    await expectSeededMetricValue(page, "Cities", SEEDED_CITY_RE);
    await expectSeededMetricValue(page, "Providers", SEEDED_PROVIDER_RE);
    await expectSeededMetricValue(page, "ASNs", SEEDED_ASN_RE);
}

async function clickSeededMetricRow(page, title, valuePattern) {
    const panel = await expectSeededMetricValue(page, title, valuePattern);

    const row = panel.getByRole("button").filter({ hasText: valuePattern }).first();
    await expect(row).toBeVisible();
    // Path rows carry an "open in new tab" link, so the click has to land on the
    // label rather than anywhere inside the row.
    const label = row.locator(".metric-list__label").first();
    const target = (await label.count()) > 0 ? label : row;
    await target.click();
}

/**
 * Numeric value of one stat-strip entry. KPI cards render their number through
 * ng-number-flow's custom element, which stays empty under the dashboard's CSP,
 * so scalar assertions read the plain-text strips instead.
 */
async function readStatStripValue(page, containerSelector, label) {
    const entry = page.locator(containerSelector).locator(".stat-groups__tile").filter({ hasText: label }).first();
    await expect(entry).toBeVisible();
    const text = (await entry.innerText()) || "";
    return Number(text.replace(/^[^\d]*/, "").replace(/[^\d]/g, "") || "0");
}

async function expectMetricCardGroupPolish(page, expectedCardCount = 5) {
    await expect(page.locator(".metric-card-group")).toBeVisible();

    await expect
        .poll(async () => {
            const result = await collectMetricCardGroupState(page);
            return metricCardGroupHasPolish(result, expectedCardCount);
        })
        .toBe(true);

    const result = await collectMetricCardGroupState(page);

    expect(result.overflowX).toBeLessThanOrEqual(1);
    expect(result.cardCount).toBe(expectedCardCount);
    expect(new Set(result.cards.map((card) => card.height)).size).toBe(1);
    expect(Math.min(...result.cards.map((card) => card.width))).toBeGreaterThan(280);
    expect(result.cards.some((card) => card.tabCount > 0)).toBeTruthy();
    expect(result.cards.some((card) => card.scrollableCount > 0 && card.visibleScrollbarCount > 0 && card.visibleFadeCount > 0)).toBeTruthy();
}

async function collectMetricCardGroupState(page) {
    return page.evaluate(() => {
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
                    return scrollbar && Number(getComputedStyle(scrollbar).opacity) > 0.9;
                }).length,
                visibleFadeCount: scrollableShells.filter((shell) => {
                    const fade = shell.querySelector(".metric-list__scroll-fade");
                    return fade && Number(getComputedStyle(fade).opacity) > 0.9;
                }).length
            };
        });

        return {
            cardCount: cards.length,
            cards,
            overflowX: document.documentElement.scrollWidth - document.documentElement.clientWidth
        };
    });
}

function metricCardGroupHasPolish(result, expectedCardCount) {
    return (
        result.overflowX <= 1 &&
        result.cardCount === expectedCardCount &&
        new Set(result.cards.map((card) => card.height)).size === 1 &&
        Math.min(...result.cards.map((card) => card.width)) > 280 &&
        result.cards.some((card) => card.tabCount > 0) &&
        result.cards.some((card) => card.scrollableCount > 0 && card.visibleScrollbarCount > 0 && card.visibleFadeCount > 0)
    );
}

test("dashboard switches date ranges without tearing the KPIs and chart down", async ({ page }) => {
    await login(page, "/dashboard");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

    const chartCanvas = page.locator("[echarts] canvas").first();
    await expect(chartCanvas).toBeVisible();
    const kpiValue = page.locator("app-kpi-card app-animated-number").first();
    await expect(kpiValue).toBeAttached();

    // Tag the live nodes, switch range, and check the same nodes are still in
    // the document: a skeleton or a chart teardown would have replaced them,
    // which is exactly what stops the numbers and the graph from animating.
    await chartCanvas.evaluate((node) => (window.__hkChartCanvas = node));
    await kpiValue.evaluate((node) => (window.__hkKpiValue = node));

    await selectRange(page, ALTERNATE_RANGE);
    await expect(page.locator("app-kpi-card p-skeleton")).toHaveCount(0);
    await expect(chartCanvas).toBeVisible();

    const survived = await page.evaluate(() => ({
        chart: window.__hkChartCanvas?.isConnected === true,
        kpi: window.__hkKpiValue?.isConnected === true
    }));
    expect(survived).toEqual({ chart: true, kpi: true });
});

test("dashboard renders seeded data and product controls", async ({ page }) => {
    await login(page, "/dashboard");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);
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
    await expect(page).toHaveURL(/\/sites\/[^/]+\/settings\/general$/);
    await expect(page.getByRole("tab", { name: /general/i })).toHaveAttribute("aria-selected", "true");
    await page.getByRole("tab", { name: /tracking/i }).click();
    await expect(page).toHaveURL(/\/sites\/[^/]+\/settings\/tracking$/);
    await expect(page.getByTestId("tracking-snippet")).toContainText("hk.js");

    const installMethods = page.getByTestId("tracking-install-method");
    await installMethods.getByText("WordPress", { exact: true }).click();
    await expect(page.getByTestId("tracking-wordpress-directory-link")).toHaveAttribute("href", "https://wordpress.org/plugins/hitkeep/");
    await installMethods.getByText("Server-side", { exact: true }).click();
    await expect(page.getByTestId("tracking-server-snippet")).toContainText("/api/ingest/server/pageview");
    await installMethods.getByText("npm", { exact: true }).click();
    await expect(page.getByTestId("tracking-npm-install")).toContainText("npm install @hitkeep/tracker");
    await installMethods.getByText("Script tag", { exact: true }).click();

    await page.getByRole("button", { name: /advanced options/i }).click();
    await expect(page.getByText("Automatic event tracking")).toBeVisible();
    await expect(page.getByText("Track outbound clicks")).toBeVisible();
    await expect(page.getByText("Track file downloads")).toBeVisible();
    await expect(page.getByText("Track form submissions")).toBeVisible();

    await page.reload();
    await expect(page).toHaveURL(/\/sites\/[^/]+\/settings\/tracking$/);
    await expect(page.getByTestId("tracking-snippet")).toContainText("hk.js");

    await page.getByRole("tab", { name: /access/i }).click();
    await expect(page).toHaveURL(/\/sites\/[^/]+\/settings\/access$/);
    await expect(page.getByRole("heading", { name: "Transfer site" })).toBeVisible();
});

test("team invitation route opens and closes the invite dialog", async ({ page }) => {
    await login(page, "/admin/team/members/invite");

    await expect(page).toHaveURL(/\/admin\/team\/members\/invite$/);
    await expect(page.getByRole("dialog").getByText("Invite team member")).toBeVisible();
    await page.getByRole("dialog").getByRole("button", { name: "Cancel" }).click();
    await expect(page).toHaveURL(/\/admin\/team\/members$/);
});

test("invalid site settings URLs fall back to overview with a notice", async ({ page }) => {
    await login(page, "/sites/not-accessible/settings/general");

    await expect(page).toHaveURL(/\/overview$/);
    await expect(page.getByText("This site is unavailable or you no longer have access.")).toBeVisible();
});

test("dashboard filters by seeded geography and network metrics", async ({ page }) => {
    await login(page, "/dashboard");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

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
    await selectRange(page, SEEDED_EVENT_RANGE);

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
    await selectRange(page, SEEDED_EVENT_RANGE);

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

    const selectedEvent = await selectSeededEvent(page);
    expect(selectedEvent).toBe(SEEDED_EVENT_NAME);
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

    const selectedEvent = await selectSeededEvent(page);
    expect(selectedEvent).toBe(SEEDED_EVENT_NAME);

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
    const selectedEvent = await selectSeededEvent(page);
    expect(selectedEvent).toBe(SEEDED_EVENT_NAME);
    await expectMetricCardGroupPolish(page);

    await page.setViewportSize({ width: 390, height: 900 });
    await login(page, "/ai-chatbots");
    await selectRange(page, SEEDED_EVENT_RANGE);
    await expectMetricCardGroupPolish(page);
});

test("goals page surfaces seeded geography and network data", async ({ page }) => {
    await login(page, "/goals");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

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
    await selectRange(page, SEEDED_EVENT_RANGE);

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
    await selectRange(page, SEEDED_EVENT_RANGE);

    await expect(page.getByText("Entries", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("Cities")).toBeVisible();
    await expect(page.getByText("Providers")).toBeVisible();
    await expect(page.getByText("ASNs")).toBeVisible();
    await expectSeededGeoNetworkMetrics(page);
});

test("funnels page filters by seeded geography and network metrics", async ({ page }) => {
    await login(page, "/funnels");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

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

test("ai agents page shows the fetch depth section with correlation insights", async ({ page }) => {
    await login(page, "/ai-agents");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

    // The fetch depth section lives inside the single AI agents page, and its
    // scalars split into three themed strips once fetch records exist.
    await expect(page.locator(AI_VISIBILITY_FETCH_DEPTH_STRIPS)).toBeVisible();
    await expect(page.getByRole("heading", { name: "Crawler fetch depth" })).toBeVisible();
    await expect(page.locator(AI_VISIBILITY_FETCH_DEPTH_STRIPS).getByRole("heading")).toHaveText(["Fetch volume", "Fetch-to-visit", "Delivery health"]);
    await expect.poll(async () => readStatStripValue(page, AI_VISIBILITY_CORRELATION_STRIP, "Correlated paths")).toBeGreaterThan(0);
    await expect(page.locator(AI_VISIBILITY_CORRELATION_STRIP).getByText("Later AI-referred visits", { exact: true })).toBeVisible();
    // The correlation breakdowns are one group of the single card grid, not a
    // second one stacked below it.
    await expect(page.locator(AI_VISIBILITY_CORRELATION_TABLES)).toBeVisible();
    await expect(page.locator("app-metric-card-group")).toHaveCount(1);
    await expect(page.getByRole("heading", { name: "Correlation breakdowns" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Citation yield" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Opportunity pages" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Failure hotspots" })).toBeVisible();
    await expect(page.getByText("GPTBot").first()).toBeVisible();
    // There is no tab bar any more: one page, stacked sections.
    await expect(page.locator('[data-testid="ai-agents-nav-tabs"]')).toHaveCount(0);
    // The seeded site forwards fetch logs, so the page never pitches the setup.
    await expect(page.locator('[data-testid="ai-agents-enrich-callout"]')).toHaveCount(0);
});

test("ai agents page reports merged activity with per-row provenance", async ({ page }) => {
    await login(page, "/ai-agents");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

    // Pages crawled counts distinct paths across both feeds, so it can never be
    // zero for this site, and the merged KPI card is the only place it appears.
    await expect(page.locator(AI_AGENTS_TRAFFIC_KPIS).getByText("Pages crawled", { exact: true })).toBeVisible();
    // The fetch-depth strip states fetch-only scalars, headed by the fetch volume.
    await expect.poll(async () => readStatStripValue(page, AI_VISIBILITY_FETCH_DEPTH_STRIPS, "Total fetches")).toBeGreaterThan(0);
    await expect(page.locator(AI_VISIBILITY_FETCH_DEPTH_STRIPS).getByText("Unique paths")).toHaveCount(0);

    // GPTBot arrives through both feeds, so its row states where the count came from.
    const agentsPanel = await expectSeededMetricValue(page, "AI agents", SEEDED_AI_AGENT_RE);
    await expect(agentsPanel.locator(".metric-list__provenance").filter({ hasText: SEEDED_AI_PROVENANCE_RE }).first()).toBeVisible();
});

test("ai agents page filters by a crawled page from the merged breakdown", async ({ page }) => {
    await login(page, "/ai-agents");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

    await clickSeededMetricRow(page, "Pages crawled", SEEDED_AI_PATH);

    const chips = page.locator(AI_AGENTS_PAGE_CHIPS);
    await expect(chips.getByText(`Page: ${SEEDED_AI_PATH}`, { exact: true })).toBeVisible();
    // The page filter narrows both feeds, so the whole report re-renders under it.
    await expect(page.locator(AI_AGENTS_TRAFFIC_KPIS)).toBeVisible();
    await expect(page.locator('[data-testid="ai-agents-hero-chart"]')).toBeVisible();
    await expect(page.locator(AI_VISIBILITY_FETCH_DEPTH_STRIPS)).toBeVisible();

    await chips.getByRole("button", { name: "Clear all" }).click();
    await expect(chips.getByText("No active filter")).toBeVisible();
});

test("legacy ai routes redirect to the single ai agents page", async ({ page }) => {
    await login(page, "/ai-visibility");
    await expect(page).toHaveURL(/\/ai-agents$/);

    await page.goto("/ai-agents/crawlers", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/ai-agents$/);

    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

    await expect(page.getByRole("heading", { name: "Crawler fetch depth" })).toBeVisible();
});

test("ai agents page shows seeded agent traffic and filters by agent", async ({ page }) => {
    await login(page, "/ai-agents");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

    await expect(page.locator('[data-testid="ai-agent-traffic-kpis"]')).toBeVisible();
    await expect(page.locator('[data-testid="ai-agents-hero-chart"]')).toBeVisible();
    await expect(page.getByText("AI referral visits", { exact: true }).first()).toBeVisible();
    await expectSeededMetricValue(page, "AI sources", SEEDED_AI_SOURCE_RE);

    await clickSeededMetricRow(page, "AI agents", SEEDED_AI_AGENT_RE);

    const chips = page.locator(AI_AGENTS_PAGE_CHIPS);
    // The agent dimension exists on both feeds, so the chip carries no qualifier.
    await expect(chips.getByText("AI agent: GPTBot", { exact: true })).toBeVisible();
    // The agent filter reaches the fetch-only side too, so the fetch depth card
    // and its correlation tiles stay on screen with the filter applied.
    await expect(page.locator(AI_VISIBILITY_FETCH_DEPTH_STRIPS)).toBeVisible();
    await expect(page.locator(AI_VISIBILITY_CORRELATION_STRIP).getByText("Correlated paths", { exact: true })).toBeVisible();

    await chips.getByRole("button", { name: "Remove filter" }).first().click();
    await expect(chips.getByText("AI agent: GPTBot", { exact: true })).toHaveCount(0);
    await expect(chips.getByText("No active filter")).toBeVisible();
});

test("ai agents page qualifies only the tracked-visit-scoped AI source chip", async ({ page }) => {
    await login(page, "/ai-agents");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

    // Categories scope the whole merged report: no qualifier on the chip.
    await clickSeededMetricRow(page, "AI agent categories", SEEDED_AI_CATEGORY_RE);

    const chips = page.locator(AI_AGENTS_PAGE_CHIPS);
    await expect(chips.getByText(/^AI category: (Training crawlers|AI search crawlers|AI assistants)$/)).toBeVisible();

    // A crawler-category filter selects hits that carry no referrer at all, so
    // it has to come off before the referral dimension has rows to click.
    await chips.getByRole("button", { name: "Clear all" }).click();
    await expect(chips.getByText("No active filter")).toBeVisible();

    // Referrers only exist on the tracked side, so that one chip says so.
    await clickSeededMetricRow(page, "AI sources", SEEDED_AI_SOURCE_RE);
    await expect(chips.getByText(/^AI source: (ChatGPT|Claude|Perplexity|Gemini|DeepSeek) · tracked visits only$/)).toBeVisible();

    await chips.getByRole("button", { name: "Clear all" }).click();
    await expect(chips.getByText("No active filter")).toBeVisible();
});

test("ai chatbot page surfaces seeded audience geography and network data", async ({ page }) => {
    await login(page, "/ai-chatbots");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

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
    await selectRange(page, SEEDED_EVENT_RANGE);

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

test("team admin page shows seeded members and administration tabs", async ({ page }) => {
    await login(page, "/admin/team");
    await expect(page.getByRole("heading", { name: "Acme Analytics" })).toBeVisible();

    await page.getByRole("tab", { name: /^Members$/i }).click();
    await expect(page).toHaveURL(/\/admin\/team\/members$/);
    const membersPanel = page.locator("app-team-members");
    await expect(membersPanel.getByText("bob@devtools.co")).toBeVisible();
    await expect(membersPanel.getByText("diana@saaslaunch.com")).toBeVisible();

    await page.getByRole("tab", { name: /^API Clients$/i }).click();
    await expect(page.getByRole("heading", { name: "Team API clients" })).toBeVisible();

    await page.getByRole("tab", { name: /^Custom Domains$/i }).click();
    await expect(page.getByRole("heading", { name: "Custom domains" })).toBeVisible();
    const trackingDomain = `track-${Date.now()}.acme-analytics.io`;
    const customDomainsUrl = page.url();
    await page.locator("app-team-tracking-domains header").getByRole("button", { name: "Add domain" }).click();
    const addDomainDialog = page.getByRole("dialog", { name: "Add domain" });
    await addDomainDialog.getByPlaceholder("analytics.example.com").fill(trackingDomain);
    await expect(addDomainDialog.getByRole("button", { name: "Add domain" })).toBeEnabled();
    await addDomainDialog.getByRole("button", { name: "Add domain" }).click();
    // A successful create opens the DNS setup dialog with the records to copy.
    const setupDialog = page.getByRole("dialog", { name: "DNS setup" });
    await expect(setupDialog.getByText(`_hitkeep-tracking.${trackingDomain}`)).toBeVisible();
    await setupDialog.getByRole("button", { name: "Cancel" }).click();
    await expect(page).toHaveURL(customDomainsUrl);
    await expect(page.getByText(/Tracking domain added\./)).toBeVisible();
    await expect(page.locator("app-team-tracking-domains tbody").getByText(trackingDomain, { exact: true })).toBeVisible();

    await page.getByRole("tab", { name: /^Branding$/i }).click();
    await expect(page.getByText("Team name", { exact: true })).toBeVisible();
    await expect(page.getByText("Team logo", { exact: true })).toBeVisible();

    await page.getByRole("tab", { name: /^Danger Zone$/i }).click();
    await expect(page.getByRole("heading", { name: "Leave team" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Archive team" })).toBeVisible();
    await page.getByRole("button", { name: "Leave team" }).click();
    const leaveDialog = page.getByRole("alertdialog", { name: "Leave team" });
    await expect(leaveDialog).toBeVisible();
    await expect(leaveDialog.getByRole("button", { name: "Leave team" })).toBeDisabled();
    await leaveDialog.getByLabel("Type Acme Analytics to confirm").fill("Acme Analytics");
    await expect(leaveDialog.getByRole("button", { name: "Leave team" })).toBeEnabled();
    await leaveDialog.getByRole("button", { name: "Cancel" }).click();

    await page.getByRole("tab", { name: /^Activity$/i }).click();
    await expect(page.getByRole("search", { name: "Audit filters" })).toBeVisible();
});

test("public share links render seeded analytics without login", async ({ page }) => {
    await page.goto(`/share/${E2E_SHARE_TOKEN}/dashboard`, { waitUntil: "domcontentloaded" });
    await selectRange(page, SEEDED_EVENT_RANGE);

    await expect(page).toHaveURL(new RegExp(`/share/${E2E_SHARE_TOKEN}/dashboard`));
    await expect(page.getByText("Latest Hits")).toBeVisible();
    await expect(page.getByText("Top Sources")).toBeVisible();
    await expect(page.getByText("Cities")).toBeVisible();
    await expect(page.getByText("Providers")).toBeVisible();
    await expect(page.getByText("ASNs")).toBeVisible();
    await expectSeededGeoNetworkMetrics(page);
    await expect(page.getByRole("button", { name: /share dashboard/i })).toHaveCount(0);
});
