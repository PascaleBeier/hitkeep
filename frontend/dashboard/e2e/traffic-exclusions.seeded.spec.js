const { randomUUID } = require("node:crypto");
const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

test("team path exclusions suppress traffic and appear as inherited at the site", async ({ page }) => {
    const token = randomUUID().slice(0, 8);
    const rulePath = `/e2e-traffic-exclusion-${token}`;
    const controlPath = `/e2e-traffic-control-${token}`;
    const description = `E2E traffic exclusion ${token}`;
    let teamID = "";
    let site = null;

    await login(page, "/admin/team/exclusions");

    try {
        const teamsResponse = await page.request.get("/api/user/teams");
        expect(teamsResponse.ok()).toBeTruthy();
        const teams = await teamsResponse.json();
        teamID = teams.active_team_id;
        expect(teamID).toBeTruthy();

        const siteResponse = await page.request.post("/api/sites", {
            data: { domain: `traffic-${token}.example.test` }
        });
        site = await siteResponse.json();
        expect(siteResponse.ok()).toBeTruthy();

        await page
            .getByRole("button", { name: /Add team filter/ })
            .first()
            .click();
        const dialog = page.getByRole("dialog", { name: "Add team filter" });
        await dialog.getByRole("combobox", { name: "IP/CIDR" }).click();
        await page.getByRole("option", { name: "Path", exact: true }).click();
        await dialog.getByRole("textbox", { name: "Path" }).fill(rulePath);
        await dialog.getByRole("textbox", { name: "Description" }).fill(description);
        await dialog.getByRole("button", { name: /Add team filter/ }).click();
        await expect(page.getByRole("cell", { name: description })).toBeVisible();

        const rule = await findTeamRule(page, teamID, description);
        expect(rule).toBeTruthy();

        await page.goto(`/sites/${site.id}/settings/filtering`);
        const inheritedRow = page.getByRole("cell", { name: description }).locator("..");
        await expect(inheritedRow).toContainText("Team");
        await expect(inheritedRow).toContainText("Inherited");

        expect((await sendPageview(page, site.domain, rulePath)).status()).toBe(202);
        expect((await sendPageview(page, site.domain, controlPath)).status()).toBe(202);

        await expect.poll(() => hitExists(page, site.id, controlPath), { timeout: 15_000 }).toBe(true);
        expect(await hitExists(page, site.id, rulePath)).toBe(false);

        const deleteResponse = await page.request.delete(`/api/user/teams/${teamID}/exclusions/${rule.id}`);
        expect(deleteResponse.status()).toBe(204);

        expect((await sendPageview(page, site.domain, rulePath)).status()).toBe(202);
        await expect.poll(() => hitExists(page, site.id, rulePath), { timeout: 15_000 }).toBe(true);
    } finally {
        if (teamID) {
            const remaining = await findTeamRule(page, teamID, description);
            if (remaining) {
                await page.request.delete(`/api/user/teams/${teamID}/exclusions/${remaining.id}`);
            }
        }
        if (site?.id) {
            await page.request.delete(`/api/sites/${site.id}`);
        }
    }
});

async function findTeamRule(page, teamID, description) {
    const response = await page.request.get(`/api/user/teams/${teamID}/exclusions`);
    if (!response.ok()) {
        return null;
    }
    const rules = await response.json();
    return rules.find((rule) => rule.description === description) ?? null;
}

async function sendPageview(page, domain, path) {
    return page.request.post("/ingest", {
        headers: {
            Origin: `https://${domain}`,
            "Content-Type": "application/json"
        },
        data: {
            path,
            ua: "HitKeep traffic exclusion e2e",
            session_id: randomUUID(),
            page_id: randomUUID(),
            tsrc: "e2e",
            tv: "traffic-exclusions"
        }
    });
}

async function hitExists(page, siteID, path) {
    const now = Date.now();
    const response = await page.request.get(`/api/sites/${siteID}/hits`, {
        params: {
            from: new Date(now - 86_400_000).toISOString().slice(0, 10),
            to: new Date(now + 86_400_000).toISOString().slice(0, 10),
            limit: "100",
            offset: "0",
            q: path
        }
    });
    if (!response.ok()) {
        return false;
    }
    const hits = await response.json();
    return (hits.data ?? []).some((hit) => hit.path === path);
}
