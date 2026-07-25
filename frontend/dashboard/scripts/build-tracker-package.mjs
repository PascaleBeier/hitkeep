import { build } from "esbuild";
import { spawnSync } from "node:child_process";
import { mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const dashboardDir = resolve(__dirname, "..");
const packageDir = resolve(dashboardDir, "../tracker");
const distDir = resolve(packageDir, "dist");
const entryPoint = resolve(dashboardDir, "src/tracker/package.ts");

function readJson(path) {
    return JSON.parse(readFileSync(path, "utf8"));
}

// Release metadata drift across the version-bearing files is validated by the
// developer-docs gate (internal/devtool/docs.go); verify-tracker-package.mjs asserts
// the built bundle embeds this version. Only the local build inputs are compared here.
const dashboardVersion = readJson(resolve(dashboardDir, "package.json")).version;
const packageVersion = readJson(resolve(packageDir, "package.json")).version;

if (packageVersion !== dashboardVersion) {
    console.error(`@hitkeep/tracker version drift: dashboard package.json=${dashboardVersion} tracker package.json=${packageVersion}`);
    process.exit(1);
}

rmSync(distDir, { recursive: true, force: true });
mkdirSync(distDir, { recursive: true });

const sharedBuildOptions = {
    entryPoints: [entryPoint],
    bundle: true,
    target: "es2020",
    legalComments: "inline"
};

await Promise.all([build({ ...sharedBuildOptions, format: "esm", outfile: resolve(distDir, "index.js") }), build({ ...sharedBuildOptions, format: "cjs", outfile: resolve(distDir, "index.cjs") })]);

const tscBin = resolve(dashboardDir, "node_modules", "typescript", "bin", "tsc");
const tscResult = spawnSync(process.execPath, [tscBin, "-p", resolve(dashboardDir, "tsconfig.tracker-package.json")], { stdio: "inherit" });
if (tscResult.status !== 0) {
    process.exit(tscResult.status ?? 1);
}

// tsc emits extensionless relative specifiers, which nodenext consumers reject in
// declaration files; rewrite them per module flavor before shipping.
for (const entry of readdirSync(distDir)) {
    if (!entry.endsWith(".d.ts")) {
        continue;
    }
    const declaration = readFileSync(resolve(distDir, entry), "utf8");
    writeFileSync(resolve(distDir, entry), declaration.replace(/(from\s+')(\.\/[\w./-]+?)(?<!\.js)(';)/g, "$1$2.js$3"));
    writeFileSync(resolve(distDir, entry.replace(/\.d\.ts$/, ".d.cts")), declaration.replace(/(from\s+')(\.\/[\w./-]+?)(?<!\.cjs)(';)/g, "$1$2.cjs$3"));
}

console.log(`@hitkeep/tracker ${packageVersion} built to ${distDir}`);
