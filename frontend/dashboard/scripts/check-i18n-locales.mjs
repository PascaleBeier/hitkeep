import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(scriptDir, "..");
const localeDir = path.join(root, "public", "i18n");
const requiredKeys = [
    {
        path: "admin.system.health.ip2locationAttribution",
        includes: ["HitKeep", "IP2Location LITE"]
    }
];

if (!fs.existsSync(localeDir)) {
    throw new Error(`public/i18n not found at ${localeDir}.`);
}

const localeFiles = fs
    .readdirSync(localeDir)
    .filter((file) => file.endsWith(".json"))
    .sort((a, b) => a.localeCompare(b, "en", { sensitivity: "base" }));

if (localeFiles.length === 0) {
    throw new Error("No public/i18n locale files found.");
}

const failures = [];

for (const file of localeFiles) {
    const fullPath = path.join(localeDir, file);
    const locale = path.basename(file, ".json");
    const data = JSON.parse(fs.readFileSync(fullPath, "utf8"));

    for (const requirement of requiredKeys) {
        const value = readPath(data, requirement.path);
        if (typeof value !== "string" || value.trim() === "") {
            failures.push(`${locale}: missing ${requirement.path}`);
            continue;
        }
        for (const requiredText of requirement.includes) {
            if (!value.includes(requiredText)) {
                failures.push(`${locale}: ${requirement.path} must include ${requiredText}`);
            }
        }
    }
}

if (failures.length > 0) {
    throw new Error(`Locale check failed:\n${failures.map((failure) => `- ${failure}`).join("\n")}`);
}

console.log(`Checked ${localeFiles.length} public i18n locale files.`);

function readPath(value, dottedPath) {
    return dottedPath.split(".").reduce((current, part) => {
        if (current && typeof current === "object" && part in current) {
            return current[part];
        }
        return undefined;
    }, value);
}
