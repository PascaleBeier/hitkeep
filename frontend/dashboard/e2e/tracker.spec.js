const path = require("node:path");
const { test, expect } = require("playwright/test");

const TRACKER_FIXTURE_DIR = path.resolve(__dirname, "../../../tests/fixtures/tracker");
const TRACKER_SCRIPT_PATH = path.resolve(__dirname, "../dist/dashboard/browser/hk.js");

test.beforeEach(async ({ context }) => {
    await context.route("**/ingest", async (route) => {
        await route.fulfill({ status: 204, body: "" });
    });

    await context.route("**/ingest/event", async (route) => {
        await route.fulfill({ status: 204, body: "" });
    });

    await context.route("**/hk.js", async (route) => {
        await route.fulfill({
            path: TRACKER_SCRIPT_PATH,
            contentType: "application/javascript"
        });
    });

    await context.route("**/tracker-fixtures/**", async (route) => {
        const url = new URL(route.request().url());
        const relativePath = decodeURIComponent(url.pathname).replace(/^\/tracker-fixtures\/?/, "");
        const fixturePath = path.resolve(TRACKER_FIXTURE_DIR, relativePath);
        if (!fixturePath.startsWith(`${TRACKER_FIXTURE_DIR}${path.sep}`)) {
            await route.fulfill({ status: 403, body: "Forbidden" });
            return;
        }

        await route.fulfill({
            path: fixturePath,
            contentType: trackerFixtureContentType(fixturePath)
        });
    });
});

function trackerFixtureUrl(baseURL, search = "") {
    const base = new URL(baseURL);
    const params = new URLSearchParams(search);
    if (!params.has("disableBeacon")) {
        params.set("disableBeacon", "1");
    }
    const query = params.toString() ? `?${params.toString()}` : "";
    return `${base.protocol}//lvh.me:${base.port}/tracker-fixtures/auto-events.html${query}`;
}

async function gotoTrackerFixture(page, baseURL, search = "") {
    await page.goto(trackerFixtureUrl(baseURL, search));
    await expect.poll(() => page.evaluate(() => Boolean(window.hk?._bootstrapped))).toBe(true);
}

function trackerFixtureContentType(filePath) {
    switch (path.extname(filePath)) {
        case ".csv":
            return "text/csv";
        case ".html":
            return "text/html";
        default:
            return "application/octet-stream";
    }
}

function collectEventPayloads(page) {
    const payloads = [];

    page.on("request", (request) => {
        if (!request.url().includes("/ingest/event")) {
            return;
        }

        const body = request.postData();
        if (!body) {
            return;
        }

        payloads.push(JSON.parse(body));
    });

    return payloads;
}

async function waitForEventRequest(page) {
    const request = await page.waitForRequest((candidate) => candidate.url().includes("/ingest/event") && candidate.method() === "POST");
    return JSON.parse(request.postData() || "{}");
}

test("tracks outbound clicks and still navigates", async ({ page, baseURL }) => {
    await gotoTrackerFixture(page, baseURL);

    const requestPromise = waitForEventRequest(page);
    await Promise.all([page.waitForURL(/external\.lvh\.me.*external-target\.html/), page.locator("#outbound-link").click()]);

    const payload = await requestPromise;
    expect(payload.n).toBe("outbound_click");
    expect(payload.p).toEqual({
        target_host: "external.lvh.me",
        target_path: "/tracker-fixtures/external-target.html",
        target_protocol: "http"
    });
});

test("tracks middle-click outbound links without hijacking the current tab", async ({ page, baseURL, context }) => {
    await gotoTrackerFixture(page, baseURL);

    const requestPromise = waitForEventRequest(page);
    const newPagePromise = context.waitForEvent("page");

    await page.locator("#outbound-link").click({ button: "middle" });

    const [payload, newPage] = await Promise.all([requestPromise, newPagePromise]);
    await newPage.waitForURL(/external\.lvh\.me.*external-target\.html/);

    expect(payload.n).toBe("outbound_click");
    await expect(page).toHaveURL(/tracker-fixtures\/auto-events\.html/);

    await newPage.close();
});

test("tracks local downloads once and only downloads once", async ({ page, baseURL }) => {
    await gotoTrackerFixture(page, baseURL);

    let downloadCount = 0;
    page.on("download", () => {
        downloadCount += 1;
    });

    const requestPromise = waitForEventRequest(page);
    const downloadPromise = page.waitForEvent("download");

    await page.locator("#local-download-link").click();

    const [payload, download] = await Promise.all([requestPromise, downloadPromise]);
    await download.path();
    await page.waitForTimeout(400);

    expect(downloadCount).toBe(1);
    expect(payload.n).toBe("file_download");
    expect(payload.p).toEqual({
        file_host: "lvh.me",
        file_path: "/tracker-fixtures/downloads/demo-report.csv",
        file_ext: "csv"
    });
});

test("prefers outbound_click over file_download for outbound downloads", async ({ page, baseURL }) => {
    const payloads = collectEventPayloads(page);

    await gotoTrackerFixture(page, baseURL);

    const requestPromise = waitForEventRequest(page);
    await page.locator("#outbound-download-link").click();

    const payload = await requestPromise;
    await page.waitForTimeout(400);

    expect(payload.n).toBe("outbound_click");
    expect(payloads).toHaveLength(1);
});

test("tracks form_submit for click-based submissions", async ({ page, baseURL }) => {
    await gotoTrackerFixture(page, baseURL);

    const requestPromise = waitForEventRequest(page);
    await Promise.all([page.waitForURL(/form-complete\.html\?source=click/), page.locator("#click-submit-button").click()]);

    const payload = await requestPromise;
    expect(payload.n).toBe("form_submit");
    expect(payload.p).toEqual({
        action_host: "lvh.me",
        action_path: "/tracker-fixtures/form-complete.html",
        method: "post",
        same_origin: true,
        form_id: "click-submit-form"
    });
});

test("tracks form_submit for enter-key submissions", async ({ page, baseURL }) => {
    await gotoTrackerFixture(page, baseURL);

    const requestPromise = waitForEventRequest(page);
    await page.locator("#enter-submit-input").fill("analytics");
    await page.locator("#enter-submit-input").press("Enter");

    const payload = await requestPromise;
    expect(payload.n).toBe("form_submit");
    expect(payload.p).toEqual({
        action_host: "lvh.me",
        action_path: "/tracker-fixtures/form-complete.html",
        method: "get",
        same_origin: true,
        form_id: "enter-submit-form"
    });
});

test("does not track outbound link clicks inside forms", async ({ page, baseURL, context }) => {
    const payloads = collectEventPayloads(page);

    await gotoTrackerFixture(page, baseURL);

    const newPagePromise = context.waitForEvent("page");
    await page.locator("#inside-form-link").click();

    const newPage = await newPagePromise;
    await newPage.waitForURL(/external\.lvh\.me.*external-target\.html/);
    await page.waitForTimeout(400);

    expect(payloads).toHaveLength(0);
    await newPage.close();
});

test("disabling outbound auto-tracking only suppresses outbound_click", async ({ page, baseURL, context }) => {
    const payloads = collectEventPayloads(page);

    await gotoTrackerFixture(page, baseURL, "disableOutbound=1&disableBeacon=1");

    const downloadPromise = page.waitForEvent("download");
    await page.locator("#local-download-link").click();
    await downloadPromise;
    await page.waitForTimeout(300);

    const newPagePromise = context.waitForEvent("page");
    await page.locator("#outbound-link").click({ button: "middle" });
    const newPage = await newPagePromise;
    await newPage.waitForURL(/external\.lvh\.me.*external-target\.html/);
    await page.waitForTimeout(400);

    expect(payloads.map((payload) => payload.n)).toEqual(["file_download"]);
    await newPage.close();
});

test("disabling download auto-tracking only suppresses file_download", async ({ page, baseURL }) => {
    const payloads = collectEventPayloads(page);

    await gotoTrackerFixture(page, baseURL, "disableDownload=1&disableBeacon=1");

    const requestPromise = waitForEventRequest(page);
    await Promise.all([page.waitForURL(/form-complete\.html\?source=click/), page.locator("#click-submit-button").click()]);
    await requestPromise;
    await page.waitForTimeout(300);

    expect(payloads.map((payload) => payload.n)).toEqual(["form_submit"]);
});

test("disabling form auto-tracking only suppresses form_submit", async ({ page, baseURL, context }) => {
    const payloads = collectEventPayloads(page);

    await gotoTrackerFixture(page, baseURL, "disableForm=1&disableBeacon=1");

    const newPagePromise = context.waitForEvent("page");
    await page.locator("#outbound-link").click({ button: "middle" });
    const newPage = await newPagePromise;
    await newPage.waitForURL(/external\.lvh\.me.*external-target\.html/);
    await page.waitForTimeout(300);
    await newPage.close();

    await gotoTrackerFixture(page, baseURL, "disableForm=1&disableBeacon=1");
    await Promise.all([page.waitForURL(/form-complete\.html\?source=click/), page.locator("#click-submit-button").click()]);
    await page.waitForTimeout(400);

    expect(payloads.map((payload) => payload.n)).toEqual(["outbound_click"]);
});
