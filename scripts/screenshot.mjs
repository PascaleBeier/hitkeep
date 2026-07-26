#!/usr/bin/env node
/**
 * HitKeep Dashboard Screenshot Tool
 *
 * Captures dashboard screenshots for docs/marketing.
 *
 * Prerequisites:
 *   npm install playwright
 *   npx playwright install chromium
 *
 * Usage:
 *   HITKEEP_URL=http://localhost:8080 \
 *   HITKEEP_EMAIL=admin@example.com \
 *   HITKEEP_PASSWORD=yourpassword \
 *   node scripts/screenshot.mjs
 *
 * Environment variables:
 *   HITKEEP_URL      Base URL of your local instance (default: http://localhost:8080)
 *   HITKEEP_EMAIL    Admin account email (required)
 *   HITKEEP_PASSWORD Admin account password (required)
 *   OUTPUT_DIR       Output directory (default: ../hitkeep-docs/src/assets/screenshots)
 *   SCALE            Device pixel ratio (default: 2)
 *
 * For fast agent visual QA against an active seeded workspace, use
 * `./hk screenshot /route` instead. This script remains the curated docs set.
 */

import { existsSync, mkdirSync } from "fs";
import { join, resolve, dirname } from "path";
import { fileURLToPath, pathToFileURL } from "url";

const __dir = dirname(fileURLToPath(import.meta.url));
const { chromium } = await loadPlaywright();

const BASE_URL = (process.env.HITKEEP_URL ?? "http://localhost:8080").replace(/\/$/, "");
const EMAIL = process.env.HITKEEP_EMAIL;
const PASSWORD = process.env.HITKEEP_PASSWORD;
const SCALE = parseFloat(process.env.SCALE ?? "2");
const DESKTOP_VIEWPORT = { width: 1440, height: 1024 };
const MOBILE_VIEWPORT = { width: 390, height: 844 };
// The shared metric card grid is three columns from 1280px up
// (frontend/dashboard/src/app/features/analytics/components/metric-card-group.css),
// so two columns keep the AI Agents grid's four groups a gap-free 2x2.
const AI_CORRELATION_VIEWPORT = { width: 1180, height: 1024 };
const SCREENSHOT_TARGET = (process.env.SCREENSHOT_TARGET ?? "").trim().toLowerCase();
const OUTPUT_DIR = resolve(
  process.env.OUTPUT_DIR ?? join(__dir, "../../hitkeep-docs/src/assets/screenshots"),
);

if (!EMAIL || !PASSWORD) {
  console.error("\n  Error: HITKEEP_EMAIL and HITKEEP_PASSWORD are required.\n");
  process.exit(1);
}

const CHART_SETTLE = 2500;
const TABLE_SETTLE = 1000;
const FORM_SETTLE = 600;
const DEMO_SITE_DOMAIN = process.env.HITKEEP_SCREENSHOT_SITE ?? "acme-analytics.io";

async function loadPlaywright() {
  try {
    return await import("playwright");
  } catch (err) {
    const workspacePlaywright = join(__dir, "../frontend/dashboard/node_modules/playwright/index.mjs");
    if (existsSync(workspacePlaywright)) {
      return await import(pathToFileURL(workspacePlaywright).href);
    }
    throw err;
  }
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function login(page) {
  await page.goto(`${BASE_URL}/login`, { waitUntil: "domcontentloaded", timeout: 20_000 });
  await page.waitForSelector('input[type="password"]', { state: "visible", timeout: 10_000 });

  await page.locator('input[type="email"], input[name="email"]').first().fill(EMAIL);
  await page.locator('input[type="password"]').fill(PASSWORD);

  await page.locator('button[type="submit"]').click();
  await page.waitForURL((url) => !url.pathname.includes("/login"), { timeout: 12_000 });
}

async function nav(page, path, settle = TABLE_SETTLE) {
  await page.goto(`${BASE_URL}${path}`, { waitUntil: "domcontentloaded", timeout: 20_000 });
  await page.waitForFunction(() => !document.fonts || document.fonts.status === "loaded", null, { timeout: 3_000 }).catch(() => {});
  await page.evaluate(() => new Promise((resolveFrame) => requestAnimationFrame(() => requestAnimationFrame(resolveFrame))));
  await page.waitForTimeout(settle);
}

async function clickTab(page, label, settle = TABLE_SETTLE) {
  const tab = page.getByRole("tab", { name: new RegExp(label, "i") }).first();
  if (!(await tab.count())) {
    console.warn(`    ! Tab not found: ${label}`);
    return false;
  }
  await tab.click();
  await page.waitForFunction((tabLabel) => {
    const tabs = Array.from(document.querySelectorAll('[role="tab"]'));
    const matched = tabs.find((el) => new RegExp(tabLabel, "i").test(el.textContent || ""));
    return matched?.getAttribute("aria-selected") === "true";
  }, label);
  await page.waitForTimeout(settle);
  return true;
}

async function selectRangePreset(page, label, settle = CHART_SETTLE) {
  const toolbar = page.locator("app-range-toolbar").first();
  const option = toolbar
    .getByRole("button", { name: new RegExp(`^${escapeRegExp(label)}$`) })
    .first();
  if (!(await option.count())) {
    console.warn(`    ! Range preset not found: ${label}`);
    return false;
  }
  await option.click();
  await page.waitForTimeout(settle);
  return true;
}

async function shoot(page, slug, { fullPage = false, clip } = {}) {
  const file = join(OUTPUT_DIR, `${slug}.png`);
  try {
    await page.screenshot({ path: file, fullPage, clip, animations: "disabled" });
    return { ok: true, file };
  } catch (err) {
    return { ok: false, file, error: err.message };
  }
}

async function installAskAIDemoRoutes(page) {
  await page.route("**/api/user/bootstrap", async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    body.status = body.status || {};
    body.status.ask_ai = {
      enabled: true,
      available: true,
      status: "available",
      provider: "openai-compatible",
      model: "gpt-4.1-mini",
      budget_exhausted: false,
    };
    await route.fulfill({ response, json: body });
  });

  await page.route("**/api/sites/*/ask-ai/events", async (route) => {
    const events = [
      { type: "progress", status: "accepted", message_key: "askAi.progress.accepted" },
      { type: "progress", status: "generating", message_key: "askAi.progress.generating" },
      {
        type: "progress",
        status: "tool_call_start",
        message_key: "askAi.progress.readingAnalytics",
        tool_call_id: "tool-site-overview",
        tool_name: "hitkeep_get_site_overview",
      },
      {
        type: "progress",
        status: "tool_call_finish",
        message_key: "askAi.progress.composing",
        tool_call_id: "tool-site-overview",
        tool_name: "hitkeep_get_site_overview",
      },
      {
        type: "progress",
        status: "tool_call_start",
        message_key: "askAi.progress.readingAnalytics",
        tool_call_id: "tool-ai-visibility",
        tool_name: "hitkeep_get_ai_visibility",
      },
      {
        type: "progress",
        status: "tool_call_finish",
        message_key: "askAi.progress.composing",
        tool_call_id: "tool-ai-visibility",
        tool_name: "hitkeep_get_ai_visibility",
      },
      {
        type: "final",
        status: "success",
        message_key: "askAi.progress.complete",
        response: {
          run_id: "ask-ai-screenshot-demo",
          answer_markdown:
            "In this demo site, ChatGPT sent **128 visits** in the last 14 days.\n\n- 94 came from ChatGPT referrals.\n- 34 landed after OpenAI crawler fetches.\n- The docs and pricing pages were the strongest paths.",
          citations: [
            { label: "Site overview", tool_call_id: "tool-site-overview" },
            { label: "AI visibility", tool_call_id: "tool-ai-visibility" },
          ],
          charts: [
            {
              type: "table",
              title: "Top demo ChatGPT paths",
              rows: [
                { path: "/docs", visits: 54 },
                { path: "/pricing", visits: 38 },
                { path: "/guides/analytics/ai-visibility", visits: 21 },
              ],
            },
          ],
          actions: [{ type: "navigate", label: "Open AI visibility", target: "/ai-agents" }],
        },
      },
    ];
    const body = events.map((event) => `event: ${event.type}\ndata: ${JSON.stringify(event)}\n`).join("\n");
    await route.fulfill({
      status: 200,
      headers: {
        "Cache-Control": "no-cache",
        "Content-Type": "text/event-stream; charset=utf-8",
      },
      body: `${body}\n`,
    });
  });
}

async function maybeSelectFirstOption(page, comboLabel) {
  const combo = page.getByRole("combobox", { name: new RegExp(comboLabel, "i") }).first();
  if (!(await combo.count())) return false;
  if (!(await combo.isEnabled())) return false;
  if ((await combo.getAttribute("aria-disabled")) === "true") return false;

  await combo.click({ timeout: 1500 });
  const option = page.getByRole("option").first();
  if (!(await option.count())) return false;

  await option.click();
  await page.waitForTimeout(TABLE_SETTLE);
  return true;
}

async function waitForSelectEnabled(page, selector, timeout = 8_000) {
  await page.waitForFunction(
    (cssSelector) => {
      const el = document.querySelector(cssSelector);
      if (!el) return false;
      const ariaDisabled = el.getAttribute("aria-disabled");
      return ariaDisabled !== "true" && !el.classList.contains("p-disabled");
    },
    selector,
    { timeout },
  );
}

async function selectFirstVisibleOption(page, selector) {
  const select = page.locator(`${selector}:visible`).first();
  if (!(await select.count())) return false;
  if (!(await select.isEnabled())) return false;
  if ((await select.getAttribute("aria-disabled")) === "true") return false;

  await select.click({ timeout: 2_000 });

  await page.waitForSelector('[role="option"]:visible', { timeout: 5_000 });
  const option = page.locator('[role="option"]:visible').first();
  if (!(await option.count())) return false;

  const optionText = ((await option.textContent()) ?? "").trim();
  await option.click();
  await page.waitForTimeout(TABLE_SETTLE);

  if (optionText) {
    try {
      await page.waitForFunction(
        ({ cssSelector, expectedText }) => {
          const el = document.querySelector(cssSelector);
          return !!el && (el.textContent || "").includes(expectedText);
        },
        { cssSelector: selector, expectedText: optionText },
        { timeout: 5_000 },
      );
    } catch {
      // OptimusUI may render the selected label outside the host element.
    }
  }

  return true;
}

async function selectVisibleOptionMatching(page, selector, pattern) {
  const select = page.locator(`${selector}:visible`).first();
  if (!(await select.count())) return false;
  if (!(await select.isEnabled())) return false;
  if ((await select.getAttribute("aria-disabled")) === "true") return false;

  await select.click({ timeout: 2_000 });
  await page.waitForSelector('[role="option"]:visible', { timeout: 5_000 });

  const options = page.locator('[role="option"]:visible');
  const count = await options.count();
  for (let i = 0; i < count; i += 1) {
    const option = options.nth(i);
    const optionText = ((await option.textContent()) ?? "").trim();
    if (!pattern.test(optionText)) continue;

    await option.click();
    await page.waitForTimeout(TABLE_SETTLE);
    try {
      await page.waitForFunction(
        ({ cssSelector, expectedText }) => {
          const el = document.querySelector(cssSelector);
          return !!el && (el.textContent || "").includes(expectedText);
        },
        { cssSelector: selector, expectedText: optionText.replace(/\s*Auto\s*$/i, "") },
        { timeout: 5_000 },
      );
    } catch {
      // OptimusUI may render the selected label outside the host element.
    }
    return true;
  }

  await page.keyboard.press("Escape");
  return false;
}

async function prepareEventBreakdown(page) {
  const eventSelector = "#event-name-select";
  const propertySelector = "#property-key-select";

  if (!(await selectVisibleOptionMatching(page, eventSelector, /outbound_click/i)) && !(await selectFirstVisibleOption(page, eventSelector))) {
    console.warn("    ! Event selector could not pick a value");
    return false;
  }

  await waitForSelectEnabled(page, propertySelector);

  if (!(await selectFirstVisibleOption(page, propertySelector))) {
    console.warn("    ! Property selector could not pick a value");
    return false;
  }

  await page.waitForTimeout(CHART_SETTLE);
  return true;
}

async function prepareAIVisibilityShot(page) {
  const tables = page.locator('[data-testid="metric-card-group-correlation"]').first();
  if (await tables.count()) {
    // The caller restores the viewport: this clip is measured at this width, so
    // the shot has to be taken before anything reflows it.
    await page.setViewportSize(AI_CORRELATION_VIEWPORT);
    await tables.scrollIntoViewIfNeeded();
    await page.waitForTimeout(TABLE_SETTLE);

    const clip = await page.evaluate(() => {
      const tables = document.querySelector('[data-testid="metric-card-group-correlation"]');
      const card = document.querySelector('[data-testid="ai-visibility-fetch-depth"]');
      const grid = document.querySelector('[data-testid="metric-card-group"]');

      if (!tables || !card || !grid) {
        return null;
      }

      const gridRect = grid.getBoundingClientRect();
      const tablesRect = tables.getBoundingClientRect();
      const cardRect = card.getBoundingClientRect();

      const horizontalPadding = 18;
      const verticalPadding = 18;
      const scrollX = window.scrollX;
      const scrollY = window.scrollY;
      const absLeft = gridRect.left + scrollX;
      const absRight = gridRect.right + scrollX;
      const absTop = tablesRect.top + scrollY;
      const absBottom = cardRect.bottom + scrollY;
      const documentWidth = Math.max(document.documentElement.scrollWidth, document.body.scrollWidth);
      const documentHeight = Math.max(document.documentElement.scrollHeight, document.body.scrollHeight);

      const x = Math.max(absLeft - horizontalPadding, 0);
      const y = Math.max(absTop - verticalPadding, 0);
      const width = Math.max(Math.min(absRight + horizontalPadding, documentWidth) - x, 1);

      // Keep this asset landscape-oriented so it reads like a marketing hero instead of a document scan.
      const preferredHeight = Math.min(Math.floor(window.innerHeight * 0.76), 760);
      const maxBottom = Math.min(absBottom + verticalPadding, documentHeight);
      const height = Math.max(Math.min(maxBottom - y, preferredHeight), 1);

      return {
        x,
        y,
        width,
        height,
      };
    });

    if (clip) {
      return clip;
    }
  }

  await page.evaluate(() => window.scrollTo({ top: Math.floor(window.innerHeight * 1.2), behavior: "instant" }));
  await page.waitForTimeout(TABLE_SETTLE);
  return null;
}

async function captureRoute(page, record, slug, path, settle = TABLE_SETTLE) {
  await nav(page, path, settle);
  record(slug, await shoot(page, slug));
}

async function selectSiteByDomain(page, domain = DEMO_SITE_DOMAIN) {
  const anySelector = page.locator("app-site-selector").first();
  if (await anySelector.count()) {
    const currentText = ((await anySelector.textContent()) ?? "").trim();
    if (currentText.includes(domain)) {
      return true;
    }
  }

  const selector = page.locator("app-site-selector:visible").first();
  if (!(await selector.count())) {
    console.warn(`    ! Site selector not found while selecting ${domain}`);
    return false;
  }

  const currentText = ((await selector.textContent()) ?? "").trim();
  if (currentText.includes(domain)) {
    return true;
  }

  const combobox = selector.getByRole("combobox").first();
  if (!(await combobox.count())) {
    console.warn(`    ! Site combobox not found while selecting ${domain}`);
    return false;
  }

  await combobox.click({ timeout: 3_000 });
  const exactOption = page.getByRole("option", { name: new RegExp(`^${escapeRegExp(domain)}$`, "i") }).first();
  const fuzzyOption = page.getByRole("option", { name: new RegExp(escapeRegExp(domain), "i") }).first();
  const option = (await exactOption.count()) ? exactOption : fuzzyOption;
  if (!(await option.count())) {
    await page.keyboard.press("Escape");
    console.warn(`    ! Site option not found: ${domain}`);
    return false;
  }

  await option.click();
  await page.waitForFunction(
    (expectedDomain) => {
      const selector = document.querySelector("app-site-selector");
      return !!selector && (selector.textContent || "").includes(expectedDomain);
    },
    domain,
    { timeout: 8_000 },
  );
  await page.waitForTimeout(CHART_SETTLE);
  return true;
}

async function captureSearchConsoleDrilldown(page, record, slug) {
  await nav(page, "/dashboard", CHART_SETTLE);
  await selectSiteByDomain(page);
  await selectRangePreset(page, "7d", CHART_SETTLE);
  const drilldown = page.locator('[data-testid="search-console-drilldown"]').first();
  if (!(await drilldown.count())) {
    console.warn("    ! Search Console drilldown not found, skipping screenshot");
    return;
  }
  await drilldown.evaluate((el) => el.scrollIntoView({ behavior: "instant", block: "start" }));
  await page.waitForSelector('[data-testid="search-console-setup-prompt"]', { state: "detached", timeout: 8_000 }).catch(() => {});
  await page.getByText(/privacy friendly analytics|self hosted web analytics|cookie free analytics/i).first().waitFor({ state: "visible", timeout: 8_000 }).catch(() => {
    console.warn("    ! Search Console data rows were not visible before capture");
  });
  await page.waitForTimeout(TABLE_SETTLE);
  record(slug, await shoot(page, slug));
}

async function captureOpportunities(page, record, slug) {
  await nav(page, "/opportunities", TABLE_SETTLE);
  await selectSiteByDomain(page);
  await page.locator(".opportunities-inbox").first().waitFor({ state: "visible", timeout: 15_000 }).catch(() => {
    console.warn("    ! Opportunity inbox was not visible before capture");
  });
  const firstCard = page.locator("app-opportunity-card").first();
  await firstCard.waitFor({ state: "visible", timeout: 15_000 }).catch(() => {
    console.warn("    ! Opportunity cards were not visible before capture");
  });
  record(slug, await shoot(page, slug));
}

async function captureWebVitals(page, record) {
  await nav(page, "/web-vitals", CHART_SETTLE);
  record("analytics-web-vitals", await shoot(page, "analytics-web-vitals"));

  const poorButton = page.getByRole("button", { name: /poor/i }).first();
  if (!(await poorButton.count())) {
    console.warn("    ! Web Vitals poor rating control not found, skipping drilldown screenshot");
    return;
  }

  await poorButton.click();
  await page.getByRole("heading", { name: /poor .* pages/i }).waitFor({ state: "visible", timeout: 8_000 });
  await page.evaluate(() => {
    const heading = Array.from(document.querySelectorAll("h3")).find((el) => /poor .* pages/i.test(el.textContent || ""));
    heading?.scrollIntoView({ behavior: "instant", block: "start" });
  });
  await page.waitForTimeout(TABLE_SETTLE);
  record("analytics-web-vitals-rating-pages", await shoot(page, "analytics-web-vitals-rating-pages"));
}

async function captureGoogleSearchConsoleIntegration(page, record, slug) {
  await nav(page, "/integration/google-search-console", FORM_SETTLE);
  await selectSiteByDomain(page);
  await page.getByText(/sc-domain:acme-analytics\.io/i).first().waitFor({ state: "visible", timeout: 8_000 }).catch(() => {
    console.warn("    ! Mapped Search Console property was not visible before capture");
  });
  record(slug, await shoot(page, slug));
}

async function openAskAIDrawer(page) {
  await nav(page, "/dashboard", CHART_SETTLE);
  await selectSiteByDomain(page);
  if (!(await page.locator(".ask-ai-trigger:visible").first().count())) {
    const mobileMenu = page.getByRole("button", { name: /main sidebar/i }).first();
    if (await mobileMenu.count()) {
      await mobileMenu.click();
      await page.waitForTimeout(FORM_SETTLE);
    }
  }
  const trigger = page.locator(".ask-ai-trigger:visible").first();
  await trigger.waitFor({ state: "visible", timeout: 10_000 });
  await trigger.click();
  await page.locator(".ask-ai-drawer").waitFor({ state: "visible", timeout: 8_000 });
  await page.waitForTimeout(FORM_SETTLE);
}

async function captureAskAI(page, record) {
  await openAskAIDrawer(page);
  await page.locator(".ai-suggestion-row").waitFor({ state: "visible", timeout: 8_000 });
  record("feature-ask-ai-empty", await shoot(page, "feature-ask-ai-empty"));

  const prompt = "How many hits did I get from ChatGPT in the last 14 days?";
  await page.locator('input[name="ask-ai-panel-query"]').fill(prompt);
  await page.getByRole("button", { name: /send ask ai question/i }).click();
  await page.getByText(/In this demo site, ChatGPT sent 128 visits/i).waitFor({ state: "visible", timeout: 8_000 });
  await page.getByText(/Top demo ChatGPT paths/i).waitFor({ state: "visible", timeout: 8_000 });
  await page.waitForTimeout(FORM_SETTLE);
  record("feature-ask-ai-answer", await shoot(page, "feature-ask-ai-answer"));

  await page.setViewportSize(MOBILE_VIEWPORT);
  await openAskAIDrawer(page);
  await page.locator(".ai-suggestion-row").waitFor({ state: "visible", timeout: 8_000 });
  await page.waitForTimeout(FORM_SETTLE);
  record("feature-ask-ai-mobile", await shoot(page, "feature-ask-ai-mobile"));
  await page.setViewportSize(DESKTOP_VIEWPORT);
}

async function openTeamSwitcher(page) {
  const trigger = page.locator('[data-testid="team-switcher-trigger"]:visible').first();
  if (!(await trigger.count())) return false;
  try {
    await trigger.click({ timeout: 2_000 });
    await page.waitForTimeout(FORM_SETTLE);
    return true;
  } catch (err) {
    console.warn(`    ! Team switcher trigger could not be opened: ${err.message}`);
    return false;
  }
}

async function openCreateTeamDialog(page) {
  const trigger = page.locator('[data-testid="team-switcher-add"]:visible').first();
  if (!(await trigger.count())) return false;
  try {
    await trigger.click({ timeout: 2_000 });
    await page.waitForSelector(".p-dialog", { state: "visible", timeout: 8_000 });
    await page.waitForTimeout(FORM_SETTLE);
    return true;
  } catch (err) {
    console.warn(`    ! Create team dialog could not be opened: ${err.message}`);
    return false;
  }
}

async function captureTeamCustomDomains(page, record) {
  if (!(await clickTab(page, "custom domains", TABLE_SETTLE))) {
    console.warn("    ! Custom domains tab not found, skipping custom domain screenshots");
    return;
  }

  const section = page.locator("app-team-tracking-domains");
  const hasDomains = await section.locator("tbody tr").count();
  if (!hasDomains) {
    // Create one domain through the add dialog so the table has content;
    // a successful create opens the DNS setup dialog on its own.
    await section.locator("header").getByRole("button", { name: /add domain/i }).click();
    const addDialog = page.getByRole("dialog", { name: /add domain/i });
    await addDialog.getByPlaceholder("analytics.example.com").fill(`track.${DEMO_SITE_DOMAIN}`);
    await addDialog.getByRole("button", { name: /add domain/i }).click();
  } else {
    const rowActions = section.locator("app-table-row-actions button").first();
    if (await rowActions.count()) {
      await rowActions.click();
      await page.getByRole("menuitem", { name: /dns setup/i }).click();
    }
  }

  const setupDialog = page.getByRole("dialog", { name: /dns setup/i });
  await setupDialog.waitFor({ state: "visible", timeout: 8_000 }).catch(() => {});
  if (await setupDialog.count()) {
    await page.waitForTimeout(FORM_SETTLE);
    record("feature-custom-domain-setup", await shoot(page, "feature-custom-domain-setup"));
    await setupDialog.getByRole("button", { name: /cancel/i }).click();
    await page.waitForTimeout(300);
  }

  await page.waitForTimeout(TABLE_SETTLE);
  record("admin-team-custom-domains", await shoot(page, "admin-team-custom-domains"));
}

async function run() {
  console.log("\n  HitKeep Dashboard Screenshot Tool");
  console.log("  ────────────────────────────────");
  console.log(`  Instance : ${BASE_URL}`);
  console.log(`  Output   : ${OUTPUT_DIR}`);
  console.log(`  Scale    : ${SCALE}x\n`);

  mkdirSync(OUTPUT_DIR, { recursive: true });

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: DESKTOP_VIEWPORT,
    deviceScaleFactor: SCALE,
    colorScheme: "light",
  });

  const page = await context.newPage();
  page.on("console", () => {});
  page.on("pageerror", () => {});
  await installAskAIDemoRoutes(page);

  const results = [];
  const record = (slug, result) => {
    results.push({ slug, ...result });
    console.log(`    ${result.ok ? "✓" : "✗"} ${slug}${result.ok ? "" : ` — ${result.error}`}`);
  };

  try {
    if (SCREENSHOT_TARGET !== "ask-ai") {
      console.log("  Pre-auth:");
      await page.goto(`${BASE_URL}/login`, { waitUntil: "domcontentloaded" });
      await page.waitForSelector('input[type="password"]', { state: "visible", timeout: 10_000 });
      await page.waitForTimeout(FORM_SETTLE);
      record("page-login", await shoot(page, "page-login"));
    }

    console.log("\n  Authenticating...");
    await login(page);
    console.log("  ✓ Logged in\n");

    if (SCREENSHOT_TARGET === "ask-ai") {
      console.log("  Ask AI:");
      await captureAskAI(page, record);
    } else {
      console.log("  Dashboard:");
      await captureRoute(page, record, "dashboard-overview", "/dashboard", CHART_SETTLE);
      record("feature-onboarding-checklist", await shoot(page, "feature-onboarding-checklist"));

    if (await openTeamSwitcher(page)) {
      record("feature-team-switcher", await shoot(page, "feature-team-switcher"));
      await page.keyboard.press("Escape");
      await page.waitForTimeout(300);
    } else {
      console.warn("    ! Team switcher combobox not found, skipping switcher screenshot");
    }

    if (await openCreateTeamDialog(page)) {
      record("feature-create-team", await shoot(page, "feature-create-team"));
      await page.keyboard.press("Escape");
      await page.waitForTimeout(300);
    } else {
      console.warn("    ! Create team button not found, skipping dialog screenshot");
    }

    const shareBtn = page.getByRole("button", { name: /share dashboard/i }).first();
    if (await shareBtn.count()) {
      await shareBtn.click();
      await page.waitForSelector(".p-dialog", { state: "visible", timeout: 8_000 });
      await page.waitForTimeout(FORM_SETTLE);
      record("feature-share-dashboard", await shoot(page, "feature-share-dashboard"));
      await page.keyboard.press("Escape");
      await page.waitForTimeout(300);
    } else {
      console.warn("    ! Share dashboard button not found, skipping dialog screenshot");
    }

    const siteSettingsBtn = page.getByRole("button", { name: /site settings/i }).first();
    if (await siteSettingsBtn.count()) {
      await siteSettingsBtn.click();
      await page.getByRole("heading", { name: /site settings/i }).waitFor({ state: "visible", timeout: 8_000 });
      if (await clickTab(page, "tracking", FORM_SETTLE)) {
        await page.getByText(/live tracking verifier/i).first().waitFor({ state: "visible", timeout: 8_000 });
        record("feature-tracking-verifier", await shoot(page, "feature-tracking-verifier"));
        await page.getByText(/automatic event tracking/i).first().waitFor({ state: "visible", timeout: 8_000 });
        record("feature-site-tracking", await shoot(page, "feature-site-tracking"));
      }
      if (await clickTab(page, "team", FORM_SETTLE)) {
        await page.getByRole("heading", { name: /transfer site/i }).waitFor({ state: "visible", timeout: 8_000 });
        record("feature-site-transfer", await shoot(page, "feature-site-transfer"));
      }
      await page.keyboard.press("Escape");
      await page.waitForTimeout(300);
    } else {
      console.warn("    ! Site settings button not found, skipping team transfer screenshot");
    }

    console.log("\n  Analytics:");
    await captureRoute(page, record, "analytics-goals", "/goals", CHART_SETTLE);
    await captureRoute(page, record, "analytics-funnels", "/funnels", CHART_SETTLE);
    await captureRoute(page, record, "analytics-ecommerce", "/ecommerce", CHART_SETTLE);

    await nav(page, "/events", CHART_SETTLE);
    await prepareEventBreakdown(page);

    await page.evaluate(() => window.scrollTo({ top: 0, behavior: "instant" }));
    record("analytics-events", await shoot(page, "analytics-events"));

    await page.evaluate(() => {
      const audience = document.querySelector('[data-testid="events-audience"], .lg\\:grid-cols-4');
      if (audience) {
        audience.scrollIntoView({ behavior: "instant", block: "start" });
      } else {
        window.scrollTo({ top: Math.floor(window.innerHeight * 0.8), behavior: "instant" });
      }
    });
    await page.waitForTimeout(TABLE_SETTLE);
    record("analytics-events-audience", await shoot(page, "analytics-events-audience"));

    // One page now: the hero KPIs and chart at the top, then the one card grid
    // holding every breakdown, then the fetch depth card. All three shots come
    // from /ai-agents, the last two scrolled.
    await captureRoute(page, record, "analytics-ai-agents-traffic", "/ai-agents", CHART_SETTLE);

    await page.evaluate(() => {
      const target = document.querySelector('[data-testid="ai-agents-page-chips"], [data-testid="ai-agents-enrich-callout"]');
      if (target) {
        target.scrollIntoView({ behavior: "instant", block: "start" });
      } else {
        window.scrollTo({ top: document.documentElement.scrollHeight, behavior: "instant" });
      }
    });
    await page.waitForTimeout(TABLE_SETTLE);
    record("analytics-ai-visibility", await shoot(page, "analytics-ai-visibility"));
    const aiVisibilityClip = await prepareAIVisibilityShot(page);
    record("analytics-ai-visibility-correlation", await shoot(page, "analytics-ai-visibility-correlation", { clip: aiVisibilityClip ?? undefined }));
    await page.setViewportSize(DESKTOP_VIEWPORT);
    await captureRoute(page, record, "analytics-ai-chatbots", "/ai-chatbots", CHART_SETTLE);
    await captureWebVitals(page, record);
    await captureOpportunities(page, record, "analytics-opportunities");
    await captureRoute(page, record, "analytics-utm", "/utm", CHART_SETTLE);
    await captureRoute(page, record, "dashboard-comparison", "/dashboard", CHART_SETTLE);
    await captureSearchConsoleDrilldown(page, record, "analytics-search-console");
    await captureAskAI(page, record);

    console.log("\n  Settings:");
    await captureRoute(page, record, "settings-profile", "/settings", FORM_SETTLE);

    await page.evaluate(() => {
      const el = document.querySelector("app-settings-security");
      if (el) el.scrollIntoView({ behavior: "instant", block: "start" });
    });
    await page.waitForTimeout(400);
    record("security-2fa-setup", await shoot(page, "security-2fa-setup"));

    await captureRoute(page, record, "feature-email-reports", "/settings/reports", FORM_SETTLE);

    console.log("\n  Integrations:");
    await captureRoute(page, record, "security-api-clients", "/integration/api-clients", TABLE_SETTLE);
    await captureRoute(page, record, "integration-api-reference", "/integration/api-reference", TABLE_SETTLE);
    await captureGoogleSearchConsoleIntegration(page, record, "integration-google-search-console");

    console.log("\n  Mobile spot checks:");
    await page.setViewportSize(MOBILE_VIEWPORT);
    await captureGoogleSearchConsoleIntegration(page, record, "integration-google-search-console-mobile");
    await captureSearchConsoleDrilldown(page, record, "analytics-search-console-mobile");
    await page.setViewportSize(DESKTOP_VIEWPORT);

    console.log("\n  Admin:");
    await nav(page, "/admin/system", TABLE_SETTLE);
    let adminSitesCaptured = false;
    if (await clickTab(page, "sites")) {
      record("admin-sites-list", await shoot(page, "admin-sites-list"));
      adminSitesCaptured = true;
    }
    if (!adminSitesCaptured) {
      const sitesTabFallback = page.getByText(/^Sites$/).first();
      if (await sitesTabFallback.count()) {
        await sitesTabFallback.click();
        await page.waitForTimeout(TABLE_SETTLE);
        record("admin-sites-list", await shoot(page, "admin-sites-list"));
      } else {
        console.warn("    ! Sites tab not found, skipping admin sites screenshot");
      }
    }
    await nav(page, "/admin/status", TABLE_SETTLE);
    if (await clickTab(page, "activation", TABLE_SETTLE)) {
      await page.getByText(/user activation/i).first().waitFor({ state: "visible", timeout: 8_000 });
      record("admin-system-activation", await shoot(page, "admin-system-activation"));
    } else {
      console.warn("    ! Activation tab not found, skipping admin activation screenshot");
    }
    await captureRoute(page, record, "admin-team-overview", "/admin/team", TABLE_SETTLE);

    if (await clickTab(page, "members")) {
      record("admin-team-members", await shoot(page, "admin-team-members"));
    }
    if (await clickTab(page, "api clients", TABLE_SETTLE)) {
      record("admin-team-api-clients", await shoot(page, "admin-team-api-clients"));
    }
    await captureTeamCustomDomains(page, record);
    if (await clickTab(page, "branding", FORM_SETTLE)) {
      record("admin-team-settings", await shoot(page, "admin-team-settings"));
    }
    if (await clickTab(page, "activity", FORM_SETTLE)) {
      record("admin-team-audit", await shoot(page, "admin-team-audit"));
    }

      console.log("\n  Tools:");
      await captureRoute(page, record, "tools-utm-builder", "/utm/builder", FORM_SETTLE);
    }
  } finally {
    await browser.close();
  }

  const ok = results.filter((r) => r.ok);
  const failed = results.filter((r) => !r.ok);

  console.log(`\n  ✓ ${ok.length} screenshot(s) saved to:`);
  console.log(`    ${OUTPUT_DIR}`);

  if (failed.length) {
    console.log(`\n  ✗ ${failed.length} failed:`);
    failed.forEach((r) => console.log(`    - ${r.slug}: ${r.error}`));
  }

  console.log("\n  Usage in MDX:");
  console.log("    import img from '../../../../assets/screenshots/dashboard-overview.png'");
  console.log("    <Image src={img} alt=\"...\" />\n");

  if (failed.length) process.exit(1);
}

run().catch((err) => {
  console.error("Fatal:", err.message);
  process.exit(1);
});
