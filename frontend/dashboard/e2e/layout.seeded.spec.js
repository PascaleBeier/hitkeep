const { test, expect } = require("playwright/test");
const { login, E2E_SHARE_TOKEN } = require("./support/auth");
const { enableAskAIForBootstrap } = require("./support/ask-ai");

async function screenshot(page, info, name) {
    const path = info.outputPath(`${name}.png`);
    await expect(page.locator("main p-skeleton")).toHaveCount(0);
    await page.screenshot({ path, animations: "disabled" });
    await info.attach(name, { path, contentType: "image/png" });
}

async function overrideBootstrap(page, update) {
    await page.route("**/api/user/bootstrap", async (route) => {
        const response = await route.fetch();
        const data = await response.json();
        update(data);
        const headers = response.headers();
        delete headers["content-length"];
        await route.fulfill({ status: response.status(), headers, body: JSON.stringify(data) });
    });
}

const siteControl = (page) => page.locator('app-site-selector [role="combobox"]');
const sidebar = (page) => page.locator("app-layout-sidebar > aside");
const sidebarWidth = async (page) => (await sidebar(page).boundingBox())?.width ?? 0;

async function selectSeededSite(page) {
    await siteControl(page).click();
    await page.getByRole("option", { name: /acme-analytics\.io/ }).click();
    await expect(siteControl(page)).toContainText("acme-analytics.io");
}

async function expectNoHorizontalOverflow(page) {
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
}

test("Assistants uses the shared route heading and keeps connection controls reachable", async ({ page }) => {
    await overrideBootstrap(page, (data) => {
        data.assistants = { enabled: true, endpoint: "https://analytics.example.com/mcp" };
    });
    await login(page, "/integration/assistants");
    await expect(page.locator("main h1")).toHaveText("Assistants");
    await expect(page.locator("main input[readonly]")).toHaveValue("https://analytics.example.com/mcp");
    await expect(page.locator('main a[href="/settings/connected-applications"]')).toBeVisible();
    await expect(page.locator('main a[href="/integration/oauth-clients"]')).toBeVisible();
});

test("Focus preserves the active page while resizing, collapsing and restoring navigation", async ({ page }, info) => {
    await page.setViewportSize({ width: 1440, height: 960 });
    await page.addInitScript(() => localStorage.setItem("hk_theme", "light"));
    await enableAskAIForBootstrap(page);
    await login(page, "/dashboard");
    await selectSeededSite(page);
    await expect(siteControl(page)).toBeVisible();
    await expect(page.getByRole("button", { name: "Ask AI about this site" })).toBeEnabled();
    await expect(page.locator("main h1")).toHaveText("Dashboard");
    await expect(page.locator("main canvas").first()).toBeVisible();
    await page.getByRole("button", { name: "7d", exact: true }).first().click();
    await expect(page.getByRole("button", { name: "7d", exact: true }).first()).toHaveAttribute("aria-pressed", "true");
    const route = page.url();
    await page.locator("main router-outlet").evaluate((el) => el.setAttribute("data-focus-proof", "mounted"));
    const initial = await sidebarWidth(page);
    expect(initial).toBeGreaterThanOrEqual(220);
    expect(initial).toBeLessThanOrEqual(320);
    const handle = page.getByRole("separator", { name: "Resize sidebar" });
    const box = await handle.boundingBox();
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await page.mouse.move(box.x + 50, box.y + box.height / 2, { steps: 8 });
    await page.mouse.up();
    await expect.poll(() => sidebarWidth(page)).toBeGreaterThan(initial + 35);
    const pointerWidth = await sidebarWidth(page);
    await handle.press("ArrowRight", { delay: 120 });
    await expect.poll(() => sidebarWidth(page)).toBeGreaterThan(pointerWidth + 1);
    const width = await sidebarWidth(page);
    expect(width).toBeLessThanOrEqual(320);
    await page.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
    await expect(sidebar(page)).not.toBeVisible();
    await expect(siteControl(page)).toBeVisible();
    await expect(page.locator("app-team-switcher")).toHaveCount(1);
    await expect(page.getByRole("button", { name: "Ask AI about this site" })).toBeVisible();
    await expect(page.locator("main router-outlet")).toHaveAttribute("data-focus-proof", "mounted");
    expect(page.url()).toBe(route);
    await expect(page.getByRole("button", { name: "7d", exact: true }).first()).toHaveAttribute("aria-pressed", "true");
    await screenshot(page, info, "focus-desktop-collapsed");
    await page.getByRole("button", { name: "Expand sidebar", exact: true }).click();
    await expect.poll(() => sidebarWidth(page)).toBeCloseTo(width, 0);
    await sidebar(page).getByRole("button", { name: "Integration", exact: true }).click();
    await sidebar(page).getByRole("button", { name: "Account", exact: true }).click();
    await expect(sidebar(page).getByRole("button", { name: "Integration", exact: true })).toHaveAttribute("aria-expanded", "true");
    await expect(sidebar(page).getByRole("button", { name: "Account", exact: true })).toHaveAttribute("aria-expanded", "true");
    await screenshot(page, info, "focus-desktop-light");
    await page.getByRole("button", { name: "Open user menu" }).click();
    await page.getByRole("menuitem", { name: "Switch to dark mode" }).click();
    await expect(page.locator("html")).toHaveClass(/p-dark/);
    await screenshot(page, info, "focus-desktop-dark");
    await page.getByRole("button", { name: "Ask AI about this site" }).click();
    await expect(page.getByRole("heading", { name: "Ask AI", exact: true })).toBeVisible();
    await screenshot(page, info, "focus-ask-ai");
    await page.keyboard.press("Escape");
    await expect(page.locator(".ask-ai-drawer")).not.toBeVisible();
    await expect(page.locator(".p-drawer-mask")).toHaveCount(0);
    await page.reload();
    await expect.poll(() => sidebarWidth(page)).toBeCloseTo(width, 0);
    await expect(page.locator("app-team-switcher")).toHaveCount(1);
    await page.keyboard.press("ControlOrMeta+k");
    const search = page.getByRole("combobox", { name: "Search navigation", exact: true });
    await search.fill("UTM");
    await expect(page.getByRole("option", { name: "UTM", exact: true })).toBeVisible();
    await screenshot(page, info, "focus-navigation-search");
    await search.press("Enter");
    await expect(page).toHaveURL(/\/utm$/);
    await expect(page.getByRole("tab", { name: "Analytics", exact: true })).toHaveAttribute("aria-selected", "true");
});

test("Focus keeps context and Ask AI usable at mobile widths, breakpoints and 200% zoom", async ({ page }, info) => {
    await enableAskAIForBootstrap(page);
    await page.setViewportSize({ width: 360, height: 800 });
    await login(page, "/dashboard");
    await selectSeededSite(page);
    for (const width of [320, 360, 768, 769]) {
        await page.setViewportSize({ width, height: 800 });
        await expect(siteControl(page)).toBeVisible();
        await expect(page.locator("app-team-switcher")).toHaveCount(1);
        await expect(page.getByRole("button", { name: "Ask AI about this site" })).toBeVisible();
        await expectNoHorizontalOverflow(page);
        if (width <= 768) await expect(sidebar(page)).not.toBeVisible();
        else await expect(sidebar(page)).toBeVisible();
    }
    await page.setViewportSize({ width: 360, height: 800 });
    const trigger = page.getByRole("button", { name: "Open navigation", exact: true });
    await trigger.click();
    const drawer = page.locator(".focus-navigation-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer).toHaveAttribute("role", "dialog");
    await expect.poll(() => drawer.evaluate((el) => el.contains(document.activeElement))).toBe(true);
    for (let i = 0; i < 18; i++) {
        await page.keyboard.press("Tab");
        await expect.poll(() => drawer.evaluate((el) => el.contains(document.activeElement))).toBe(true);
    }
    await page.keyboard.press("Escape");
    await expect(drawer).not.toBeVisible();
    await expect(page.locator(".p-drawer-mask")).toHaveCount(0);
    await expect(trigger).toBeFocused();
    await trigger.click();
    await page.setViewportSize({ width: 769, height: 800 });
    await expect(drawer).not.toBeVisible();
    await expect(page.locator(".p-drawer-mask")).toHaveCount(0);
    await page.setViewportSize({ width: 360, height: 800 });
    await trigger.click();
    await drawer.getByRole("link", { name: "Events", exact: true }).click();
    await expect(page).toHaveURL(/\/events(?:\?.*)?$/);
    await expect(drawer).not.toBeVisible();
    await expect(page.locator("main h1")).toHaveText("Events");
    await screenshot(page, info, "focus-mobile");
    await page.setViewportSize({ width: 1280, height: 960 });
    await page.evaluate(() => (document.documentElement.style.zoom = "2"));
    await expectNoHorizontalOverflow(page);
    await expect(siteControl(page)).toBeVisible();
    await expect(page.getByRole("button", { name: "Ask AI about this site" })).toBeVisible();
    await screenshot(page, info, "focus-zoom-200");
});

test("Focus preserves read-only shared navigation and hides privileged context actions", async ({ page }) => {
    await page.goto(`/share/${E2E_SHARE_TOKEN}/dashboard`);
    await expect(page.locator("main h1")).toHaveText("Dashboard");
    await expect(page.locator("app-team-switcher")).toHaveCount(0);
    await expect(page.locator("app-site-selector")).toHaveCount(0);
    await expect(page.locator(".ask-ai-trigger")).toHaveCount(0);
    await page.getByRole("button", { name: "Search navigation", exact: true }).click();
    const search = page.getByRole("combobox", { name: "Search navigation", exact: true });
    await search.fill("System settings");
    await expect(page.getByText("No pages found", { exact: true })).toBeVisible();
    await search.fill("UTM");
    await search.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/share/${E2E_SHARE_TOKEN}/utm$`));
    await expect(page.getByRole("tab", { name: /Generator|Builder/ })).toHaveCount(0);
});

test("Focus keeps long German team and site names beside a visible Ask AI action", async ({ page }, info) => {
    const de = require("../public/i18n/de.json");
    const teamName = "Internationale Unternehmensanalysen und Produktentwicklung";
    const domain = "produktentwicklung.internationale-unternehmensanalysen.example.test";
    await overrideBootstrap(page, (data) => {
        data.preferences.default_locale = "de";
        data.teams.teams.forEach((team) => (team.name = teamName));
        data.sites.forEach((site) => (site.domain = domain));
        data.status.ask_ai = { enabled: true, available: false, status: "not_configured", budget_exhausted: false };
    });
    await page.setViewportSize({ width: 320, height: 800 });
    await login(page, "/overview");
    await expect(page.locator("html")).toHaveAttribute("lang", "de");
    await expect(page.locator('app-team-switcher [role="combobox"]')).toContainText(teamName);
    await expect(siteControl(page)).toContainText(domain);
    for (const selector of ["app-team-switcher .p-select-label .truncate", "app-site-selector .site-select-option__domain"]) {
        await expect.poll(() => page.locator(selector).evaluate((el) => el.getBoundingClientRect().width)).toBeGreaterThan(64);
    }
    await expect(page.locator(".ask-ai-trigger")).toHaveText(de.askAi.title);
    await expect(page.locator(".ask-ai-trigger")).toBeEnabled();
    await expectNoHorizontalOverflow(page);
    await screenshot(page, info, "focus-mobile-german-long-context");
});

test("Focus offers contextual Add site when the team has no sites", async ({ page }) => {
    await overrideBootstrap(page, (data) => {
        data.sites = [];
    });
    await login(page, "/overview");
    await siteControl(page).click();
    await page.getByRole("button", { name: "Add site", exact: true }).last().click();
    await expect(page.getByRole("dialog", { name: "Add site", exact: true })).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog", { name: "Add site", exact: true })).not.toBeVisible();
});

test("Focus tolerates corrupt or unavailable navigation preferences", async ({ page }) => {
    await page.addInitScript(() => {
        localStorage.setItem("hk_sidebar_layout", "{invalid");
        localStorage.setItem("hk_sidebar_sections", "[]");
        const read = Storage.prototype.getItem;
        const write = Storage.prototype.setItem;
        Storage.prototype.getItem = function (key) {
            if (key === "hk_sidebar_layout" || key === "hk_sidebar_sections") throw new DOMException("Blocked", "SecurityError");
            return read.call(this, key);
        };
        Storage.prototype.setItem = function (key, value) {
            if (key === "hk_sidebar_layout" || key === "hk_sidebar_sections") throw new DOMException("Blocked", "SecurityError");
            return write.call(this, key, value);
        };
    });
    await login(page, "/dashboard");
    await expect(sidebar(page)).toBeVisible();
    await page.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
    await page.getByRole("button", { name: "Expand sidebar", exact: true }).click();
    await expect.poll(() => sidebarWidth(page)).toBeGreaterThanOrEqual(220);
    await expect.poll(() => sidebarWidth(page)).toBeLessThanOrEqual(320);
});
