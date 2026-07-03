const { test, expect } = require("playwright/test");
const { E2E_EMAIL, E2E_PASSWORD, expectAuthPage, login } = require("./support/auth");
const { inviteLinkFromMessage, startSmtpCapture } = require("./support/smtp");

test("protected routes redirect to login and return after authentication", async ({ page }) => {
    await page.goto("/events", { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(/\/login\?returnUrl=%2Fevents/);
    await expectAuthPage(page, "Sign in");

    await page.locator('input[type="email"], input[name="email"]').first().fill(E2E_EMAIL);
    await page.locator('input[type="password"]').first().fill(E2E_PASSWORD);
    await page.locator('button[type="submit"]').first().click();

    await expect(page).toHaveURL(/\/events(?:\?.*)?$/);
    await expect(page.getByText("Event activity")).toBeVisible();
});

test("login shows an error for invalid credentials", async ({ page }) => {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.locator('input[type="email"], input[name="email"]').first().fill(E2E_EMAIL);
    await page.locator('input[type="password"]').first().fill("definitely-wrong-password");
    await page.locator('button[type="submit"]').first().click();

    await expect(page).toHaveURL(/\/login/);
    await expect(page.getByRole("alert")).toContainText("Invalid email or password.");
});

test("signed-in users can sign out from the user menu", async ({ page }) => {
    await login(page, "/dashboard");
    await expect(page).toHaveURL(/\/dashboard(?:\?.*)?$/);

    await page.getByRole("button", { name: "Open user menu" }).click();
    await page.getByRole("menuitem", { name: "Sign out" }).click();

    await expect(page).toHaveURL(/\/login(?:\?.*)?$/);
    await expectAuthPage(page, "Sign in");
});

test("forgot password keeps the response generic for unknown accounts", async ({ page }) => {
    await page.goto("/forgot-password", { waitUntil: "domcontentloaded" });
    await expectAuthPage(page, "Reset password");

    await page.locator('input[type="email"]').first().fill("nobody@example.invalid");
    await page.locator('button[type="submit"]').first().click();

    await expect(page.getByRole("status")).toContainText("Check your inbox");
    await expect(page.getByText("If an account exists for nobody@example.invalid, we have sent a reset link.")).toBeVisible();
});

test("invite links open the invitation acceptance page for unauthenticated recipients", async ({ page }) => {
    await page.goto("/accept-invite?token=test-token", { waitUntil: "domcontentloaded" });

    await expect(page).toHaveURL(/\/accept-invite\?token=test-token/);
    await expectAuthPage(page, "Accept invitation");
});

test("placeholder invite links set a password and sign the invitee in", async ({ page }) => {
    const smtp = await startSmtpCapture();
    try {
        await login(page, "/dashboard");
        const team = await teamByName(page, "Northwind Studio");
        const email = `placeholder-invite-${Date.now()}@example.test`;

        await inviteTeamMember(page, team.id, email, "member");
        const message = await smtp.waitForMessage((emailMessage) => emailMessage.includes(email));
        const link = inviteLinkFromMessage(message);
        expect(new URL(link).pathname).toBe("/accept-invite");

        await logoutViaApi(page);
        await page.goto(link, { waitUntil: "domcontentloaded" });
        await expectAuthPage(page, "Accept invitation");

        await page.locator('input[type="password"]').first().fill("newDemoPwd!1");
        await page.locator('button[type="submit"]').first().click();

        await expect(page).toHaveURL(/\/dashboard(?:\?.*)?$/);
        await expect(page.getByRole("button", { name: /Open user menu/ })).toBeVisible();
    } finally {
        await smtp.close();
    }
});

test("existing user invite links use login and return to invite acceptance", async ({ page }) => {
    const smtp = await startSmtpCapture();
    try {
        await login(page, "/dashboard");
        const team = await teamByName(page, "Northwind Studio");
        const email = "bob@devtools.co";

        await inviteTeamMember(page, team.id, email, "member");
        const message = await smtp.waitForMessage((emailMessage) => emailMessage.includes(email));
        const link = inviteLinkFromMessage(message);
        const url = new URL(link);
        expect(url.pathname).toBe("/login");
        expect(url.searchParams.get("returnUrl")).toMatch(/^\/accept-invite\?token=/);

        await logoutViaApi(page);
        await page.goto(link, { waitUntil: "domcontentloaded" });
        await expectAuthPage(page, "Sign in");

        await page.locator('input[type="email"], input[name="email"]').first().fill(email);
        await page.locator('input[type="password"]').first().fill("demoPwd!1");
        await page.locator('button[type="submit"]').first().click();

        await expect(page).toHaveURL(/\/dashboard(?:\?.*)?$/);
        await expect(page.getByRole("button", { name: /Open user menu/ })).toBeVisible();
    } finally {
        await smtp.close();
    }
});

test("invalid mfa email links bounce back to login with a clear error", async ({ page }) => {
    const response = await page.request.get("/api/auth/mfa/email-link/verify?token=not-a-valid-token", {
        maxRedirects: 0
    });
    expect(response.status()).toBe(303);
    expect(response.headers().location).toContain("/login?error=mfa_link_invalid");

    await page.goto("/login?error=mfa_link_invalid", {
        waitUntil: "domcontentloaded"
    });

    await expectAuthPage(page, "Sign in");
    await expect(page.getByRole("alert")).toContainText("This sign-in link is invalid or has expired.");
});

async function teamByName(page, name) {
    const response = await page.request.get("/api/user/teams");
    expect(response.ok()).toBeTruthy();
    const body = await response.json();
    const team = body.teams.find((candidate) => candidate.name === name);
    expect(team, `expected seeded team ${name}`).toBeTruthy();
    return team;
}

async function inviteTeamMember(page, teamId, email, role) {
    const response = await page.request.post(`/api/user/teams/${teamId}/members`, {
        headers: sameOriginHeaders(page),
        data: { email, role }
    });
    const responseBody = await response.text();
    expect(response.ok(), `invite ${email} to team ${teamId} failed with ${response.status()}: ${responseBody}`).toBeTruthy();
    const body = JSON.parse(responseBody);
    expect(body.is_invite).toBe(true);
    return body.invite;
}

async function logoutViaApi(page) {
    await page.request.post("/api/logout", {
        headers: sameOriginHeaders(page)
    });
}

function sameOriginHeaders(page) {
    const origin = new URL(page.url()).origin;
    return {
        Origin: origin,
        Referer: page.url()
    };
}
