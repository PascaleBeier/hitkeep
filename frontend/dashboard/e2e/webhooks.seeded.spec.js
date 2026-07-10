const crypto = require("node:crypto");
const http = require("node:http");

const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

test("webhook admin can create, test, inspect, rotate, and delete an endpoint", async ({ page }) => {
    const received = [];
    const receiver = http.createServer((request, response) => {
        const chunks = [];
        request.on("data", (chunk) => chunks.push(chunk));
        request.on("end", () => {
            received.push({ headers: request.headers, body: Buffer.concat(chunks) });
            response.writeHead(204).end();
        });
    });
    await new Promise((resolve) => receiver.listen(0, "127.0.0.1", resolve));

    try {
        const address = receiver.address();
        await login(page, "/integration/webhooks");
        await expect(page.getByText("Send signed operational events to your systems without delaying product actions.")).toBeVisible();

        await page.getByTestId("create-webhook").click();
        await page.getByLabel("Name").fill("E2E receiver");
        await page.getByLabel("Destination URL").fill(`http://127.0.0.1:${address.port}/hitkeep`);
        await page.locator(".event-options label", { hasText: "goal.created" }).getByRole("checkbox").check();
        await page.getByRole("button", { name: "Save webhook" }).click();

        const secretCode = page.locator(".secret-reveal code");
        await expect(secretCode).toHaveText(/^whsec_/);
        const originalSecret = (await secretCode.textContent()).trim();
        const row = page.locator("article", { hasText: "E2E receiver" });
        await row.getByRole("button", { name: "Send test" }).click();
        await expect.poll(() => received.length).toBe(1);

        const delivery = received[0];
        const timestamp = delivery.headers["x-hitkeep-timestamp"];
        const signature = delivery.headers["x-hitkeep-signature"];
        const expectedSignature = crypto
            .createHmac("sha256", originalSecret)
            .update(`${timestamp}.${delivery.body.toString("utf8")}`)
            .digest("hex");
        expect(signature).toBe(`v1=${expectedSignature}`);
        expect(delivery.headers["x-hitkeep-event-id"]).toMatch(/^[0-9a-f-]{36}$/);
        expect(delivery.headers["x-hitkeep-delivery-id"]).toMatch(/^[0-9a-f-]{36}$/);

        await expect
            .poll(async () => {
                const response = await page.request.get(`/api/sites/${await activeSiteID(page)}/webhooks/${await webhookID(page)}/deliveries`);
                const deliveries = await response.json();
                return deliveries[0]?.status;
            })
            .toBe("succeeded");
        await row.getByRole("button", { name: "Deliveries" }).click();
        await expect(page.getByText("Succeeded")).toBeVisible();

        await row.getByRole("button", { name: "Rotate secret" }).click();
        await page.getByRole("alertdialog").getByRole("button", { name: "Rotate secret" }).click();
        await expect.poll(async () => (await secretCode.textContent()).trim()).not.toBe(originalSecret);

        await row.getByRole("button", { name: "Delete" }).click();
        await page.getByRole("alertdialog").getByRole("button", { name: "Delete" }).click();
        await expect(row).toHaveCount(0);
    } finally {
        await new Promise((resolve) => receiver.close(resolve));
    }
});

test("webhook controls stay hidden without instance or site permission", async ({ page }) => {
    await page.route("**/api/user/bootstrap", async (route) => {
        const response = await route.fetch();
        const bootstrap = await response.json();
        bootstrap.permissions = deniedWebhookPermissions();
        const headers = response.headers();
        delete headers["content-length"];
        await route.fulfill({ status: response.status(), headers: { ...headers, "content-type": "application/json" }, body: JSON.stringify(bootstrap) });
    });
    await page.route("**/api/user/permissions", async (route) => {
        await route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify(deniedWebhookPermissions())
        });
    });

    await login(page, "/integration/webhooks");
    await expect(page.getByText("Webhook access required")).toBeVisible();
    await expect(page.getByTestId("create-webhook")).toHaveCount(0);
});

function deniedWebhookPermissions() {
    return {
        instance_role: "owner",
        permissions: {},
        instance_capabilities: [],
        site_capabilities: {},
        active_team_capabilities: []
    };
}

async function activeSiteID(page) {
    const response = await page.request.get("/api/sites");
    const sites = await response.json();
    return sites[0].id;
}

async function webhookID(page) {
    const siteID = await activeSiteID(page);
    const response = await page.request.get(`/api/sites/${siteID}/webhooks`);
    const webhooks = await response.json();
    return webhooks.find((webhook) => webhook.name === "E2E receiver").id;
}
