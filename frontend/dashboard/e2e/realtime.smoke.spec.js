const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

const PRIMARY_SEEDED_SITE_DOMAIN = "acme-analytics.io";
const REFRESH_TIMEOUT_MS = 10_000;
const STREAM_TIMEOUT_MS = 15_000;

test.describe("realtime smoke", () => {
    test("opens the seeded site stream and refreshes dashboard analytics after a live definition change", async ({ page }) => {
        const realtimeRequests = collectRealtimeRequests(page);
        const analyticsResponses = collectAnalyticsResponses(page);

        await login(page, "/dashboard");
        const site = await getSeededSite(page);
        analyticsResponses.setSiteID(site.id);

        await selectSeededSite(page, site.domain);
        await expect(page.getByText("Latest Hits")).toBeVisible();

        await expect.poll(() => realtimeRequests.some((url) => url.includes(`/api/sites/${site.id}/realtime`)), { timeout: STREAM_TIMEOUT_MS }).toBeTruthy();

        const goalName = `Realtime smoke ${Date.now()}`;

        try {
            const triggerStartedAt = Date.now();
            const createResponse = await page.request.post(`/api/sites/${site.id}/goals`, {
                headers: originHeaders(page),
                data: {
                    name: goalName,
                    type: "event",
                    value: `realtime_smoke_${triggerStartedAt}`
                }
            });
            const createBody = await createResponse.text();
            expect(createResponse.status(), `create goal returned ${createResponse.status()}: ${createBody}`).toBe(201);

            await expect.poll(() => analyticsResponses.hasSuccessfulResponse("stats", triggerStartedAt), { timeout: REFRESH_TIMEOUT_MS }).toBeTruthy();
            await expect.poll(() => analyticsResponses.hasSuccessfulResponse("hits", triggerStartedAt), { timeout: REFRESH_TIMEOUT_MS }).toBeTruthy();
        } finally {
            await deleteGoalByName(page, site.id, goalName);
        }
    });
});

async function getSeededSite(page, domain = PRIMARY_SEEDED_SITE_DOMAIN) {
    const response = await page.request.get("/api/sites");
    const body = await response.text();
    expect(response.ok(), `get sites returned ${response.status()}: ${body}`).toBeTruthy();

    const site = JSON.parse(body).find((candidate) => candidate.domain === domain);
    expect(site, `expected seeded site ${domain}`).toBeTruthy();
    return site;
}

async function selectSeededSite(page, domain = PRIMARY_SEEDED_SITE_DOMAIN) {
    const combobox = page.locator('app-site-selector [role="combobox"]:visible').first();
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

async function deleteGoalByName(page, siteID, name) {
    const goalsResponse = await page.request.get(`/api/sites/${siteID}/goals`);
    const goalsBody = await goalsResponse.text();
    expect(goalsResponse.ok(), `get goals returned ${goalsResponse.status()}: ${goalsBody}`).toBeTruthy();

    const goal = JSON.parse(goalsBody).find((candidate) => candidate.name === name);
    if (!goal) {
        return;
    }

    const deleteResponse = await page.request.delete(`/api/sites/${siteID}/goals/${goal.id}`, {
        headers: originHeaders(page)
    });
    expect([200, 404]).toContain(deleteResponse.status());
}

function collectRealtimeRequests(page) {
    const requests = [];
    page.on("request", (request) => {
        const url = request.url();
        if (url.includes("/api/sites/") && url.includes("/realtime")) {
            requests.push(url);
        }
    });
    return requests;
}

function collectAnalyticsResponses(page) {
    let siteID = "";
    const requestStartedAt = new WeakMap();
    const responses = [];

    page.on("request", (request) => {
        const kind = analyticsKind(request.url(), siteID);
        if (request.method() === "GET" && kind) {
            requestStartedAt.set(request, Date.now());
        }
    });

    page.on("response", (response) => {
        const kind = analyticsKind(response.url(), siteID);
        if (!kind || response.request().method() !== "GET") {
            return;
        }

        responses.push({
            kind,
            startedAt: requestStartedAt.get(response.request()) || Date.now(),
            status: response.status()
        });
    });

    return {
        setSiteID(nextSiteID) {
            siteID = nextSiteID;
        },
        hasSuccessfulResponse(kind, since) {
            return responses.some((response) => response.kind === kind && response.startedAt >= since && response.status < 400);
        }
    };
}

function analyticsKind(rawURL, siteID) {
    if (!siteID) {
        return "";
    }

    const pathname = new URL(rawURL).pathname;
    if (pathname.includes(`/api/sites/${siteID}/stats`)) {
        return "stats";
    }
    if (pathname.includes(`/api/sites/${siteID}/hits`)) {
        return "hits";
    }
    return "";
}

function originHeaders(page) {
    return { Origin: new URL(page.url()).origin };
}
