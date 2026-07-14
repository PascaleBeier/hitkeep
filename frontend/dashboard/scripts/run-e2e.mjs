#!/usr/bin/env node

import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const dashboardDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const passthroughArgs = process.argv.slice(2);
const npx = process.platform === "win32" ? "npx.cmd" : "npx";
const basePort = Number(process.env.HITKEEP_E2E_PORT || 8098);
const htmlReportRoot = process.env.HITKEEP_E2E_HTML_REPORT || "playwright-report";
const outputRoot = process.env.HITKEEP_E2E_OUTPUT_DIR || "test-results";

class PlaywrightFailure extends Error {
    constructor(exitCode) {
        super(`Playwright exited with code ${exitCode}.`);
        this.exitCode = exitCode;
    }
}

try {
    if (passthroughArgs.length > 0) {
        await runPlaywright("Focused e2e run", passthroughArgs, process.env);
    } else {
        const tempDir = process.env.HITKEEP_E2E_BIN_PATH ? null : mkdtempSync(path.join(tmpdir(), "hitkeep-e2e-runner-"));
        const binPath = process.env.HITKEEP_E2E_BIN_PATH || path.join(tempDir, "hitkeep-e2e");

        try {
            await runPlaywright("Seeded dashboard suite", [], {
                ...process.env,
                HITKEEP_E2E_BIN_PATH: binPath,
                HITKEEP_E2E_HTML_REPORT: path.join(htmlReportRoot, "seeded"),
                HITKEEP_E2E_OUTPUT_DIR: path.join(outputRoot, "seeded")
            });

            await runPlaywright("Subdirectory deployment smoke", ["e2e/deployment.smoke.spec.js", "--workers=1"], {
                ...process.env,
                HITKEEP_E2E_BIN_PATH: binPath,
                HITKEEP_E2E_PORT: String(basePort + 1),
                HITKEEP_E2E_PUBLIC_PATH: "/hitkeep",
                HITKEEP_E2E_DAYS: "1",
                HITKEEP_E2E_SKIP_BUILD: "1",
                HITKEEP_E2E_HTML_REPORT: path.join(htmlReportRoot, "subdirectory"),
                HITKEEP_E2E_OUTPUT_DIR: path.join(outputRoot, "subdirectory")
            });
        } finally {
            if (tempDir) {
                rmSync(tempDir, { recursive: true, force: true });
            }
        }
    }
} catch (error) {
    if (error instanceof PlaywrightFailure) {
        process.exitCode = error.exitCode;
    } else {
        console.error(error);
        process.exitCode = 1;
    }
}

async function runPlaywright(label, args, env) {
    console.log(`\n[e2e] ${label}`);
    console.log(`[e2e] npx playwright test ${args.join(" ")}`.trimEnd());

    const code = await new Promise((resolve) => {
        const child = spawn(npx, ["playwright", "test", ...args], {
            cwd: dashboardDir,
            env,
            stdio: "inherit"
        });

        child.once("error", (error) => {
            console.error(`[e2e] ${error.message}`);
            resolve(1);
        });

        child.once("close", (exitCode, signal) => {
            if (signal) {
                console.error(`[e2e] Playwright terminated with signal ${signal}.`);
                resolve(1);
                return;
            }

            resolve(exitCode ?? 1);
        });
    });

    if (code !== 0) {
        throw new PlaywrightFailure(code);
    }
}
