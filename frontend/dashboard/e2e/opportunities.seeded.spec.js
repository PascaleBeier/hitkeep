const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

const PRIMARY_SEEDED_SITE_DOMAIN = "acme-analytics.io";

async function selectSeededSite(page, domain = PRIMARY_SEEDED_SITE_DOMAIN) {
    const combobox = page.locator('[role="combobox"]:visible').first();
    await expect(combobox).toBeVisible();

    const currentSite = ((await combobox.textContent()) || "").trim();
    if (currentSite.includes(domain)) {
        return;
    }

    await page.locator('[aria-label="Select a site to view stats"]:visible').first().click();
    const option = page.locator('[role="option"]:visible').filter({ hasText: domain }).first();
    await expect(option).toBeVisible();
    await option.click();
    await expect(combobox).toContainText(domain);
}

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
        checkout_starts: 120,
        top_city: "Berlin",
        top_provider: "Hetzner Online GmbH",
        top_asn: "AS24940 Hetzner Online GmbH"
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
        { id: "conversion_rate", label_key: "opportunities.evidence.checkout_conversion_rate", value: "42%" },
        { id: "top_city", label_key: "opportunities.evidence.top_city", value: "Berlin" },
        { id: "top_provider", label_key: "opportunities.evidence.top_provider", value: "Hetzner Online GmbH" },
        { id: "top_asn", label_key: "opportunities.evidence.top_asn", value: "AS24940 Hetzner Online GmbH" }
    ],
    cited_evidence_ids: ["checkout_starts", "conversion_rate", "top_city", "top_provider", "top_asn"],
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
        sessions: 1100,
        top_city: "Berlin",
        top_provider: "Hetzner Online GmbH",
        top_asn: "AS24940 Hetzner Online GmbH"
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
        { id: "top_city", label_key: "opportunities.evidence.top_city", value: "Berlin" },
        { id: "top_provider", label_key: "opportunities.evidence.top_provider", value: "Hetzner Online GmbH" },
        { id: "top_asn", label_key: "opportunities.evidence.top_asn", value: "AS24940 Hetzner Online GmbH" },
        { id: "source_hits", label_key: "opportunities.evidence.source_hits", value: "240" },
        { id: "total_pageviews", label_key: "opportunities.evidence.total_pageviews", value: "2400" },
        { id: "sessions", label_key: "opportunities.evidence.sessions", value: "1100" }
    ],
    cited_evidence_ids: ["top_source", "top_city", "top_provider", "top_asn", "source_hits", "total_pageviews", "sessions"]
};

const webVitalsOpportunity = {
    ...baseOpportunity,
    id: "e2e-op-3",
    kind: "performance",
    type_key: "opportunities.types.web_vitals_performance",
    title_key: "opportunities.catalog.web_vitals_performance.title",
    summary_key: "opportunities.catalog.web_vitals_performance.summary",
    action_key: "opportunities.catalog.web_vitals_performance.action",
    digest_key: "opportunities.catalog.web_vitals_performance.digest",
    copy_params: {
        metric: "LCP",
        p75: "3,840 ms",
        rating: "needs improvement",
        samples: 320,
        path: "/",
        top_city: "Berlin",
        top_provider: "Hetzner Online GmbH",
        top_asn: "AS24940 Hetzner Online GmbH"
    },
    impact_value: "320",
    impact_label_key: "opportunities.impact.web_vitals_samples",
    score: 86,
    route_label_key: "opportunities.routes.web_vitals",
    route_params: {
        metric: "LCP",
        path: "/"
    },
    route_icon: "pi pi-gauge",
    evidence: [
        { id: "web_vital_metric", label_key: "opportunities.evidence.web_vital_metric", value: "LCP" },
        { id: "web_vital_p75", label_key: "opportunities.evidence.web_vital_p75", value: "3,840 ms" },
        { id: "web_vital_rating", label_key: "opportunities.evidence.web_vital_rating", value: "needs improvement" },
        { id: "web_vital_samples", label_key: "opportunities.evidence.web_vital_samples", value: "320" },
        { id: "web_vital_top_page", label_key: "opportunities.evidence.web_vital_top_page", value: "/" },
        { id: "top_city", label_key: "opportunities.evidence.top_city", value: "Berlin" },
        { id: "top_provider", label_key: "opportunities.evidence.top_provider", value: "Hetzner Online GmbH" },
        { id: "top_asn", label_key: "opportunities.evidence.top_asn", value: "AS24940 Hetzner Online GmbH" }
    ],
    cited_evidence_ids: ["web_vital_metric", "web_vital_p75", "web_vital_rating", "web_vital_samples", "web_vital_top_page", "top_city", "top_provider", "top_asn"]
};

const aiVisibilityOpportunity = {
    ...baseOpportunity,
    id: "e2e-op-4",
    kind: "ai",
    type_key: "opportunities.types.ai_visibility",
    title_key: "opportunities.catalog.ai_visibility.title",
    summary_key: "opportunities.catalog.ai_visibility.summary",
    action_key: "opportunities.catalog.ai_visibility.action",
    digest_key: "opportunities.catalog.ai_visibility.digest",
    copy_params: {
        requests: 82,
        unique_paths: 7,
        top_path: "/pricing",
        ai_referrals: 32,
        top_path_pageviews: 420,
        top_city: "Berlin",
        top_provider: "Hetzner Online GmbH",
        top_asn: "AS24940 Hetzner Online GmbH"
    },
    impact_value: "+7",
    impact_label_key: "opportunities.impact.ai_touched_pages",
    score: 94,
    route_label_key: "opportunities.routes.path",
    route_params: {
        path: "/pricing"
    },
    route_icon: "pi pi-sparkles",
    evidence: [
        { id: "ai_requests", label_key: "opportunities.evidence.ai_requests", value: "82" },
        { id: "ai_paths", label_key: "opportunities.evidence.ai_paths", value: "7" },
        { id: "top_ai_path", label_key: "opportunities.evidence.top_ai_path", value: "/pricing" },
        { id: "ai_referrals", label_key: "opportunities.evidence.ai_referrals", value: "32" },
        { id: "ai_path_pageviews", label_key: "opportunities.evidence.ai_path_pageviews", value: "420" },
        { id: "top_city", label_key: "opportunities.evidence.top_city", value: "Berlin" },
        { id: "top_provider", label_key: "opportunities.evidence.top_provider", value: "Hetzner Online GmbH" },
        { id: "top_asn", label_key: "opportunities.evidence.top_asn", value: "AS24940 Hetzner Online GmbH" }
    ],
    cited_evidence_ids: ["ai_requests", "ai_paths", "top_ai_path", "ai_referrals", "ai_path_pageviews", "top_city", "top_provider", "top_asn"]
};

test("opportunities inbox renders real seeded recommendations", async ({ page }) => {
    await login(page, "/opportunities");
    await selectSeededSite(page);

    const inbox = page.getByLabel("Opportunity inbox");
    await expect(page.getByRole("heading", { name: "Opportunity inbox" })).toBeVisible();
    await expect(inbox.locator(".opportunity-card").first()).toBeVisible();
    await expect(page.getByText("Web Vitals samples").first()).toBeVisible();
    await expect(page.getByRole("button", { name: /Review .* performance on \// }).first()).toBeVisible();
    await expect(page.getByText("Evidence-backed recommendations")).toHaveCount(0);
    await expect(page.getByText("Prioritized recommendations")).toHaveCount(0);
});

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
    await expect(page.getByText("Top city: Berlin").first()).toBeVisible();
    await expect(page.getByText("Top provider: Hetzner Online GmbH").first()).toBeVisible();
    await expect(page.getByText("Top ASN: AS24940 Hetzner Online GmbH").first()).toBeVisible();

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

test("opportunities inbox renders checkout geo and network evidence", async ({ page }) => {
    await stubOpportunitiesApis(page);
    await login(page, "/opportunities");

    const inbox = page.getByLabel("Opportunity inbox");
    await inbox.getByRole("button", { name: "Review checkout drop-off" }).click();
    await expect(page.getByText("Top city: Berlin").first()).toBeVisible();
    await expect(page.getByText("Top provider: Hetzner Online GmbH").first()).toBeVisible();
    await expect(page.getByText("Top ASN: AS24940 Hetzner Online GmbH").first()).toBeVisible();
});

test("opportunities inbox renders Web Vitals performance evidence", async ({ page }) => {
    await stubOpportunitiesApis(page, { initial: webVitalsOpportunity });
    await login(page, "/opportunities");

    const inbox = page.getByLabel("Opportunity inbox");
    await expect(inbox.getByRole("button", { name: "Review LCP performance on /" })).toBeVisible();
    await expect(inbox.getByText("LCP p75 is 3,840 ms with a needs improvement rating across 320 samples.")).toBeVisible();
    await expect(inbox.getByText("Web Vitals samples")).toBeVisible();
    await expect(inbox.getByText("Slowest page")).toBeVisible();

    await inbox.getByRole("button", { name: "Review LCP performance on /" }).click();
    await expect(page.getByText("Top city: Berlin").first()).toBeVisible();
    await expect(page.getByText("Top provider: Hetzner Online GmbH").first()).toBeVisible();
    await expect(page.getByText("Top ASN: AS24940 Hetzner Online GmbH").first()).toBeVisible();
});

test("opportunities inbox renders AI visibility geo and network evidence", async ({ page }) => {
    await stubOpportunitiesApis(page, { initial: aiVisibilityOpportunity });
    await login(page, "/opportunities");

    const inbox = page.getByLabel("Opportunity inbox");
    await expect(inbox.getByRole("button", { name: "Review AI crawler attention" })).toBeVisible();
    await expect(inbox.getByText("AI assistants requested 82 pages; the strongest path is /pricing.")).toBeVisible();

    await inbox.getByRole("button", { name: "Review AI crawler attention" }).click();
    await expect(page.getByText("Top city: Berlin").first()).toBeVisible();
    await expect(page.getByText("Top provider: Hetzner Online GmbH").first()).toBeVisible();
    await expect(page.getByText("Top ASN: AS24940 Hetzner Online GmbH").first()).toBeVisible();
});

async function stubOpportunitiesApis(page, { initial = baseOpportunity, generated = generatedOpportunity } = {}) {
    let currentOpportunity = { ...initial };

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
            currentOpportunity = { ...generated, status: "new" };
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
