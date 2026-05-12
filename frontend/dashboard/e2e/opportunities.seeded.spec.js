const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

const baseOpportunity = {
    id: "e2e-op-1",
    team_id: "team-1",
    site_id: "site-1",
    kind: "conversion",
    type_key: "opportunities.types.checkout_conversion",
    title_key: "opportunities.catalog.checkout_conversion.title",
    summary_key: "opportunities.catalog.checkout_conversion.summary",
    action_key: "opportunities.catalog.checkout_conversion.action",
    digest_key: "opportunities.catalog.checkout_conversion.digest",
    copy_params: {
        conversion_rate: "42%",
        monthly_upside: "$8,500"
    },
    impact_value: "$8,500",
    impact_label_key: "opportunities.impact.estimated_monthly_upside",
    monthly_upside: 8500,
    confidence: "high",
    score: 92,
    status: "new",
    route_label_key: "opportunities.routes.checkout",
    route_params: {
        path: "/checkout"
    },
    route_icon: "pi pi-shopping-cart",
    detector_version: "opportunities-detectors-v1",
    evidence: [
        { id: "checkout_starts", label_key: "opportunities.evidence.checkout_starts", value: "120" },
        { id: "conversion_rate", label_key: "opportunities.evidence.checkout_conversion_rate", value: "42%" }
    ],
    cited_evidence_ids: ["checkout_starts", "conversion_rate"],
    title: "API should not render me",
    summary: "API should not render me",
    generated_at: "2026-05-09T10:00:00Z",
    created_at: "2026-05-09T10:00:00Z",
    updated_at: "2026-05-09T10:00:00Z"
};

const generatedOpportunity = {
    ...baseOpportunity,
    id: "e2e-op-2",
    kind: "revenue",
    type_key: "opportunities.types.traffic_quality",
    title_key: "opportunities.catalog.traffic_quality.title",
    summary_key: "opportunities.catalog.traffic_quality.summary",
    action_key: "opportunities.catalog.traffic_quality.action",
    digest_key: "opportunities.catalog.traffic_quality.digest",
    copy_params: {
        source: "google / cpc",
        pageviews: 2400
    },
    impact_value: "+2,400",
    impact_label_key: "opportunities.impact.pageviews_to_route",
    monthly_upside: 1200,
    score: 88,
    route_label_key: "opportunities.routes.source",
    route_params: {
        source: "google / cpc"
    },
    evidence: [
        { id: "pageviews", label_key: "opportunities.evidence.pageviews", value: "2400" },
        { id: "top_source", label_key: "opportunities.evidence.top_source", value: "google / cpc" }
    ],
    cited_evidence_ids: ["pageviews", "top_source"]
};

test("opportunities inbox supports localized read and manage workflow", async ({ page }) => {
    await stubOpportunitiesApis(page);
    await login(page, "/opportunities");

    await expect(page.getByRole("heading", { name: "Opportunity inbox" })).toBeVisible();
    await expect(page.getByText("Recover checkout drop-off")).toBeVisible();
    await expect(page.getByText("Checkout starts are converting at 42%")).toBeVisible();
    await expect(page.getByText("API should not render me")).toHaveCount(0);
    await expect(page.getByText("Self-hosted AI: openai gpt-test")).toBeVisible();

    await page.getByRole("button", { name: /refresh opportunities/i }).click();
    await expect(page.getByText("Focus on the source already pulling demand")).toBeVisible();

    const generatedCard = page.locator(".opportunity-card").filter({ hasText: "Focus on the source already pulling demand" }).first();
    await generatedCard.getByRole("button", { name: /save/i }).click();
    await expect(page.getByText("Saved").first()).toBeVisible();

    await generatedCard.getByRole("button", { name: /inspect/i }).click();
    await expect(page.getByText("Move campaign attention toward the highest-intent landing page.")).toBeVisible();

    await page.getByRole("button", { name: /mark done/i }).click();
    await expect(page.getByText("Done").first()).toBeVisible();

    await page.getByRole("button", { name: /dismiss/i }).click();
    await expect(page.getByText("No opportunities match this view")).toBeVisible();
});

test("opportunities inbox renders the same keyed recommendation in German", async ({ page }) => {
    await stubOpportunitiesApis(page);
    await login(page, "/opportunities");
    const originalLocale = await currentLocale(page);

    try {
        await setLocale(page, "de");
        await page.goto("/opportunities", { waitUntil: "domcontentloaded" });

        await expect(page.getByText("Checkout-Abbruch zurückholen")).toBeVisible();
        await expect(page.getByText("Checkout-Starts konvertieren mit 42%")).toBeVisible();
        await expect(page.getByText("API should not render me")).toHaveCount(0);
    } finally {
        await setLocale(page, originalLocale);
    }
});

async function stubOpportunitiesApis(page) {
    let currentOpportunity = { ...baseOpportunity };

    await page.route("**/api/admin/system/ai", async (route) => {
        await route.fulfill({
            contentType: "application/json",
            body: JSON.stringify({
                status: "configured",
                enabled: true,
                configured: true,
                config_mode: "self_hosted",
                provider: "openai",
                model: "gpt-test",
                base_url_configured: false,
                requests_used: 0,
                request_limit: 100,
                tokens_used: 0,
                token_limit: 10000,
                budget_window_minutes: 60,
                budget_exhausted: false
            })
        });
    });

    await page.route("**/api/sites/*/opportunities**", async (route) => {
        const request = route.request();
        const url = new URL(request.url());
        if (request.method() === "GET") {
            await route.fulfill({ contentType: "application/json", body: JSON.stringify({ opportunities: [currentOpportunity] }) });
            return;
        }
        if (request.method() === "POST" && url.pathname.endsWith("/opportunities/generate")) {
            currentOpportunity = { ...generatedOpportunity, status: "new" };
            await route.fulfill({ contentType: "application/json", body: JSON.stringify({ opportunities: [currentOpportunity], ai_status: "success" }) });
            return;
        }
        if (request.method() === "PATCH") {
            const body = request.postDataJSON();
            currentOpportunity = { ...currentOpportunity, status: body.status };
            await route.fulfill({ contentType: "application/json", body: JSON.stringify(currentOpportunity) });
            return;
        }
        await route.fallback();
    });
}

async function setLocale(page, locale) {
    const response = await page.request.put("/api/user/preferences", {
        headers: originHeaders(page),
        data: { default_locale: locale }
    });
    const body = await response.text();
    expect(response.ok(), `set locale returned ${response.status()}: ${body}`).toBeTruthy();
}

async function currentLocale(page) {
    const response = await page.request.get("/api/user/preferences");
    const body = await response.text();
    expect(response.ok(), `get locale returned ${response.status()}: ${body}`).toBeTruthy();
    return JSON.parse(body).default_locale || "en";
}

function originHeaders(page) {
    return { Origin: new URL(page.url()).origin };
}
