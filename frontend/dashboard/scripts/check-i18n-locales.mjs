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
const requiredNamespaces = ["integration.webhooks"];
const forbiddenPaths = ["password.webhooks"];

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
const englishPath = path.join(localeDir, "en.json");
if (!fs.existsSync(englishPath)) {
    throw new Error("English locale file is required as the namespace reference.");
}
const english = JSON.parse(fs.readFileSync(englishPath, "utf8"));

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

    for (const namespacePath of requiredNamespaces) {
        const reference = readPath(english, namespacePath);
        const namespace = readPath(data, namespacePath);
        if (!reference || typeof reference !== "object") {
            throw new Error(`English locale is missing required namespace ${namespacePath}.`);
        }
        if (!namespace || typeof namespace !== "object") {
            failures.push(`${locale}: missing ${namespacePath}`);
            continue;
        }

        for (const [relativePath, referenceValue] of stringLeaves(reference)) {
            const fullPath = `${namespacePath}.${relativePath}`;
            const value = readPath(data, fullPath);
            if (typeof value !== "string" || value.trim() === "") {
                failures.push(`${locale}: missing ${fullPath}`);
                continue;
            }
            const referenceParams = interpolationParams(referenceValue);
            const localeParams = interpolationParams(value);
            if (referenceParams.join(",") !== localeParams.join(",")) {
                failures.push(`${locale}: ${fullPath} interpolation must match ${referenceParams.join(", ") || "none"}`);
            }
        }
    }

    for (const forbiddenPath of forbiddenPaths) {
        if (readPath(data, forbiddenPath) !== undefined) {
            failures.push(`${locale}: misplaced namespace ${forbiddenPath}`);
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

function stringLeaves(value, prefix = "") {
    return Object.entries(value).flatMap(([key, child]) => {
        const childPath = prefix ? `${prefix}.${key}` : key;
        if (typeof child === "string") {
            return [[childPath, child]];
        }
        return child && typeof child === "object" ? stringLeaves(child, childPath) : [];
    });
}

function interpolationParams(value) {
    return [...value.matchAll(/\{\{\s*([\w.-]+)\s*\}\}/g)].map((match) => match[1]).sort();
}
