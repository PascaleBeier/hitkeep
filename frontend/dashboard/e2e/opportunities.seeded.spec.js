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
        checkout_starts: 120
    },
    impact_value: "120",
    impact_label_key: "opportunities.impact.checkout_starts",
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
    kind: "traffic",
    type_key: "opportunities.types.traffic_quality",
    title_key: "opportunities.catalog.traffic_quality.title",
    summary_key: "opportunities.catalog.traffic_quality.summary",
    action_key: "opportunities.catalog.traffic_quality.action",
    digest_key: "opportunities.catalog.traffic_quality.digest",
    copy_params: {
        source: "google / cpc",
        source_hits: 240,
        total_pageviews: 2400,
        sessions: 1100
    },
    impact_value: "240",
    impact_label_key: "opportunities.impact.pageviews_to_route",
    score: 88,
    route_label_key: "opportunities.routes.source",
    route_params: {
        source: "google / cpc"
    },
    evidence: [
        { id: "top_source", label_key: "opportunities.evidence.top_source", value: "google / cpc" },
        { id: "source_hits", label_key: "opportunities.evidence.source_hits", value: "240" },
        { id: "total_pageviews", label_key: "opportunities.evidence.total_pageviews", value: "2400" },
        { id: "sessions", label_key: "opportunities.evidence.sessions", value: "1100" }
    ],
    cited_evidence_ids: ["top_source", "source_hits", "total_pageviews", "sessions"]
};

test("opportunities inbox supports localized read and manage workflow", async ({ page }) => {
    await stubOpportunitiesApis(page);
    await login(page, "/opportunities");

    await expect(page.getByRole("heading", { name: "Opportunity inbox" })).toBeVisible();
    const inbox = page.getByLabel("Opportunity inbox");
    await expect(inbox.getByRole("button", { name: "Review checkout drop-off" })).toBeVisible();
    await expect(inbox.getByText("Checkout starts are converting at 42%")).toBeVisible();
    await expect(page.getByText("API should not render me")).toHaveCount(0);
    await expect(page.getByText("Self-hosted AI: openai gpt-test")).toBeVisible();

    await page.getByRole("button", { name: /refresh opportunities/i }).click();
    await expect(inbox.getByRole("button", { name: "Review traffic from google / cpc" })).toBeVisible();

    const generatedCard = page.locator(".opportunity-card").filter({ hasText: "Review traffic from google / cpc" }).first();
    await generatedCard.getByRole("button", { name: /save/i }).click();
    await expect(page.getByText("Saved").first()).toBeVisible();

    await generatedCard.getByRole("button", { name: /inspect/i }).click();
    await expect(page.getByText("Inspect the landing pages and intent for visitors from google / cpc.")).toBeVisible();

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

        await expect(page.getByLabel("Opportunity-Inbox").getByRole("button", { name: "Checkout-Abbruch prüfen" })).toBeVisible();
        await expect(page.getByLabel("Opportunity-Inbox").getByText("Checkout-Starts konvertieren mit 42%")).toBeVisible();
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
