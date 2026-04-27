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
 */

import { chromium } from "playwright";
import { mkdirSync } from "fs";
import { join, resolve, dirname } from "path";
import { fileURLToPath } from "url";

const __dir = dirname(fileURLToPath(import.meta.url));

const BASE_URL = (process.env.HITKEEP_URL ?? "http://localhost:8080").replace(/\/$/, "");
const EMAIL = process.env.HITKEEP_EMAIL;
const PASSWORD = process.env.HITKEEP_PASSWORD;
const SCALE = parseFloat(process.env.SCALE ?? "2");
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
  try {
    await page.waitForLoadState("networkidle", { timeout: 7_000 });
  } catch {
    // Keep moving when background polling prevents networkidle.
  }
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

async function shoot(page, slug, { fullPage = false, clip } = {}) {
  const file = join(OUTPUT_DIR, `${slug}.png`);
  try {
    await page.screenshot({ path: file, fullPage, clip, animations: "disabled" });
    return { ok: true, file };
  } catch (err) {
    return { ok: false, file, error: err.message };
  }
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
    await page.waitForFunction(
      ({ cssSelector, expectedText }) => {
        const el = document.querySelector(cssSelector);
        return !!el && (el.textContent || "").includes(expectedText);
      },
      { cssSelector: selector, expectedText: optionText },
      { timeout: 5_000 },
    );
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
    await page.waitForFunction(
      ({ cssSelector, expectedText }) => {
        const el = document.querySelector(cssSelector);
        return !!el && (el.textContent || "").includes(expectedText);
      },
      { cssSelector: selector, expectedText: optionText.replace(/\s*Auto\s*$/i, "") },
      { timeout: 5_000 },
    );
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
  const shot = page.locator('[data-testid="ai-visibility-correlation-shot"]').first();
  if (await shot.count()) {
    const kpis = page.locator('[data-testid="ai-visibility-correlation-kpis"]').first();
    if (await kpis.count()) {
      await kpis.scrollIntoViewIfNeeded();
    } else {
      await shot.scrollIntoViewIfNeeded();
    }
    await page.waitForTimeout(TABLE_SETTLE);

    const clip = await page.evaluate(() => {
      const section = document.querySelector('[data-testid="ai-visibility-correlation-shot"]');
      const kpis = document.querySelector('[data-testid="ai-visibility-correlation-kpis"]');
      const tables = document.querySelector('[data-testid="ai-visibility-correlation-tables"]');

      if (!section || !kpis || !tables) {
        return null;
      }

      const sectionRect = section.getBoundingClientRect();
      const kpiRect = kpis.getBoundingClientRect();
      const tablesRect = tables.getBoundingClientRect();

      const horizontalPadding = 18;
      const verticalPadding = 18;
      const scrollX = window.scrollX;
      const scrollY = window.scrollY;
      const absLeft = sectionRect.left + scrollX;
      const absRight = sectionRect.right + scrollX;
      const absTop = kpiRect.top + scrollY;
      const absBottom = tablesRect.bottom + scrollY;
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

  await page.evaluate(() => {
    const correlationCard =
      Array.from(document.querySelectorAll("p-card, .p-card"))
        .find((el) => /fetch-to-visit correlation/i.test(el.textContent || ""));
    const tablesGrid =
      Array.from(document.querySelectorAll("p-table, .p-datatable"))
        .find((el) => /citation yield|opportunity pages|failure hotspots/i.test(el.textContent || ""));

    const target = tablesGrid || correlationCard;
    if (target) {
      target.scrollIntoView({ behavior: "instant", block: "start" });
    } else {
      window.scrollTo({ top: Math.floor(window.innerHeight * 1.2), behavior: "instant" });
    }
  });
  await page.waitForTimeout(TABLE_SETTLE);
  return null;
}

async function captureRoute(page, record, slug, path, settle = TABLE_SETTLE) {
  await nav(page, path, settle);
  record(slug, await shoot(page, slug));
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

async function run() {
  console.log("\n  HitKeep Dashboard Screenshot Tool");
  console.log("  ────────────────────────────────");
  console.log(`  Instance : ${BASE_URL}`);
  console.log(`  Output   : ${OUTPUT_DIR}`);
  console.log(`  Scale    : ${SCALE}x\n`);

  mkdirSync(OUTPUT_DIR, { recursive: true });

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1024 },
    deviceScaleFactor: SCALE,
    colorScheme: "light",
  });

  const page = await context.newPage();
  page.on("console", () => {});
  page.on("pageerror", () => {});

  const results = [];
  const record = (slug, result) => {
    results.push({ slug, ...result });
    console.log(`    ${result.ok ? "✓" : "✗"} ${slug}${result.ok ? "" : ` — ${result.error}`}`);
  };

  try {
    console.log("  Pre-auth:");
    await page.goto(`${BASE_URL}/login`, { waitUntil: "domcontentloaded" });
    await page.waitForSelector('input[type="password"]', { state: "visible", timeout: 10_000 });
    await page.waitForTimeout(FORM_SETTLE);
    record("page-login", await shoot(page, "page-login"));

    console.log("\n  Authenticating...");
    await login(page);
    console.log("  ✓ Logged in\n");

    console.log("  Dashboard:");
    await captureRoute(page, record, "dashboard-overview", "/dashboard", CHART_SETTLE);

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

    await nav(page, "/ai-visibility", CHART_SETTLE);
    await page.evaluate(() => window.scrollTo({ top: 0, behavior: "instant" }));
    await page.waitForTimeout(TABLE_SETTLE);
    record("analytics-ai-visibility", await shoot(page, "analytics-ai-visibility"));
    const aiVisibilityClip = await prepareAIVisibilityShot(page);
    record("analytics-ai-visibility-correlation", await shoot(page, "analytics-ai-visibility-correlation", { clip: aiVisibilityClip ?? undefined }));
    await captureRoute(page, record, "analytics-ai-chatbots", "/ai-chatbots", CHART_SETTLE);
    await captureRoute(page, record, "analytics-utm", "/utm", CHART_SETTLE);
    await captureRoute(page, record, "dashboard-comparison", "/dashboard", CHART_SETTLE);

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

    console.log("\n  Admin:");
    await captureRoute(page, record, "admin-users", "/admin", TABLE_SETTLE);
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
    await captureRoute(page, record, "admin-team-overview", "/admin/team", TABLE_SETTLE);

    if (await clickTab(page, "members")) {
      record("admin-team-members", await shoot(page, "admin-team-members"));
    }
    if (await clickTab(page, "settings", FORM_SETTLE)) {
      record("admin-team-settings", await shoot(page, "admin-team-settings"));
      await page.evaluate(() => {
        const apiClients = document.querySelector("app-settings-api-clients");
        if (apiClients) {
          apiClients.scrollIntoView({ behavior: "instant", block: "start" });
        }
      });
      await page.waitForTimeout(TABLE_SETTLE);
      record("admin-team-api-clients", await shoot(page, "admin-team-api-clients"));
    }
    if (await clickTab(page, "activity", FORM_SETTLE)) {
      record("admin-team-audit", await shoot(page, "admin-team-audit"));
    }

    console.log("\n  Tools:");
    await captureRoute(page, record, "tools-utm-builder", "/utm/builder", FORM_SETTLE);
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
