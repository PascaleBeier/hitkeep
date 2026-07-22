const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");
const { startSmtpCapture } = require("./support/smtp");

test("reporting creates a browser-timezone schedule and supports its delivery controls", async ({ page }) => {
    const smtp = await startSmtpCapture();
    try {
        const name = `E2E morning summary ${Date.now()}`;
        await login(page, "/settings/reports");
        await expect(page.getByText("Create reliable, scheduled analytics reports for yourself or your team.")).toBeVisible();

        await page.getByTestId("new-report").click();
        const editor = page.getByRole("dialog", { name: "Create report" });
        await expect(editor).toBeVisible();
        await editor.getByTestId("report-name").fill(name);

        const timezone = await page.evaluate(() => Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC");
        await expect(editor.getByTestId("report-timezone")).toContainText(timezone);
        await expect(editor.getByTestId("report-delivery-time")).toContainText("08:00");
        await editor.getByTestId("report-delivery-time").click();
        await page.getByRole("option", { name: "08:15", exact: true }).click();

        await editor.getByRole("button", { name: "Generate preview" }).click();
        await expect(editor.locator(".preview-card strong")).toContainText("Daily Report for");
        await expect(editor.locator(".preview-card")).toContainText(`08:15 · ${timezone}`);

        await editor.getByTestId("report-status").click();
        await page.getByRole("option", { name: "Active", exact: true }).click();
        await editor.getByRole("button", { name: /Save$/ }).click();
        const row = page.locator("tr[data-report-id]", { hasText: name });
        await expect(row).toBeVisible();
        await expect(row).toContainText("08:15");
        await expect(row).toContainText("Active");

        await runReportRowAction(page, row, "Test send");
        await expect(page.getByText("Test message accepted by the mail server.")).toBeVisible();
        await runReportRowAction(page, row, "Pause");
        await expect(row).toContainText("Paused");
        await runReportRowAction(page, row, "Resume");
        await expect(row).toContainText("Active");

        await runReportRowAction(page, row, "History");
        await expect(page.getByRole("dialog", { name: "Delivery history" }).getByText("No scheduled runs yet.")).toBeVisible();
        await page.keyboard.press("Escape");

        const reportID = await row.getAttribute("data-report-id");
        await page.goto(`/settings/reports?report=${reportID}`);
        const linkedRow = page.locator(`tr[data-report-id="${reportID}"]`);
        await expect(linkedRow).toHaveClass(/report-row--focused/);
        await expect(page.getByRole("dialog", { name: "Edit report" })).toBeVisible();
        await page.keyboard.press("Escape");

        await runReportRowAction(page, linkedRow, "Delete");
        await page.getByRole("alertdialog").getByRole("button", { name: "Delete" }).click();
        await expect(linkedRow).toHaveCount(0);
    } finally {
        await smtp.close();
    }
});

test("reporting warns when SMTP is unavailable and allows draft-only creation", async ({ page }) => {
    await page.route("**/api/user/bootstrap", async (route) => {
        const response = await route.fetch();
        const bootstrap = await response.json();
        bootstrap.status.mail_delivery = { available: false, status: "unavailable" };
        const headers = response.headers();
        delete headers["content-length"];
        await route.fulfill({ status: response.status(), headers: { ...headers, "content-type": "application/json" }, body: JSON.stringify(bootstrap) });
    });

    await login(page, "/settings/reports");
    await expect(page.getByText("SMTP is unavailable. You can create and edit drafts, but reports cannot be activated, tested, or retried.")).toBeVisible();
    await page.getByTestId("new-report").click();
    const editor = page.getByRole("dialog", { name: "Create report" });
    await editor.getByTestId("report-name").fill("SMTP unavailable draft");
    const save = editor.getByRole("button", { name: /Save$/ });
    await expect(save).toBeEnabled();
    await editor.getByTestId("report-status").click();
    await page.getByRole("option", { name: "Active", exact: true }).click();
    await expect(save).toBeDisabled();
});

test("team reporting validates and tracks a pending external recipient", async ({ page }) => {
    const smtp = await startSmtpCapture();
    try {
        const suffix = Date.now();
        const name = `E2E client report ${suffix}`;
        const externalEmail = `client-${suffix}@example.test`;
        await login(page, "/settings/reports");
        await page.getByTestId("new-report").click();
        const editor = page.getByRole("dialog", { name: "Create report" });
        await editor.getByTestId("report-name").fill(name);

        await editor.getByTestId("report-scope").click();
        await page.getByRole("option", { name: "Team", exact: true }).click();
        await editor.getByTestId("report-external-recipient").fill("not-an-email");
        await editor.getByRole("button", { name: "Add email" }).click();
        await expect(editor.getByText("Enter a valid email address.")).toBeVisible();

        await editor.getByTestId("report-external-recipient").fill(` ${externalEmail.toUpperCase()} `);
        await editor.getByTestId("report-external-recipient").press("Enter");
        await expect(editor.locator(".external-recipient-chip", { hasText: externalEmail })).toBeVisible();

        await editor.getByRole("button", { name: "Generate preview" }).click();
        await expect(editor.locator(".preview-card")).toContainText(/1 external recipient.*pending confirmation/);
        await editor.getByTestId("report-status").click();
        await page.getByRole("option", { name: "Active", exact: true }).click();
        await editor.getByRole("button", { name: /Save$/ }).click();

        const row = page.locator("tr[data-report-id]", { hasText: name });
        await expect(row).toBeVisible();
        await expect(row).toContainText(externalEmail);
        await expect(row).toContainText("Pending confirmation");

        await runReportRowAction(page, row, `Remove · ${externalEmail}`);
        await page.getByRole("alertdialog").getByRole("button", { name: "Remove" }).click();
        await expect(row).not.toContainText(externalEmail);

        await runReportRowAction(page, row, "Delete");
        await page.getByRole("alertdialog").getByRole("button", { name: "Delete" }).click();
        await expect(row).toHaveCount(0);
    } finally {
        await smtp.close();
    }
});

async function runReportRowAction(page, row, name) {
    await row.getByRole("button", { name: "More actions" }).click();
    const menu = page.locator(".table-row-actions-menu");
    await expect(menu).toBeVisible();
    await menu.getByText(name, { exact: true }).click();
}
