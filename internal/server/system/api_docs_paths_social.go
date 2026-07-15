package system

func openAPIV1SocialAuthPaths() map[string]any {
	providerParam := map[string]any{
		"name": "provider", "in": "path", "required": true,
		"schema": map[string]any{"type": "string", "enum": []string{"google", "github", "microsoft"}},
	}
	return map[string]any{
		"/api/auth/social/providers": map[string]any{
			"get": op([]string{"Auth"}, "List social sign-in providers", "Returns only completely configured social providers. Open signup is reported separately and may be disabled while login, invitations, and account linking remain available.", nil, nil, nil,
				map[string]any{"200": jsonRefResp("Social provider availability", "#/components/schemas/SocialProvidersResponse")}),
		},
		"/api/auth/social/{provider}/start": map[string]any{
			"post": op([]string{"Auth"}, "Start social authentication", "Starts a login, signup, or invitation flow using authorization code, browser-bound one-time state, PKCE S256, and an OIDC nonce where applicable. Return URLs are restricted to safe local application paths.", nil, []any{providerParam},
				jsonBody(map[string]any{"$ref": "#/components/schemas/SocialStartRequest"}),
				map[string]any{"200": jsonRefResp("Provider authorization URL", "#/components/schemas/SSOStartResponse"), "400": errResp("Invalid flow"), "404": errResp("Provider or social signup unavailable"), "503": errResp("Provider unavailable")}),
		},
		"/api/auth/social/{provider}/callback": map[string]any{
			"get": op([]string{"Auth"}, "Validate social provider callback", "Consumes the state once, exchanges the code, validates the immutable provider identity and required claims, discards provider tokens, and redirects with a short-lived completion token in the URL fragment. Upstream responses are never exposed.", nil,
				[]any{providerParam, map[string]any{"name": "state", "in": "query", "required": true, "schema": map[string]any{"type": "string"}}, map[string]any{"name": "code", "in": "query", "required": false, "schema": map[string]any{"type": "string"}}, map[string]any{"name": "error", "in": "query", "required": false, "schema": map[string]any{"type": "string"}}}, nil,
				map[string]any{"303": desc("Redirects to the bound local flow with a fragment completion token or stable error code")}),
		},
		"/api/auth/social/preview": map[string]any{
			"post": op([]string{"Auth"}, "Preview social completion", "Returns safe provider and observed-email metadata for a still-valid completion token. Provider subjects are never returned.", nil, nil,
				jsonBody(map[string]any{"$ref": "#/components/schemas/SocialCompleteRequest"}),
				map[string]any{"200": jsonRefResp("Social completion preview", "#/components/schemas/SocialPreviewResponse"), "409": errResp("Completion token invalid, expired, or already consumed")}),
		},
		"/api/auth/social/complete": map[string]any{
			"post": op([]string{"Auth"}, "Complete social login", "Consumes a short-lived completion token once, resolves or safely links the immutable identity, accepts a bound invitation when applicable, and returns the standard login or MFA-required shape. Microsoft first links require HitKeep email confirmation unless an authenticated session or valid invitation proves the account.", nil, nil,
				jsonBody(map[string]any{"$ref": "#/components/schemas/SocialCompleteRequest"}),
				map[string]any{"200": jsonRefResp("Social login result", "#/components/schemas/SocialCompleteResponse"), "400": errResp("Invalid flow or target email"), "403": errResp("Account unavailable"), "409": errResp("Completion replay or identity conflict")}),
		},
		"/api/auth/social/confirm": map[string]any{
			"get": op([]string{"Auth"}, "Confirm Microsoft social identity", "Consumes a hashed one-time email confirmation token for a Microsoft account link or signup, then redirects without exposing provider identity data.", nil,
				[]any{map[string]any{"name": "token", "in": "query", "required": true, "schema": map[string]any{"type": "string"}}}, nil,
				map[string]any{"303": desc("Redirects to sign-in, onboarding, checkout, or a stable error path")}),
		},
		"/api/cloud/signup/social/complete": map[string]any{
			"post": cloudOp("Complete managed social signup", "Creates a hosted cloud user and team after provider authentication, preserving the existing plan, billing-intent, locale, region, Terms, and onboarding behavior. Google and GitHub verified email skip HitKeep email verification; Microsoft returns verification_sent.", nil, nil,
				jsonBody(map[string]any{"$ref": "#/components/schemas/SocialSignupCompleteRequest"}),
				map[string]any{"201": jsonRefResp("Social signup result", "#/components/schemas/SocialSignupCompleteResponse"), "400": errResp("Invalid signup intent"), "404": errResp("Social signup disabled"), "409": errResp("Account or identity already exists")}),
		},
		"/api/user/security/social/{provider}/start": map[string]any{
			"post": op([]string{"User"}, "Start authenticated social link", "Starts a provider authorization bound to the authenticated HitKeep account.", secCookie(), []any{providerParam}, jsonBody(map[string]any{"type": "object"}),
				map[string]any{"200": jsonRefResp("Provider authorization URL", "#/components/schemas/SSOStartResponse"), "404": errResp("Provider unavailable")}),
		},
		"/api/user/security/social/{provider}": map[string]any{
			"delete": op([]string{"User"}, "Unlink social provider", "Unlinks a provider only when another social provider, passkey, or recently confirmed enabled password remains usable. TOTP, recovery codes, email MFA, and team SSO do not satisfy this guard.", secCookie(), []any{providerParam},
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"current_password": map[string]any{"type": "string", "writeOnly": true}}}),
				map[string]any{"204": desc("Provider unlinked"), "404": errResp("Identity not linked"), "409": errResp("No alternative primary login method")}),
		},
	}
}
