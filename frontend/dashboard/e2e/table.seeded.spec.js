const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

// The shared <app-table> behaviors, exercised once on the admin users table.
test.describe("shared table", () => {
    test.beforeEach(async ({ page }) => {
        await login(page, "/admin/system");
        await page.evaluate(() =>
            Object.keys(localStorage)
                .filter((key) => key.startsWith("hitkeep.table."))
                .forEach((key) => localStorage.removeItem(key))
        );
        await page.reload({ waitUntil: "domcontentloaded" });
        await expect(usersTable(page).locator("tr.app-table__row").first()).toBeVisible();
    });

    test("filters by search, shows a chip, and clears it", async ({ page }) => {
        const table = usersTable(page);
        const email = (await table.locator("tr.app-table__row td").first().innerText()).trim();
        const term = email.split("@")[0];

        await table.getByTestId("table-search").fill(term);
        await expect(table.locator("app-filter-chip-row")).toContainText(`Search: ${term}`);
        await expect(table.locator("tr.app-table__row").first()).toContainText(term);

        await table.getByRole("button", { name: "Clear all" }).click();
        await expect(table.locator("app-filter-chip-row")).toHaveCount(0);
        await expect(table.getByTestId("table-search")).toHaveValue("");
    });

    test("groups rows and keeps the grouping after reload", async ({ page }) => {
        const table = usersTable(page);
        await table.getByTestId("table-group-by").click();
        await page.getByRole("option", { name: "Role" }).click();
        await expect(table.locator("tr.app-table__group").first()).toBeVisible();

        await page.reload({ waitUntil: "domcontentloaded" });
        await expect(usersTable(page).locator("tr.app-table__group").first()).toBeVisible();
    });

    test("persists resized column widths across reloads", async ({ page }) => {
        const header = usersTable(page).locator("thead th").first();
        const before = (await header.boundingBox()).width;
        const resizer = header.locator(".p-datatable-column-resizer");
        const box = await resizer.boundingBox();
        await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
        await page.mouse.down();
        await page.mouse.move(box.x + 80, box.y + box.height / 2, { steps: 5 });
        await page.mouse.up();
        const resized = (await header.boundingBox()).width;
        expect(resized).toBeGreaterThan(before + 20);

        await page.reload({ waitUntil: "domcontentloaded" });
        const restored = usersTable(page).locator("thead th").first();
        await expect(usersTable(page).locator("tr.app-table__row").first()).toBeVisible();
        expect(Math.abs((await restored.boundingBox()).width - resized)).toBeLessThan(4);
    });

    test("exports the current view as CSV", async ({ page }) => {
        const download = page.waitForEvent("download");
        await usersTable(page).getByTestId("table-export").click();
        expect((await download).suggestedFilename()).toBe("users.csv");
    });

    test("keeps the actions column in view on a phone", async ({ page }) => {
        await page.setViewportSize({ width: 390, height: 844 });
        const actions = usersTable(page).locator("td.app-table__actions").first();
        await expect(actions).toBeVisible();
        const box = await actions.boundingBox();
        expect(box.x + box.width).toBeLessThanOrEqual(390);
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(390);
    });
});

function usersTable(page) {
    return page.getByTestId("admin-users-table");
}
