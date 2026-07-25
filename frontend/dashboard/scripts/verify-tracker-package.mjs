import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const dashboardDir = resolve(__dirname, "..");
const packageDir = resolve(dashboardDir, "../tracker");
const distDir = resolve(packageDir, "dist");

const failures = [];

function check(condition, message) {
    if (!condition) {
        failures.push(message);
    }
}

const buildResult = spawnSync(process.execPath, [resolve(__dirname, "build-tracker-package.mjs")], { stdio: "inherit" });
if (buildResult.status !== 0) {
    process.exit(buildResult.status ?? 1);
}

const expectedApi = ["init", "track", "trackPageview", "cleanup", "blockTrackingForMe", "enableTrackingForMe", "isTrackingEnabled", "trackViewItem", "trackAddToCart", "trackBeginCheckout", "trackPurchase", "TRACKER_VERSION"];

const esmModule = await import(pathToFileURL(resolve(distDir, "index.js")).href);
for (const name of expectedApi) {
    check(name in esmModule, `ESM bundle is missing export ${name}`);
}

const require = createRequire(import.meta.url);
const cjsModule = require(resolve(distDir, "index.cjs"));
for (const name of expectedApi) {
    check(name in cjsModule, `CJS bundle is missing export ${name}`);
}

const declarations = readFileSync(resolve(distDir, "package.d.ts"), "utf8");
for (const name of ["TrackerConfig", "TrackerHandle", "PurchaseProperties", "EcommerceItem", "init", "track"]) {
    check(declarations.includes(name), `package.d.ts is missing ${name}`);
}

const packResult = spawnSync("npm", ["pack", "--dry-run", "--json"], { cwd: packageDir, encoding: "utf8" });
if (packResult.status !== 0) {
    console.error(packResult.stderr);
    process.exit(packResult.status ?? 1);
}
const packOutput = JSON.parse(packResult.stdout);
const packReport = Array.isArray(packOutput) ? packOutput[0] : Object.values(packOutput)[0];
const packedFiles = packReport.files.map((file) => file.path).sort();
const expectedFiles = ["LICENSE", "README.md", "dist/core.d.cts", "dist/core.d.ts", "dist/index.cjs", "dist/index.js", "dist/package.d.cts", "dist/package.d.ts", "dist/version.d.cts", "dist/version.d.ts", "package.json"].sort();
check(JSON.stringify(packedFiles) === JSON.stringify(expectedFiles), `npm pack file list mismatch:\n  packed:   ${packedFiles.join(", ")}\n  expected: ${expectedFiles.join(", ")}`);

const packageVersion = JSON.parse(readFileSync(resolve(packageDir, "package.json"), "utf8")).version;
check(esmModule.TRACKER_VERSION === packageVersion, `TRACKER_VERSION ${esmModule.TRACKER_VERSION} does not match package version ${packageVersion}`);

// Type-check a synthetic consumer against the packed layout so exports-map or
// declaration-specifier regressions fail this gate instead of a user install.
const consumerDir = mkdtempSync(join(tmpdir(), "hitkeep-tracker-consumer-"));
try {
    mkdirSync(join(consumerDir, "node_modules", "@hitkeep"), { recursive: true });
    symlinkSync(packageDir, join(consumerDir, "node_modules", "@hitkeep", "tracker"), "dir");
    writeFileSync(join(consumerDir, "package.json"), `{"name":"hitkeep-tracker-consumer","private":true,"type":"module"}\n`);
    writeFileSync(
        join(consumerDir, "check.ts"),
        [
            "import { blockTrackingForMe, cleanup, init, isTrackingEnabled, track, trackPurchase, TRACKER_VERSION } from '@hitkeep/tracker';",
            "import type { PurchaseProperties, TrackerConfig, TrackerHandle } from '@hitkeep/tracker';",
            "const config: TrackerConfig = { host: 'https://stats.example.com', captureOnLocalhost: true };",
            "const handle: TrackerHandle = init(config);",
            "track('signup', { plan: 'pro' });",
            "const purchase: PurchaseProperties = { transaction_id: 'tx', value: 1, currency: 'EUR' };",
            "trackPurchase(purchase);",
            "blockTrackingForMe();",
            "const enabled: boolean = isTrackingEnabled();",
            "const version: string = TRACKER_VERSION;",
            "cleanup();",
            "console.log(enabled, version, handle);",
            ""
        ].join("\n")
    );
    const tscBin = resolve(dashboardDir, "node_modules", "typescript", "bin", "tsc");
    for (const moduleResolution of ["nodenext", "bundler"]) {
        const moduleKind = moduleResolution === "nodenext" ? "nodenext" : "esnext";
        writeFileSync(join(consumerDir, "tsconfig.json"), JSON.stringify({ compilerOptions: { strict: true, noEmit: true, target: "ES2022", module: moduleKind, moduleResolution, skipLibCheck: false }, include: ["check.ts"] }));
        const typecheck = spawnSync(process.execPath, [tscBin, "-p", join(consumerDir, "tsconfig.json")], { encoding: "utf8" });
        check(typecheck.status === 0, `consumer type-check failed with moduleResolution=${moduleResolution}:\n${typecheck.stdout}${typecheck.stderr}`);
    }
} finally {
    rmSync(consumerDir, { recursive: true, force: true });
}

if (failures.length > 0) {
    for (const failure of failures) {
        console.error(`verify:tracker-package: ${failure}`);
    }
    process.exit(1);
}

console.log(`@hitkeep/tracker ${packageVersion} verified: ESM/CJS exports, type declarations, and pack contents are consistent.`);
