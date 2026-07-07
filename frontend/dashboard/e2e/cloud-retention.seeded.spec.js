const { test, expect } = require("playwright/test");
const { login } = require("./support/auth");

const TEAM_A_ID = "00000000-0000-0000-0000-00000000c0a1";
const TEAM_B_ID = "00000000-0000-0000-0000-00000000c0b2";

test("cloud free-plan notices dismiss per team and lead to the plan comparison", async ({ page }) => {
    let activeTeamID = TEAM_A_ID;

    await stubCloudFreeBootstrap(page, () => activeTeamID);

    await login(page, "/dashboard");

    // Team A sits well below its limits: the time-based retention notice shows.
    const notice = page.getByTestId("free-plan-retention-notice");
    await expect(notice).toContainText("Free plan data is retained for 60 days.");
    await expect(notice).toContainText("Upgrade to keep your full visitor history");

    await notice.getByRole("button", { name: "Dismiss retention notice" }).click();
    await expect(notice).toBeHidden();
    await expect.poll(() => page.evaluate((key) => window.localStorage.getItem(key), `hitkeep.freeRetentionNotice.dismissed.${TEAM_A_ID}`)).toBe("dismissed");

    // Team B has exhausted its site limit: the usage-triggered variant shows.
    activeTeamID = TEAM_B_ID;
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(notice).toContainText("Your team is using 3 of 3 sites on the Free plan.");

    // Usage pressure has its own dismissal and falls back to the retention notice.
    await notice.getByRole("button", { name: "Dismiss retention notice" }).click();
    await expect(notice).toContainText("Free plan data is retained for 60 days.");
    await expect.poll(() => page.evaluate((key) => window.localStorage.getItem(key), `hitkeep.freeUsageNotice.dismissed.${TEAM_B_ID}.sites`)).toBe("dismissed");

    // Team A's dismissal is remembered independently.
    activeTeamID = TEAM_A_ID;
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(notice).toBeHidden();

    // The single CTA leads to the plan comparison instead of a checkout redirect.
    activeTeamID = TEAM_B_ID;
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(notice).toBeVisible();
    await notice.getByRole("button", { name: "Upgrade to Pro" }).click();
    await page.waitForURL(/\/admin\/team\/overview$/);
});

async function stubCloudFreeBootstrap(page, activeTeamID) {
    await page.route("**/api/user/bootstrap", async (route) => {
        const response = await route.fetch();
        const bootstrap = await response.json();
        const sourceTeam = bootstrap.teams?.teams?.[0] || {};

        bootstrap.status = {
            ...(bootstrap.status || {}),
            cloud: {
                hosted: true,
                signup_enabled: false
            }
        };
        bootstrap.teams = {
            ...(bootstrap.teams || {}),
            active_team_id: activeTeamID(),
            teams: [cloudFreeTeam(sourceTeam, TEAM_A_ID, "Acme Analytics", 1), cloudFreeTeam(sourceTeam, TEAM_B_ID, "Beta Analytics", 3)]
        };

        await route.fulfill({ response, json: bootstrap });
    });
}

function cloudFreeTeam(sourceTeam, id, name, currentSites) {
    return {
        ...sourceTeam,
        id,
        name,
        role: "owner",
        usage: {
            current_sites: currentSites,
            current_members: 1,
            current_pending_invites: 0
        },
        entitlements: {
            max_sites_per_team: 3,
            max_team_members: 3,
            max_retention_days: 60,
            allow_sso: false,
            allow_custom_branding: false
        },
        plan: {
            code: "free",
            name: "Free"
        }
    };
}
