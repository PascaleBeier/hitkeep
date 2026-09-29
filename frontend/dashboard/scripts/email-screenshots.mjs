// Screenshots rendered email previews (cmd/email-preview output) for review.
// With --base, the same shots are taken from the base render and compared
// pixel by pixel in the browser, producing before/after/diff images.
//
//   node scripts/email-screenshots.mjs --head <preview-dir> [--base <preview-dir>] --out <dir>
//
// This is a review aid, not a gate: HTML golden files in mailpreview catch
// regressions; these images show reviewers what a change looks like.
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";

import { chromium } from "playwright";

const SHOTS = [
    { locale: "en", width: 600, theme: "light" },
    { locale: "en", width: 600, theme: "dark" },
    { locale: "en", width: 375, theme: "light" },
    { locale: "de", width: 375, theme: "light" }
];
// Per-channel difference below this is antialiasing noise, not a change.
const CHANNEL_THRESHOLD = 24;

const options = parseArguments(process.argv.slice(2));
const head = await readManifest(options.head);
const base = options.base && existsSync(path.join(options.base, "manifest.json")) ? await readManifest(options.base) : null;
await rm(options.out, { recursive: true, force: true });
await mkdir(options.out, { recursive: true });

const browser = await chromium.launch({ headless: true });
const results = [];
try {
    const pages = {};
    for (const theme of ["light", "dark"]) {
        const context = await browser.newContext({
            colorScheme: theme,
            deviceScaleFactor: 1,
            reducedMotion: "reduce"
        });
        pages[theme] = await context.newPage();
    }
    const comparer = await (await browser.newContext()).newPage();

    const fixtures = unique([...head.keys(), ...(base ? base.keys() : [])].map((key) => key.split("/")[0]));
    for (const fixture of fixtures) {
        for (const shot of SHOTS) {
            const key = `${fixture}/${shot.locale}`;
            const name = `${fixture}/${shot.locale}-${shot.width}-${shot.theme}.png`;
            const result = { fixture, ...shot, file: name, status: "unchanged" };
            const headEntry = head.get(key);
            const baseEntry = base?.get(key);
            if (headEntry) {
                await capture(pages[shot.theme], path.join(options.head, headEntry.html_file), shot.width, path.join(options.out, "head", name));
            }
            if (baseEntry) {
                await capture(pages[shot.theme], path.join(options.base, baseEntry.html_file), shot.width, path.join(options.out, "base", name));
            }
            if (!base) {
                result.status = "current";
            } else if (!baseEntry) {
                result.status = "added";
            } else if (!headEntry) {
                result.status = "removed";
            } else {
                const diff = await compare(comparer, path.join(options.out, "base", name), path.join(options.out, "head", name));
                if (diff.changedPixels > 0) {
                    result.status = "changed";
                    result.changedPixels = diff.changedPixels;
                    await writeImage(path.join(options.out, "diff", name), diff.dataURL);
                }
            }
            results.push(result);
        }
    }
} finally {
    await browser.close();
}

const summary = {
    compared: Boolean(base),
    counts: countBy(results, (result) => result.status),
    shots: results
};
await writeFile(path.join(options.out, "summary.json"), JSON.stringify(summary, null, 2));
await writeFile(path.join(options.out, "index.html"), renderIndex(summary));
console.log(
    JSON.stringify({
        out: options.out,
        compared: summary.compared,
        counts: summary.counts
    })
);

async function capture(page, htmlFile, width, output) {
    await page.setViewportSize({ width, height: 800 });
    await page.goto(pathToFileURL(htmlFile).href, { waitUntil: "networkidle" });
    await page.evaluate(() => document.fonts.ready);
    await mkdir(path.dirname(output), { recursive: true });
    await page.screenshot({
        path: output,
        fullPage: true,
        animations: "disabled"
    });
}

async function compare(page, beforeFile, afterFile) {
    const before = await dataURL(beforeFile);
    const after = await dataURL(afterFile);
    return page.evaluate(
        async ({ before, after, threshold }) => {
            const load = (src) =>
                new Promise((resolve, reject) => {
                    const image = new Image();
                    image.onload = () => resolve(image);
                    image.onerror = reject;
                    image.src = src;
                });
            const [a, b] = await Promise.all([load(before), load(after)]);
            const width = Math.max(a.width, b.width);
            const height = Math.max(a.height, b.height);
            const pixels = (image) => {
                const canvas = new OffscreenCanvas(width, height);
                const context = canvas.getContext("2d");
                context.fillStyle = "#fff";
                context.fillRect(0, 0, width, height);
                context.drawImage(image, 0, 0);
                return context.getImageData(0, 0, width, height).data;
            };
            const pa = pixels(a);
            const pb = pixels(b);
            const canvas = new OffscreenCanvas(width, height);
            const context = canvas.getContext("2d");
            const out = context.createImageData(width, height);
            let changedPixels = 0;
            for (let i = 0; i < pa.length; i += 4) {
                const delta = Math.max(Math.abs(pa[i] - pb[i]), Math.abs(pa[i + 1] - pb[i + 1]), Math.abs(pa[i + 2] - pb[i + 2]));
                if (delta > threshold) {
                    changedPixels++;
                    out.data.set([230, 30, 90, 255], i);
                } else {
                    const gray = 235 + (pb[i] + pb[i + 1] + pb[i + 2]) / 3 / 15;
                    out.data.set([gray, gray, gray, 255], i);
                }
            }
            if (changedPixels === 0) return { changedPixels };
            context.putImageData(out, 0, 0);
            const blob = await canvas.convertToBlob({ type: "image/png" });
            const bytes = new Uint8Array(await blob.arrayBuffer());
            let binary = "";
            for (const byte of bytes) binary += String.fromCharCode(byte);
            return {
                changedPixels,
                dataURL: "data:image/png;base64," + btoa(binary)
            };
        },
        { before, after, threshold: CHANNEL_THRESHOLD }
    );
}

async function dataURL(file) {
    return "data:image/png;base64," + (await readFile(file)).toString("base64");
}

async function writeImage(file, url) {
    await mkdir(path.dirname(file), { recursive: true });
    await writeFile(file, Buffer.from(url.split(",")[1], "base64"));
}

async function readManifest(dir) {
    const entries = JSON.parse(await readFile(path.join(dir, "manifest.json"), "utf8"));
    return new Map(entries.map((entry) => [`${entry.fixture}/${entry.locale}`, entry]));
}

function renderIndex(summary) {
    const escape = (value) => String(value).replace(/[&<>"]/g, (char) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[char]);
    const label = (shot) => `${escape(shot.fixture)} · ${shot.locale} · ${shot.width}px · ${shot.theme}`;
    const img = (dir, shot) => `<a href="${dir}/${escape(shot.file)}"><img loading="lazy" src="${dir}/${escape(shot.file)}" alt="${dir} ${label(shot)}"></a>`;
    const groups = [
        ["changed", "Changed", (shot) => `<figure><figcaption>${label(shot)} — ${shot.changedPixels} px</figcaption><div class="row">${img("base", shot)}${img("head", shot)}${img("diff", shot)}</div></figure>`],
        ["added", "Added", (shot) => `<figure><figcaption>${label(shot)}</figcaption>${img("head", shot)}</figure>`],
        ["removed", "Removed", (shot) => `<figure><figcaption>${label(shot)}</figcaption>${img("base", shot)}</figure>`],
        ["current", "Screenshots", (shot) => `<figure><figcaption>${label(shot)}</figcaption>${img("head", shot)}</figure>`],
        ["unchanged", "Unchanged", (shot) => `<figure><figcaption>${label(shot)}</figcaption>${img("head", shot)}</figure>`]
    ];
    const sections = groups
        .map(([status, title, render]) => {
            const shots = summary.shots.filter((shot) => shot.status === status);
            if (shots.length === 0) return "";
            const body = `<div class="grid">${shots.map(render).join("")}</div>`;
            return status === "unchanged" ? `<details><summary><h2>${title} (${shots.length})</h2></summary>${body}</details>` : `<section><h2>${title} (${shots.length})</h2>${body}</section>`;
        })
        .join("");
    return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Email visual review</title>
<style>
:root { color-scheme: light dark; --bg: #f8fafc; --fg: #0f172a; --muted: #64748b; --line: #e2e8f0; }
@media (prefers-color-scheme: dark) { :root { --bg: #0b1120; --fg: #e2e8f0; --muted: #94a3b8; --line: #1e293b; } }
body { margin: 0; padding: 16px; font: 14px/1.5 system-ui, sans-serif; background: var(--bg); color: var(--fg); }
h1 { font-size: 18px; } h2 { font-size: 15px; display: inline; }
section, details { margin: 24px 0; }
.grid { display: flex; flex-wrap: wrap; gap: 24px; margin-top: 12px; }
figure { margin: 0; } figcaption { color: var(--muted); font-size: 12px; margin-bottom: 4px; }
.row { display: flex; gap: 8px; }
img { display: block; max-width: 320px; border: 1px solid var(--line); }
</style></head><body>
<h1>Email visual review</h1>
<p>${summary.compared ? "Before (base) · after (head) · diff for every changed screenshot." : "No base render; showing current screenshots."}</p>
${sections}
</body></html>
`;
}

function parseArguments(args) {
    const parsed = {};
    for (let i = 0; i < args.length; i += 2) {
        const key = args[i]?.replace(/^--/, "");
        if (!["head", "base", "out"].includes(key) || args[i + 1] === undefined) {
            usage();
        }
        parsed[key] = path.resolve(args[i + 1]);
    }
    if (!parsed.head || !parsed.out) usage();
    return parsed;
}

function usage() {
    console.error("usage: node scripts/email-screenshots.mjs --head <preview-dir> [--base <preview-dir>] --out <dir>");
    process.exit(2);
}

function unique(values) {
    return [...new Set(values)];
}

function countBy(values, key) {
    const counts = {};
    for (const value of values) counts[key(value)] = (counts[key(value)] ?? 0) + 1;
    return counts;
}
