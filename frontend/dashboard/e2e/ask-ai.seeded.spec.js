const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

test("ask ai stays hidden for disabled installs", async ({ page }) => {
    await login(page, "/dashboard");

    await expect(page.locator("app-ask-ai-control .ask-ai-trigger")).toHaveCount(0);
});

test("ask ai answers with a chart, page link, and export action", async ({ page }) => {
    let postedSiteId = "";

    await enableAskAIForBootstrap(page);
    await page.route("**/api/sites/*/ask-ai/events", async (route) => {
        const request = route.request();
        const url = new URL(request.url());
        postedSiteId = url.pathname.match(/\/api\/sites\/([^/]+)\/ask-ai\/events$/)?.[1] || "";
        const body = JSON.parse(request.postData() || "{}");

        expect(body.query).toContain("What changed in traffic?");
        expect(body.route).toMatch(/\/dashboard/);
        expect(body.from).toBeUndefined();
        expect(body.to).toBeUndefined();
        expect(postedSiteId).not.toBe("");

        await route.fulfill({
            status: 200,
            contentType: "text/event-stream",
            body: [
                'event: progress\ndata: {"type":"progress","status":"generating","message_key":"askAi.progress.generating"}\n',
                'event: delta\ndata: {"type":"delta","status":"streaming","delta_markdown":"Traffic increased"}\n',
                `event: final\ndata: ${JSON.stringify({
                    type: "final",
                    status: "success",
                    response: {
                        run_id: "11111111-1111-4111-8111-111111111111",
                        answer_markdown: "Traffic increased, with the strongest lift near the end of the selected view.",
                        citations: [
                            {
                                label: "Site overview",
                                tool_call_id: "hitkeep_get_site_overview"
                            }
                        ],
                        charts: [
                            {
                                type: "line",
                                title: "Traffic by day",
                                x_key: "date",
                                series: [{ label: "Visitors", key: "visitors" }],
                                rows: [
                                    { date: "2026-06-19", visitors: 42 },
                                    { date: "2026-06-20", visitors: 48 },
                                    { date: "2026-06-21", visitors: 55 }
                                ]
                            }
                        ],
                        actions: [
                            {
                                type: "download_export",
                                label: "Download export",
                                target: `/api/sites/${postedSiteId}/takeout?format=json`,
                                format: "json"
                            },
                            { type: "navigate", label: "Open events", target: "/events" }
                        ]
                    }
                })}\n`
            ].join("\n")
        });
    });
    await page.addInitScript(() => localStorage.setItem("hk_theme", "dark"));
    await login(page, "/dashboard");
    await expect(page.locator("html")).toHaveClass(/p-dark/);

    await page.getByRole("button", { name: "Ask AI about this site" }).click();
    await expect(page.getByRole("heading", { name: "Ask AI" })).toBeVisible();
    const askInput = page.getByRole("searchbox", { name: "Ask AI prompt" });
    await expect(askInput).toBeVisible();
    await askInput.fill("What changed in traffic?");
    await askInput.press("Enter");

    await expect(page.getByText("Traffic increased")).toBeVisible();
    await expect(page.getByText("Traffic increased, with the strongest lift near the end of the selected view.")).toBeVisible();
    await expect(page.getByText("Site overview")).toBeVisible();
    await expect(page.getByText("Traffic by day")).toBeVisible();
    await expect(page.locator(".ask-ai-drawer canvas")).toBeVisible();

    const exportResponse = page.waitForResponse((response) => postedSiteId !== "" && response.url().includes(`/api/sites/${postedSiteId}/takeout?format=json`) && response.ok());
    const exportDownload = page.waitForEvent("download");
    await page.getByRole("button", { name: "Download export" }).click();
    const [download] = await Promise.all([exportDownload, exportResponse]);
    expect(download.suggestedFilename().toLowerCase()).toContain(".json");
    await expect(page.getByText("Export download started.")).toBeVisible();

    await page.getByRole("button", { name: "Open events" }).click();
    await expect(page).toHaveURL(/\/events(?:\?.*)?$/);

    await page.goBack();
    await expect(page).toHaveURL(/\/dashboard(?:\?.*)?$/);
    await page.getByRole("button", { name: "Ask AI about this site" }).click();
    await expect(page.getByRole("tab", { name: /History/ })).toHaveCount(0);
    await expect(page.getByText("openai.gpt-oss-120b")).toHaveCount(0);
    await expect(page.getByText("11111111-1111-4111-8111-111111111111")).toHaveCount(0);
});

test("ask ai opens in unavailable and mobile drawer states", async ({ page }) => {
    await enableAskAIForBootstrap(page, {
        available: false,
        status: "not_configured"
    });

    await page.setViewportSize({ width: 390, height: 844 });
    await login(page, "/dashboard");

    await page.getByRole("button", { name: "Main sidebar" }).click();
    await page.getByRole("button", { name: "Ask AI about this site" }).click();
    await expect(page.getByRole("heading", { name: "Ask AI" })).toBeVisible();
    const drawer = page.locator(".ask-ai-drawer");
    await expect(drawer.getByText("Ask AI not configured")).toBeVisible();
    await expect(page.getByRole("searchbox", { name: "Ask AI prompt" })).toBeDisabled();
    await expect(page.getByRole("tab", { name: /History/ })).toHaveCount(0);
});

async function enableAskAIForBootstrap(page, askAIStatus = {}) {
    await page.route("**/api/user/bootstrap", async (route) => {
        const response = await route.fetch();
        const bootstrap = await response.json();
        const headers = response.headers();
        delete headers["content-length"];

        bootstrap.status = bootstrap.status || {};
        bootstrap.status.ai = {
            ...(bootstrap.status.ai || {}),
            enabled: true,
            configured: true,
            provider: "openai-compatible",
            model: "openai.gpt-oss-120b",
            ask_ai_enabled: true,
            ask_ai_available: true
        };
        bootstrap.status.ask_ai = {
            enabled: true,
            available: true,
            status: "available",
            provider: "openai-compatible",
            model: "openai.gpt-oss-120b",
            budget_exhausted: false,
            ...askAIStatus
        };

        await route.fulfill({
            status: response.status(),
            headers: { ...headers, "content-type": "application/json" },
            body: JSON.stringify(bootstrap)
        });
    });
}
