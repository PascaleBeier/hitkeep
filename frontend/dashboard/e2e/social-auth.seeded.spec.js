const { test, expect } = require("playwright/test");

const providers = [
    { id: "google", display_name: "Google" },
    { id: "github", display_name: "GitHub" },
    { id: "microsoft", display_name: "Microsoft" }
];

test("deterministic social provider double reaches the existing MFA flow", async ({ page, baseURL }) => {
    let startPayload = null;
    let completionPayload = null;
    const loginCompletionURL = new URL("/login#social_token=provider-double-token", baseURL).toString();
    const providerDoubleURL = new URL("/__e2e/social-provider/google", baseURL).toString();

    await page.route("**/api/auth/social/providers", (route) => route.fulfill({ json: { providers: [providers[0]], signup_enabled: false } }));
    await page.route("**/api/auth/social/google/start", async (route) => {
        startPayload = route.request().postDataJSON();
        await route.fulfill({ json: { auth_url: providerDoubleURL } });
    });
    await page.route("**/__e2e/social-provider/google", (route) =>
        route.fulfill({
            contentType: "text/html",
            body: `<script>location.replace(${JSON.stringify(loginCompletionURL)})</script>`
        })
    );
    await page.route("**/api/auth/social/preview", (route) =>
        route.fulfill({
            json: { provider: "google", display_name: "Google", observed_email: "user@example.com", email_verified: true, email_confirmation_required: false, flow: "login" }
        })
    );
    await page.route("**/api/auth/social/complete", async (route) => {
        completionPayload = route.request().postDataJSON();
        await route.fulfill({
            json: { status: "mfa_required", challenge_token: "provider-double-challenge", factors: ["totp"] }
        });
    });

    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await page.getByRole("button", { name: "Continue with Google" }).click();

    await expect(page.getByRole("heading", { name: "Two-factor verification" })).toBeVisible();
    expect(startPayload).toEqual({ flow: "login", return_url: "/", remember_me: false });
    expect(completionPayload).toEqual({ completion_token: "provider-double-token" });
});

test("social signup actions stay usable in a narrow dark layout with long labels", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.addInitScript(() => {
        localStorage.setItem("hk_theme", "dark");
        Object.defineProperty(navigator, "language", { get: () => "de-DE" });
        Object.defineProperty(navigator, "languages", { get: () => ["de-DE", "de"] });
    });
    await page.route("**/api/status", (route) =>
        route.fulfill({
            json: {
                needs_setup: false,
                version: "e2e",
                cloud: { hosted: true, signup_enabled: true, jurisdiction: "EU" }
            }
        })
    );
    await page.route("**/api/auth/social/providers", (route) => route.fulfill({ json: { providers, signup_enabled: true } }));

    await page.goto("/signup", { waitUntil: "domcontentloaded" });

    await expect(page.locator("html")).toHaveClass(/p-dark/);
    await expect(page.locator("html")).toHaveAttribute("lang", "de-DE");
    await expect(page.getByRole("button", { name: "Mit Google fortfahren" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Mit GitHub fortfahren" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Mit Microsoft fortfahren" })).toBeVisible();
    const jurisdiction = page.locator("p-fieldset");
    const socialMethods = page.locator("app-auth-methods");
    await expect(jurisdiction).toBeVisible();
    await expect(socialMethods).toBeVisible();
    const [jurisdictionBox, socialBox] = await Promise.all([jurisdiction.boundingBox(), socialMethods.boundingBox()]);
    expect(jurisdictionBox.y).toBeLessThan(socialBox.y);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
});
