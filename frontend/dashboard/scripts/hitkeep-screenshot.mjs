import { createHash } from "node:crypto";
import { chmod, mkdir, readFile, stat, writeFile } from "node:fs/promises";
import path from "node:path";

import { chromium } from "playwright";

const SCHEMA_VERSION = "hk.dev/screenshot/v1";
const VIEWPORTS = {
    desktop: { width: 1440, height: 1024 },
    mobile: { width: 390, height: 844 }
};

const options = parseArguments(process.argv.slice(2));
const startedAt = performance.now();
let browser;

try {
    validateOptions(options);
    await mkdir(options.outputDir, { recursive: true, mode: 0o700 });

    const launchStartedAt = performance.now();
    browser = await chromium.launch({ headless: true });
    const browserLaunchMS = elapsed(launchStartedAt);
    const viewport = VIEWPORTS[options.viewport];
    const context = await browser.newContext({
        viewport,
        deviceScaleFactor: options.scale,
        colorScheme: options.theme,
        reducedMotion: "reduce",
        locale: "en-US"
    });
    await context.addInitScript((theme) => {
        try {
            localStorage.setItem("hk_theme", theme);
        } catch {
            // Opaque startup documents do not expose localStorage.
        }
    }, options.theme);
    const page = await context.newPage();

    let authenticationMS = 0;
    if (!options.anonymous) {
        const authenticationStartedAt = performance.now();
        await login(page, options.baseURL, options.routes[0]);
        authenticationMS = elapsed(authenticationStartedAt);
    }

    const artifacts = [];
    for (const [index, route] of options.routes.entries()) {
        artifacts.push(await captureRoute(page, options, route, index, index === 0 && !options.anonymous));
    }

    const result = {
        schema_version: SCHEMA_VERSION,
        viewport: {
            name: options.viewport,
            width: viewport.width,
            height: viewport.height,
            scale: options.scale
        },
        theme: options.theme,
        full_page: options.fullPage,
        ...(options.selector ? { selector: options.selector } : {}),
        ...(options.anonymous ? { anonymous: true } : {}),
        manifest_path: options.manifest,
        artifacts,
        timings: {
            browser_launch_ms: browserLaunchMS,
            ...(options.anonymous ? {} : { authentication_ms: authenticationMS }),
            total_ms: elapsed(startedAt)
        }
    };
    await writeFile(options.manifest, `${JSON.stringify(result, null, 2)}\n`, { mode: 0o600 });
} catch (error) {
    process.stderr.write(`hitkeep-screenshot: ${safeErrorMessage(error)}\n`);
    process.exitCode = 1;
} finally {
    await browser?.close();
}

async function login(page, baseURL, returnRoute) {
    const email = process.env.HITKEEP_SCREENSHOT_EMAIL;
    const password = process.env.HITKEEP_SCREENSHOT_PASSWORD;
    if (!email || !password) {
        throw new Error("seeded development credentials are unavailable");
    }

    const loginURL = new URL("/login", baseURL);
    loginURL.searchParams.set("returnUrl", returnRoute);
    await page.goto(loginURL.href, { waitUntil: "domcontentloaded", timeout: 20_000 });
    await page.locator('input[type="email"], input[name="email"]').first().fill(email);
    await page.locator('input[type="password"]').first().fill(password);
    const loginResponse = page.waitForResponse((response) => response.request().method() === "POST" && new URL(response.url()).pathname.endsWith("/api/login"), { timeout: 10_000 });
    await page.locator('button[type="submit"]').first().click();
    const response = await loginResponse;
    if (!response.ok()) {
        throw new Error("seeded authentication failed; reset and seed the development workspace");
    }
    const outcome = await response.json();
    if (outcome.status !== "ok") {
        throw new Error(`seeded authentication requires unsupported ${outcome.status || "additional verification"}`);
    }
    await page.waitForURL((url) => !url.pathname.includes("/login"), { timeout: 5_000 });
}

async function captureRoute(page, options, route, index, reuseCurrentRoute) {
    const captureStartedAt = performance.now();
    let consoleErrorCount = 0;
    let pageErrorCount = 0;
    const countConsoleError = (message) => {
        if (message.type() === "error") consoleErrorCount += 1;
    };
    const countPageError = () => {
        pageErrorCount += 1;
    };
    page.on("console", countConsoleError);
    page.on("pageerror", countPageError);

    try {
        const target = new URL(route, options.baseURL);
        const current = new URL(page.url());
        const alreadyAtTarget = current.origin === target.origin && current.pathname === target.pathname && current.search === target.search;
        if (!reuseCurrentRoute || !alreadyAtTarget) {
            await page.goto(target.href, { waitUntil: "domcontentloaded", timeout: 20_000 });
        }
        if (!options.anonymous && new URL(page.url()).pathname.includes("/login")) {
            throw new Error("authentication failed; reset and seed the development workspace");
        }
        await waitForVisualReadiness(page, options.waitMS);
        await page.addStyleTag({
            content: "*,*::before,*::after{animation-delay:0s!important;animation-duration:0s!important;caret-color:transparent!important;scroll-behavior:auto!important;transition-delay:0s!important;transition-duration:0s!important}"
        });

        const filename = `${String(index + 1).padStart(2, "0")}-${routeSlug(route)}.png`;
        const outputPath = path.join(options.outputDir, filename);
        if (options.selector) {
            const subject = page.locator(options.selector).first();
            await subject.waitFor({ state: "visible", timeout: 10_000 });
            await subject.screenshot({ path: outputPath, animations: "disabled", caret: "hide" });
        } else {
            await page.screenshot({
                path: outputPath,
                fullPage: options.fullPage,
                animations: "disabled",
                caret: "hide"
            });
        }
        await chmod(outputPath, 0o600);

        const [raw, file] = await Promise.all([readFile(outputPath), stat(outputPath)]);
        const image = pngDimensions(raw);
        const finalURL = new URL(page.url());
        return {
            route,
            final_route: `${finalURL.pathname}${finalURL.search}`,
            path: outputPath,
            mime_type: "image/png",
            width: image.width,
            height: image.height,
            bytes: file.size,
            sha256: createHash("sha256").update(raw).digest("hex"),
            duration_ms: elapsed(captureStartedAt),
            ...(consoleErrorCount ? { console_error_count: consoleErrorCount } : {}),
            ...(pageErrorCount ? { page_error_count: pageErrorCount } : {})
        };
    } finally {
        page.off("console", countConsoleError);
        page.off("pageerror", countPageError);
    }
}

async function waitForVisualReadiness(page, waitMS) {
    await page.locator("app-root").waitFor({ state: "visible", timeout: 10_000 });
    await page
        .waitForFunction(() => !document.fonts || document.fonts.status === "loaded", undefined, {
            timeout: 3_000
        })
        .catch(() => {});
    await page
        .waitForFunction(() => Array.from(document.images).every((image) => image.complete), undefined, {
            timeout: 2_000
        })
        .catch(() => {});
    await page.evaluate(
        () =>
            new Promise((resolve) => {
                requestAnimationFrame(() => requestAnimationFrame(resolve));
            })
    );
    if (waitMS > 0) await page.waitForTimeout(waitMS);
}

function parseArguments(args) {
    const parsed = {
        routes: [],
        viewport: "desktop",
        theme: "light",
        scale: 1,
        waitMS: 200,
        fullPage: false,
        anonymous: false,
        selector: ""
    };
    for (let index = 0; index < args.length; index += 1) {
        const argument = args[index];
        switch (argument) {
            case "--anonymous":
                parsed.anonymous = true;
                break;
            case "--full-page":
                parsed.fullPage = true;
                break;
            case "--base-url":
                parsed.baseURL = requiredValue(args, ++index, argument);
                break;
            case "--manifest":
                parsed.manifest = path.resolve(requiredValue(args, ++index, argument));
                break;
            case "--output-dir":
                parsed.outputDir = path.resolve(requiredValue(args, ++index, argument));
                break;
            case "--route":
                parsed.routes.push(requiredValue(args, ++index, argument));
                break;
            case "--scale":
                parsed.scale = Number(requiredValue(args, ++index, argument));
                break;
            case "--selector":
                parsed.selector = requiredValue(args, ++index, argument);
                break;
            case "--theme":
                parsed.theme = requiredValue(args, ++index, argument);
                break;
            case "--viewport":
                parsed.viewport = requiredValue(args, ++index, argument);
                break;
            case "--wait-ms":
                parsed.waitMS = Number(requiredValue(args, ++index, argument));
                break;
            default:
                throw new Error(`unknown argument ${argument}`);
        }
    }
    return parsed;
}

function validateOptions(value) {
    if (!value.baseURL || !value.outputDir || !value.manifest || value.routes.length === 0) {
        throw new Error("base URL, output directory, manifest, and at least one route are required");
    }
    const baseURL = new URL(value.baseURL);
    if (baseURL.protocol !== "http:" || !["127.0.0.1", "localhost", "[::1]"].includes(baseURL.hostname)) {
        throw new Error("base URL must use loopback HTTP");
    }
    if (!VIEWPORTS[value.viewport] || !["light", "dark"].includes(value.theme)) {
        throw new Error("unknown viewport or theme");
    }
    if (![1, 2].includes(value.scale) || !Number.isInteger(value.waitMS) || value.waitMS < 0 || value.waitMS > 5000) {
        throw new Error("invalid scale or wait duration");
    }
}

function requiredValue(args, index, flag) {
    const value = args[index];
    if (!value) throw new Error(`${flag} requires a value`);
    return value;
}

function routeSlug(route) {
    const url = new URL(route, "http://hitkeep.local");
    const rawPathSlug =
        url.pathname
            .split("/")
            .filter(Boolean)
            .join("-")
            .replace(/[^a-zA-Z0-9_-]+/g, "-")
            .replace(/^-+|-+$/g, "") || "home";
    const pathSlug = rawPathSlug.slice(0, 100).replace(/-+$/g, "") || "home";
    if (!url.search && rawPathSlug.length <= 100) return pathSlug;
    return `${pathSlug}-${createHash("sha256").update(route).digest("hex").slice(0, 8)}`;
}

function pngDimensions(raw) {
    if (raw.length < 24 || raw.toString("ascii", 1, 4) !== "PNG") {
        throw new Error("captured artifact is not a PNG");
    }
    return { width: raw.readUInt32BE(16), height: raw.readUInt32BE(20) };
}

function elapsed(start) {
    return Math.round(performance.now() - start);
}

function safeErrorMessage(error) {
    if (error instanceof Error && error.message) return error.message.replace(/[\r\n]+/g, " ");
    return "unknown screenshot failure";
}
