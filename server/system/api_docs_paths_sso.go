package system

func openAPIV1SSOPaths() map[string]any {
	return map[string]any{
		"/api/auth/sso": map[string]any{
			"get": op([]string{"Auth"}, "Get SSO availability", "Returns whether at least one team has an enabled SSO configuration. This controls whether the public login page offers SSO; it does not reveal team or provider details.", nil, nil, nil,
				map[string]any{"200": jsonRefResp("SSO availability", "#/components/schemas/SSOAvailability")}),
		},
		"/api/auth/sso/invite": map[string]any{
			"post": op([]string{"Auth"}, "Get invitation SSO availability", "Returns whether the team targeted by an unconsumed invitation has an enabled and entitled SSO configuration for the invited email domain. Invalid, expired, and non-SSO invitations return enabled=false without exposing team or provider details.", nil, nil,
				jsonBody(map[string]any{
					"type":       "object",
					"properties": map[string]any{"invite_token": map[string]any{"type": "string"}},
					"required":   []string{"invite_token"},
				}),
				map[string]any{"200": jsonRefResp("Invitation SSO availability", "#/components/schemas/SSOAvailability")}),
		},
		"/api/auth/sso/start": map[string]any{
			"post": op([]string{"Auth"}, "Start SSO login", "Resolves an enabled team OIDC provider from the submitted email or invitation, requires an existing membership, a pending invitation, or enabled trusted-domain auto-provisioning, and creates a short-lived state, nonce, and PKCE-protected authorization request. Existing password login remains available.", nil, nil,
				jsonBody(map[string]any{"$ref": "#/components/schemas/SSOStartRequest"}),
				map[string]any{
					"200": jsonRefResp("OIDC authorization URL", "#/components/schemas/SSOStartResponse"),
					"400": errResp("Invalid email or no enabled SSO provider for the email domain"),
					"403": errResp("The account has no team membership, pending invitation, or permitted JIT access, or a managed-cloud limit was reached"),
					"503": errResp("Configured SSO provider is unavailable"),
				}),
		},
		"/api/auth/sso/callback": map[string]any{
			"get": op([]string{"Auth"}, "Complete SSO login", "Consumes one OIDC callback, validates the authorization state, PKCE exchange, signed ID token, issuer, audience, nonce, verified email, and allowed domain, then rechecks live team authorization. A scoped pending invitation retains its requested role; optional trusted-domain JIT provisioning creates a Member only after managed-cloud limits pass. Provider access, ID, and refresh tokens are never persisted.", nil,
				[]any{
					map[string]any{"name": "state", "in": "query", "required": true, "schema": map[string]any{"type": "string"}},
					map[string]any{"name": "code", "in": "query", "required": false, "schema": map[string]any{"type": "string"}},
					map[string]any{"name": "error", "in": "query", "required": false, "schema": map[string]any{"type": "string"}},
				}, nil,
				map[string]any{"303": desc("Redirects to the requested safe return path on success. Failed invitation flows return to invitation acceptance with the unconsumed token and a stable error code; other failures return to login.")}),
		},
		"/api/user/teams/{id}/sso": map[string]any{
			"get": op([]string{"Teams"}, "Get team SSO configuration", "Returns the team's redacted OIDC configuration and callback URL. The client secret is never returned.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil,
				map[string]any{
					"200": jsonRefResp("Redacted team SSO configuration", "#/components/schemas/TeamSSOConfig"),
					"403": errResp("Team settings permission or SSO entitlement required"),
				}),
			"put": op([]string{"Teams"}, "Create or update team SSO configuration", "Validates OIDC discovery and stores the client secret encrypted with the instance key. Leave client_secret blank to keep an existing secret, or provide a new value to rotate it. Enabling this configuration adds SSO as an optional login method; password login is unchanged.", secCookie(), []any{paramRef("#/components/parameters/teamID")},
				jsonBody(map[string]any{"$ref": "#/components/schemas/TeamSSOInput"}),
				map[string]any{
					"200": jsonRefResp("Redacted team SSO configuration", "#/components/schemas/TeamSSOConfig"),
					"400": errResp("Invalid configuration or missing client secret"),
					"403": errResp("Team settings permission or SSO entitlement required"),
					"409": errResp("An allowed email domain belongs to another team SSO configuration"),
					"502": errResp("OIDC discovery failed"),
				}),
			"delete": op([]string{"Teams"}, "Delete team SSO configuration", "Deletes the team SSO configuration and its allowed domains. Existing HitKeep sessions and team roles are unchanged.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil,
				map[string]any{
					"204": desc("SSO configuration deleted"),
					"403": errResp("Team settings permission or SSO entitlement required"),
					"404": errResp("SSO configuration not found"),
				}),
		},
		"/api/user/teams/{id}/sso/test": map[string]any{
			"post": op([]string{"Teams"}, "Test team SSO connection", "Decrypts the saved client secret in memory and validates that the configured OIDC discovery document remains reachable. Secrets and provider response bodies are not returned.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil,
				map[string]any{
					"200": jsonRefResp("Connection test status", "#/components/schemas/Status"),
					"403": errResp("Team settings permission or SSO entitlement required"),
					"404": errResp("SSO configuration not found"),
					"502": errResp("OIDC discovery failed"),
				}),
		},
	}
}
