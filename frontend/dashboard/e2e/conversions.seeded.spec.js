const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

const PRIMARY_SEEDED_SITE_DOMAIN = "acme-analytics.io";

test("dashboard goal and funnel rows replace the active conversion cohort", async ({ page }) => {
    await login(page, "/dashboard");
    await selectSeededSite(page);
    await selectRange(page, "30d");

    let filteredGoalStatsCount = 0;
    let filteredGoalHitsCount = 0;
    page.on("request", (request) => {
        const url = new URL(request.url());
        if (url.pathname.endsWith("/stats") && url.searchParams.has("goal_id")) filteredGoalStatsCount += 1;
        if (url.pathname.endsWith("/hits") && url.searchParams.has("goal_id")) filteredGoalHitsCount += 1;
    });
    await page.getByRole("button").filter({ hasText: "Newsletter Signup" }).click();
    await expect.poll(() => filteredGoalStatsCount).toBe(1);
    await expect.poll(() => filteredGoalHitsCount).toBe(1);
    await page.waitForTimeout(750);
    expect(filteredGoalStatsCount).toBe(1);
    expect(filteredGoalHitsCount).toBe(1);
    await expect(page).toHaveURL(/goal=[0-9a-f-]+/);
    await expect(page.getByText("Goal: Newsletter Signup · tracked visits only", { exact: true })).toBeVisible();

    await page.getByRole("tab", { name: /Funnels/ }).click();
    let filteredFunnelStatsCount = 0;
    let filteredFunnelHitsCount = 0;
    page.on("request", (request) => {
        const url = new URL(request.url());
        if (url.pathname.endsWith("/stats") && url.searchParams.has("funnel_id") && !url.searchParams.has("goal_id")) filteredFunnelStatsCount += 1;
        if (url.pathname.endsWith("/hits") && url.searchParams.has("funnel_id") && !url.searchParams.has("goal_id")) filteredFunnelHitsCount += 1;
    });
    await page.getByRole("button").filter({ hasText: "Acquisition Funnel" }).click();
    await expect.poll(() => filteredFunnelStatsCount).toBe(1);
    await expect.poll(() => filteredFunnelHitsCount).toBe(1);
    await page.waitForTimeout(750);
    expect(filteredFunnelStatsCount).toBe(1);
    expect(filteredFunnelHitsCount).toBe(1);
    await expect(page).toHaveURL(/funnel=[0-9a-f-]+/);
    await expect(page).not.toHaveURL(/goal=/);
    await expect(page.getByText("Funnel: Acquisition Funnel · tracked visits only", { exact: true })).toBeVisible();

    await page.getByRole("button", { name: "Remove filter" }).click();
    await expect(page).not.toHaveURL(/funnel=/);
    await expect(page.getByText("No active filter", { exact: true })).toBeVisible();
});

test("conversion workspaces present report scope as a shared subject card", async ({ page }) => {
    const allGoalTraffic = page.waitForRequest((request) => {
        const url = new URL(request.url());
        return url.pathname.endsWith("/hits") && url.searchParams.getAll("goal_id").length > 1;
    });
    await login(page, "/goals");
    await selectSeededSite(page);
    await allGoalTraffic;
    await page.getByRole("row").filter({ hasText: "Newsletter Signup" }).click();
    const goalSubject = page.locator("app-conversion-subject-card");
    const goalTraffic = page.locator("app-traffic-records-card");
    await expect(goalSubject).toContainText("Reporting subject");
    await expect(goalSubject).toContainText("Newsletter Signup");
    await expect(goalTraffic).toContainText("Page views from sessions that completed this goal.");
    await expect(page.getByRole("button", { name: "View matching traffic" })).toHaveCount(0);

    const allFunnelTraffic = page.waitForRequest((request) => {
        const url = new URL(request.url());
        return url.pathname.endsWith("/hits") && url.searchParams.getAll("funnel_id").length > 1;
    });
    await page.goto("/funnels");
    await allFunnelTraffic;
    await page.getByRole("row").filter({ hasText: "Acquisition Funnel" }).click();
    const funnelSubject = page.locator("app-conversion-subject-card");
    const funnelTraffic = page.locator("app-traffic-records-card");
    await expect(funnelSubject).toContainText("Reporting subject");
    await expect(funnelSubject).toContainText("Acquisition Funnel");
    await expect(funnelSubject).toContainText("4 steps");
    await expect(funnelTraffic).toContainText("Page views from sessions that entered this funnel.");
});

test("goal workspace supports create, update, selection, and delete", async ({ page }) => {
    const suffix = Date.now();
    const name = `Lifecycle goal ${suffix}`;
    const updatedName = `${name} updated`;

    await login(page, "/goals");
    await selectSeededSite(page);

    await page.getByRole("button", { name: "Create goal" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("Add new goal", { exact: true })).toBeVisible();
    await dialog.getByLabel("Name").fill(name);
    await dialog.getByRole("button", { name: "Event", exact: true }).click();
    await dialog.getByLabel("Event", { exact: true }).fill(`lifecycle_goal_${suffix}`);
    await dialog.getByRole("button", { name: "Create goal" }).click();

    let row = page.getByRole("row").filter({ hasText: name });
    await expect(row).toBeVisible();
    await runRowAction(page, row, "Edit");
    await expect(dialog.getByText("Edit goal", { exact: true })).toBeVisible();
    await dialog.getByLabel("Name").fill(updatedName);
    await dialog.getByRole("button", { name: "Save changes" }).click();

    row = page.getByRole("row").filter({ hasText: updatedName });
    await expect(row).toBeVisible();
    await runRowAction(page, row, "Delete");
    await page.getByRole("alertdialog").getByRole("button", { name: "Delete" }).click();
    await expect(row).toHaveCount(0);
});

test("funnel workspace supports accessible reordering and the full CRUD lifecycle", async ({ page }) => {
    const suffix = Date.now();
    const name = `Lifecycle funnel ${suffix}`;
    const updatedName = `${name} updated`;

    await login(page, "/funnels");
    await selectSeededSite(page);

    await page.getByRole("button", { name: "Create funnel" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog.getByText("Create new funnel", { exact: true })).toBeVisible();
    await dialog.getByLabel("Name").fill(name);
    await dialog.getByLabel("Value for step 1").fill("/pricing");
    await dialog.getByLabel("Value for step 2").fill("/signup");
    await dialog.getByRole("button", { name: "Move step down" }).first().click();
    await expect(dialog.getByLabel("Value for step 1")).toHaveValue("/signup");
    await dialog.getByRole("button", { name: "Create funnel" }).click();

    let row = page.getByRole("row").filter({ hasText: name });
    await expect(row).toBeVisible();
    await runRowAction(page, row, "Edit");
    await dialog.getByLabel("Name").fill(updatedName);
    await dialog.getByRole("button", { name: "Save changes" }).click();

    row = page.getByRole("row").filter({ hasText: updatedName });
    await expect(row).toBeVisible();
    await runRowAction(page, row, "Delete");
    await page.getByRole("alertdialog").getByRole("button", { name: "Delete" }).click();
    await expect(row).toHaveCount(0);
});

async function selectSeededSite(page, domain = PRIMARY_SEEDED_SITE_DOMAIN) {
    const combobox = page.locator('[role="combobox"]:visible').first();
    await expect(combobox).toBeVisible();
    if (((await combobox.textContent()) || "").includes(domain)) return;
    await page.locator('[aria-label="Select a site to view stats"]:visible').first().click();
    const option = page.locator('[role="option"]:visible').filter({ hasText: domain }).first();
    await expect(option).toBeVisible();
    await option.click();
    await expect(combobox).toContainText(domain);
}

async function selectRange(page, label) {
    const button = page.getByRole("button", { name: label, exact: true }).first();
    await expect(button).toBeVisible();
    if ((await button.getAttribute("aria-pressed")) !== "true") await button.click();
    await expect(button).toHaveAttribute("aria-pressed", "true");
}

async function runRowAction(page, row, name) {
    await row.getByRole("button", { name: "More actions" }).click();
    const menu = page.locator(".table-row-actions-menu");
    await expect(menu).toBeVisible();
    await menu.getByText(name, { exact: true }).click();
}
