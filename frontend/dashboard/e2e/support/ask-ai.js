async function enableAskAIForBootstrap(page, askAIStatus = {}, cloudHosted = false) {
    await page.route("**/api/sites/*/ask-ai/status", async (route) => {
        await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ enabled: true, available: true, status: "available", budget_exhausted: false, ...askAIStatus }) });
    });
    await page.route("**/api/user/bootstrap", async (route) => {
        const response = await route.fetch();
        const bootstrap = await response.json();
        const headers = response.headers();
        delete headers["content-length"];

        bootstrap.status = bootstrap.status || {};
        if (cloudHosted) {
            bootstrap.status.cloud = { ...(bootstrap.status.cloud || {}), hosted: true };
        }
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

module.exports = { enableAskAIForBootstrap };
