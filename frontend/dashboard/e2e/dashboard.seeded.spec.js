const { test, expect } = require("playwright/test");
const { E2E_SHARE_TOKEN, login } = require("./support/auth");

const CHART_SETTLE_MS = 2500;
const TABLE_SETTLE_MS = 1000;

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

test("dashboard renders seeded data and product controls", async ({ page }) => {
    await login(page, "/dashboard");
    await expect(page).toHaveURL(/\/dashboard(?:\?.*)?$/);
    await expect(page.getByText("Latest Hits")).toBeVisible();
    await expect(page.getByText("Top Sources")).toBeVisible();
    await expect(page.locator('[data-testid="team-switcher-trigger"]:visible').first()).toBeVisible();
    await expect(page.locator(".layout-topbar__account-cluster").getByText("Acme Analytics").first()).toBeVisible();

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

test("ecommerce page surfaces seeded orders and products", async ({ page }) => {
    await login(page, "/ecommerce");
    await page.waitForTimeout(CHART_SETTLE_MS);

    await expect(page.getByText("Revenue over time")).toBeVisible();
    await expect(page.getByText("Top products")).toBeVisible();
    await expect(page.getByText("Revenue sources")).toBeVisible();
    await expect(page.getByText("Pro Plan")).toBeVisible();
});

test("ai visibility page shows correlation insights", async ({ page }) => {
    await login(page, "/ai-visibility");
    await page.waitForTimeout(CHART_SETTLE_MS);

    await expect(page.getByRole("heading", { name: "Fetch volume over time" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Fetch-to-visit correlation" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Citation yield" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Opportunity pages" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Failure hotspots" })).toBeVisible();
    await expect(page.getByText("GPTBot").first()).toBeVisible();
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
    await expect(page.getByText("Filter activity")).toBeVisible();
});

test("public share links render seeded analytics without login", async ({ page }) => {
    await page.goto(`/share/${E2E_SHARE_TOKEN}/dashboard`, { waitUntil: "domcontentloaded" });
    await page.waitForTimeout(CHART_SETTLE_MS);

    await expect(page).toHaveURL(new RegExp(`/share/${E2E_SHARE_TOKEN}/dashboard`));
    await expect(page.getByText("Latest Hits")).toBeVisible();
    await expect(page.getByText("Top Sources")).toBeVisible();
    await expect(page.getByRole("button", { name: /share dashboard/i })).toHaveCount(0);
});
