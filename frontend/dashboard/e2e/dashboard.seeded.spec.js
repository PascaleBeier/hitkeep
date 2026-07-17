const { test, expect } = require("playwright/test");
const { E2E_SHARE_TOKEN, login } = require("./support/auth");

const PRIMARY_SEEDED_SITE_DOMAIN = "acme-analytics.io";
const SEEDED_EVENT_NAME = "newsletter_signup";
const SEEDED_EVENT_RANGE = "30d";
const SEEDED_CITY_RE = /Mountain View|New York|Seattle|Berlin|Munich|London|Paris|Amsterdam/;
const SEEDED_PROVIDER_RE = /Google LLC|Verizon Business|Comcast Cable|Deutsche Telekom AG|Vodafone GmbH|BT|Orange|KPN/;
const SEEDED_ASN_RE = /AS15169|AS701|AS7922|AS3320|AS3209|AS2856|AS3215|AS1136/;

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
    await row.click();
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
    await expect(page.getByText("Automatic event tracking")).toBeVisible();
    await expect(page.getByText("Track outbound clicks")).toBeVisible();
    await expect(page.getByText("Track file downloads")).toBeVisible();
    await expect(page.getByText("Track form submissions")).toBeVisible();

    await page.reload();
    await expect(page).toHaveURL(/\/sites\/[^/]+\/settings\/tracking$/);
    await expect(page.getByText("Automatic event tracking")).toBeVisible();

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

test("ai visibility page shows correlation insights", async ({ page }) => {
    await login(page, "/ai-visibility");
    await selectSeededSite(page);
    await selectRange(page, SEEDED_EVENT_RANGE);

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
