package system

func openAPIV1Paths() map[string]any {
	return mergeOpenAPIPathMaps(
		openAPIV1CorePaths(),
		openAPIV1AdminSitePaths(),
	)
}

func openAPIV1CorePaths() map[string]any {
	return map[string]any{
		"/healthz": map[string]any{
			"get": op([]string{"System"}, "Health check", "Liveness endpoint.", nil, nil, nil, map[string]any{"200": desc("OK")}),
		},
		"/readyz": map[string]any{
			"get": op([]string{"System"}, "Readiness check", "Readiness endpoint (leader and DB readiness).", nil, nil, nil, map[string]any{"200": desc("Ready"), "503": errResp("Not ready")}),
		},
		"/api/status": map[string]any{
			"get": op([]string{"System"}, "Instance status", "Setup and version status, plus optional managed-cloud metadata.", nil, nil, nil, map[string]any{
				"200": jsonSchemaResp("Status payload", map[string]any{
					"type": "object",
					"properties": map[string]any{
						"needs_setup": map[string]any{"type": "boolean"},
						"version":     map[string]any{"type": "string"},
						"cloud":       map[string]any{"$ref": "#/components/schemas/CloudStatus"},
					},
				}),
			}),
		},
		"/api/docs/versions": map[string]any{
			"get": op([]string{"System"}, "List API doc versions", "Returns available OpenAPI document versions.", nil, nil, nil, map[string]any{"200": jsonRefResp("Version list", "#/components/schemas/OpenAPIVersionList")}),
		},
		"/api/docs/v1/openapi.json": map[string]any{
			"get": op([]string{"System"}, "OpenAPI v1 document", "Returns the full OpenAPI 3.1 specification for v1.", nil, nil, nil, map[string]any{"200": map[string]any{"description": "OpenAPI 3.1 JSON"}}),
		},

		"/ingest": map[string]any{
			"options": op([]string{"Ingest"}, "Preflight ingest", "CORS preflight for pageview ingest.", nil, nil, nil, map[string]any{"200": desc("Preflight response")}),
			"post": op([]string{"Ingest"}, "Ingest pageview", "Ingests a pageview hit from the browser tracker.", nil, nil,
				map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{
					"path": map[string]any{"type": "string"}, "referrer": map[string]any{"type": "string"}, "ua": map[string]any{"type": "string"},
					"vp_w": map[string]any{"type": "integer"}, "vp_h": map[string]any{"type": "integer"}, "sc_w": map[string]any{"type": "integer"}, "sc_h": map[string]any{"type": "integer"},
					"lang": map[string]any{"type": "string"}, "u_src": map[string]any{"type": "string"}, "u_med": map[string]any{"type": "string"}, "u_cmp": map[string]any{"type": "string"}, "u_trm": map[string]any{"type": "string"}, "u_cnt": map[string]any{"type": "string"},
					"unique": map[string]any{"type": "boolean"}, "session_id": map[string]any{"type": "string", "format": "uuid"}, "page_id": map[string]any{"type": "string", "format": "uuid"},
				}, "required": []string{"path", "session_id", "page_id"}}}}},
				map[string]any{"202": desc("Accepted"), "400": errResp("Invalid request")}),
		},
		"/ingest/event": map[string]any{
			"options": op([]string{"Ingest"}, "Preflight event ingest", "CORS preflight for custom event ingest.", nil, nil, nil, map[string]any{"200": desc("Preflight response")}),
			"post": op([]string{"Ingest"}, "Ingest custom event", "Ingests a custom event from the browser tracker.", nil, nil,
				map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{
					"n": map[string]any{"type": "string"}, "p": map[string]any{"type": "object", "additionalProperties": true}, "sid": map[string]any{"type": "string", "format": "uuid"},
				}, "required": []string{"n", "sid"}}}}},
				map[string]any{"202": desc("Accepted"), "400": errResp("Invalid request")}),
		},

		"/api/initial-user": map[string]any{
			"post": op([]string{"Auth"}, "Create initial admin", "Bootstraps first user account during setup.", nil, nil,
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"email":      map[string]any{"type": "string", "format": "email"},
						"password":   map[string]any{"type": "string", "minLength": 8},
						"given_name": map[string]any{"type": "string"},
						"last_name":  map[string]any{"type": "string"},
					},
					"required": []string{"email", "password"},
				}),
				map[string]any{"201": jsonSchemaResp("Token created", map[string]any{"type": "object", "properties": map[string]any{"token": map[string]any{"type": "string"}}}), "403": errResp("Setup already complete")}),
		},
		"/api/login": map[string]any{
			"post": op([]string{"Auth"}, "Login", "Authenticates user credentials and issues session cookie.", nil, nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}, "password": map[string]any{"type": "string"}, "remember_me": map[string]any{"type": "boolean"}}, "required": []string{"email", "password"}}),
				map[string]any{"200": jsonRefResp("Login response", "#/components/schemas/LoginResponse"), "401": errResp("Invalid credentials")}),
		},
		"/api/logout": map[string]any{
			"post": op([]string{"Auth"}, "Logout", "Clears session and remember-me cookies.", secCookie(), nil, nil, map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},
		"/api/auth/session": map[string]any{
			"get": op([]string{"Auth"}, "Get session policy", "Returns the authenticated session expiry and configured accessibility warning policy.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("Auth session", "#/components/schemas/AuthSession")}),
		},
		"/api/auth/session/extend": map[string]any{
			"post": op([]string{"Auth"}, "Extend session", "Issues a fresh session cookie and returns the updated expiry policy.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("Auth session", "#/components/schemas/AuthSession")}),
		},
		"/api/auth/forgot-password": map[string]any{
			"post": op([]string{"Auth"}, "Request password reset", "Sends password reset email if account exists.", nil, nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}}, "required": []string{"email"}}),
				map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},
		"/api/auth/reset-password": map[string]any{
			"post": op([]string{"Auth"}, "Complete password reset", "Resets password using reset token.", nil, nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"token": map[string]any{"type": "string"}, "password": map[string]any{"type": "string", "minLength": 8}}, "required": []string{"token", "password"}}),
				map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status"), "400": errResp("Invalid or expired link")}),
		},
		"/api/auth/accept-invite": map[string]any{
			"post": op([]string{"Auth"}, "Accept invite", "Sets password for invited user using invite token.", nil, nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"token": map[string]any{"type": "string"}, "password": map[string]any{"type": "string", "minLength": 8}}, "required": []string{"token", "password"}}),
				map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},
		"/api/auth/passkey/login/start": map[string]any{
			"post": op([]string{"Auth"}, "Start passkey login", "Creates passkey login challenge.", nil, nil, nil,
				map[string]any{"200": jsonSchemaResp("Passkey challenge", map[string]any{"type": "object", "properties": map[string]any{"challenge_token": map[string]any{"type": "string"}, "publicKey": map[string]any{"type": "object", "additionalProperties": true}}})}),
		},
		"/api/auth/passkey/login/finish": map[string]any{
			"post": op([]string{"Auth"}, "Finish passkey login", "Verifies passkey assertion and issues session.", nil, nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"challenge_token": map[string]any{"type": "string"}, "credential": map[string]any{"type": "object", "additionalProperties": true}, "remember_me": map[string]any{"type": "boolean"}}, "required": []string{"challenge_token", "credential"}}),
				map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},
		"/api/auth/mfa/totp/verify": map[string]any{
			"post": op([]string{"Auth"}, "Verify MFA TOTP", "Verifies TOTP code for pending MFA challenge.", nil, nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"challenge_token": map[string]any{"type": "string", "format": "uuid"}, "code": map[string]any{"type": "string"}}, "required": []string{"challenge_token", "code"}}),
				map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},
		"/api/auth/mfa/email-link/request": map[string]any{
			"post": op([]string{"Auth"}, "Request MFA email sign-in link", "Sends a one-time email link for an active MFA challenge.", nil, nil,
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"challenge_token": map[string]any{"type": "string", "format": "uuid"},
						"return_url":      map[string]any{"type": "string"},
					},
					"required": []string{"challenge_token"},
				}),
				map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status"), "502": errResp("Failed to send email sign-in link")}),
		},
		"/api/auth/mfa/email-link/verify": map[string]any{
			"get": op([]string{"Auth"}, "Verify MFA email sign-in link", "Consumes a one-time MFA email link and completes the pending sign-in session.", nil,
				[]any{map[string]any{"name": "token", "in": "query", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}}},
				nil,
				map[string]any{"303": desc("Redirects to the requested return URL after a successful sign-in"), "503": errResp("Service not available")}),
		},
		"/api/auth/mfa/recovery-code/verify": map[string]any{
			"post": op([]string{"Auth"}, "Verify MFA recovery code", "Consumes a recovery code for a pending MFA challenge.", nil, nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"challenge_token": map[string]any{"type": "string", "format": "uuid"}, "code": map[string]any{"type": "string"}}, "required": []string{"challenge_token", "code"}}),
				map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},
		"/api/cloud/signup": map[string]any{
			"post": cloudOp("Create managed cloud account", "Creates a hosted cloud user and team, then optionally returns a Stripe Checkout URL for paid plans.", nil, nil,
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"email":        map[string]any{"type": "string", "format": "email"},
						"password":     map[string]any{"type": "string", "minLength": 8},
						"given_name":   map[string]any{"type": "string"},
						"last_name":    map[string]any{"type": "string"},
						"team_name":    map[string]any{"type": "string"},
						"plan_code":    map[string]any{"type": "string", "enum": []string{"free", "pro", "business"}},
						"jurisdiction": map[string]any{"type": "string"},
						"locale":       map[string]any{"type": "string"},
					},
					"required": []string{"email", "password", "team_name", "plan_code"},
				}),
				map[string]any{
					"201": jsonSchemaResp("Cloud signup response", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"status":       map[string]any{"type": "string"},
							"plan_code":    map[string]any{"type": "string"},
							"redirect_url": map[string]any{"type": "string"},
							"checkout_url": map[string]any{"type": "string"},
						},
						"required": []string{"status", "plan_code"},
					}),
					"400": errResp("Invalid request"),
					"404": errResp("Cloud signup disabled"),
					"409": errResp("Email already exists"),
					"502": errResp("Unable to start checkout"),
				}),
		},
		"/api/cloud/billing/portal": map[string]any{
			"post": cloudOp("Create billing portal session", "Creates a Stripe Customer Portal session for the authenticated hosted cloud team.", secCookie(), nil,
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"locale": map[string]any{"type": "string"},
					},
				}),
				map[string]any{
					"200": jsonSchemaResp("Billing portal session", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"url": map[string]any{"type": "string"},
						},
						"required": []string{"url"},
					}),
					"401": errResp("Unauthorized"),
					"404": errResp("Cloud billing account not found"),
					"409": errResp("Stripe customer is not configured"),
					"502": errResp("Unable to start billing portal"),
				}),
		},
		"/api/cloud/billing/checkout": map[string]any{
			"post": cloudOp("Create billing checkout session", "Creates a Stripe Checkout session to upgrade the authenticated hosted cloud team to a paid plan.", secCookie(), nil,
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"plan_code": map[string]any{"type": "string", "enum": []string{"pro", "business"}},
						"locale":    map[string]any{"type": "string"},
					},
					"required": []string{"plan_code"},
				}),
				map[string]any{
					"200": jsonSchemaResp("Billing checkout session", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"url": map[string]any{"type": "string"},
						},
						"required": []string{"url"},
					}),
					"400": errResp("Invalid request"),
					"401": errResp("Unauthorized"),
					"404": errResp("Cloud billing account not found"),
					"409": errResp("Use billing portal to manage an existing paid plan"),
					"502": errResp("Unable to start checkout"),
				}),
		},
		"/api/cloud/webhooks/stripe": map[string]any{
			"post": cloudOp("Stripe webhook", "Processes Stripe billing lifecycle events for managed cloud subscriptions.", nil, nil, nil, map[string]any{
				"200": desc("Webhook processed"),
				"400": errResp("Invalid webhook"),
			}),
		},
		"/api/user/password": map[string]any{
			"post": op([]string{"Auth"}, "Change password", "Changes password for authenticated user.", secCookie(), nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"current_password": map[string]any{"type": "string"}, "new_password": map[string]any{"type": "string", "minLength": 8}}, "required": []string{"current_password", "new_password"}}),
				map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status"), "403": errResp("Current password is incorrect")}),
		},

		"/api/user/profile": map[string]any{
			"get": op([]string{"User"}, "Get profile", "Returns authenticated user profile.", secCookie(), nil, nil, map[string]any{"200": jsonRefResp("User profile", "#/components/schemas/UserProfile")}),
			"put": op([]string{"User"}, "Update profile", "Updates authenticated user profile details.", secCookie(), nil,
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"email":      map[string]any{"type": "string", "format": "email"},
						"given_name": map[string]any{"type": "string"},
						"last_name":  map[string]any{"type": "string"},
					},
					"required": []string{"email"},
				}),
				map[string]any{
					"200": jsonRefResp("Updated profile", "#/components/schemas/UserProfile"),
					"400": errResp("Invalid request"),
					"404": errResp("User not found"),
					"409": errResp("Email already exists"),
				}),
		},
		"/api/user/avatar": map[string]any{
			"get": op([]string{"User"}, "Get avatar", "Proxies authenticated user's avatar image.", secCookie(), []any{paramRef("#/components/parameters/avatarSize")}, nil, map[string]any{"200": desc("Avatar image")}),
		},
		"/api/user/current-ip": map[string]any{
			"get": op([]string{"User"}, "Get current IP", "Returns the resolved client IP and single-host CIDR for quick exclusion setup.", secCookie(), nil, nil, map[string]any{
				"200": jsonSchemaResp("Current IP", map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ip":   map[string]any{"type": "string"},
						"cidr": map[string]any{"type": "string"},
					},
					"required": []string{"ip", "cidr"},
				}),
			}),
		},
		"/api/user/preferences": map[string]any{
			"get": op([]string{"User"}, "Get user preferences", "Returns authenticated user preferences.", secCookie(), nil, nil, map[string]any{"200": jsonRefResp("Preferences", "#/components/schemas/UserPreferences")}),
			"put": op([]string{"User"}, "Update user preferences", "Updates authenticated user preferences.", secCookie(), nil,
				jsonBody(map[string]any{"$ref": "#/components/schemas/UserPreferences"}),
				map[string]any{"200": jsonRefResp("Preferences", "#/components/schemas/UserPreferences")}),
		},
		"/api/user/onboarding": map[string]any{
			"get": op([]string{"User"}, "Get onboarding checklist", "Returns an onboarding checklist computed from sites, tracking status, team membership, and report subscription state.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("Onboarding state", "#/components/schemas/UserOnboarding")}),
		},
		"/api/user/onboarding/dismiss": map[string]any{
			"post": op([]string{"User"}, "Dismiss onboarding checklist", "Persists the authenticated user's onboarding dismissal timestamp.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("Status", "#/components/schemas/Status")}),
		},
		"/api/user/teams": map[string]any{
			"post": op([]string{"Teams"}, "Create team", "Creates a new team and returns the created team payload.", secCookie(), nil,
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":     map[string]any{"type": "string"},
						"logo_url": map[string]any{"type": "string"},
					},
					"required": []string{"name"},
				}),
				map[string]any{
					"201": jsonRefResp("Created team", "#/components/schemas/TeamCreateResponse"),
					"400": errResp("Invalid request"),
					"403": errResp("Team limit reached"),
				}),
			"get": op([]string{"Teams"}, "List teams", "Returns all teams for the authenticated user and the current active team.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("Team list", "#/components/schemas/TeamListResponse")}),
		},
		"/api/user/teams/active": map[string]any{
			"put": op([]string{"Teams"}, "Set active team", "Sets the current active team context for the authenticated user.", secCookie(), nil,
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"team_id": map[string]any{"type": "string", "format": "uuid"},
					},
					"required": []string{"team_id"},
				}),
				map[string]any{
					"200": jsonRefResp("Active team response", "#/components/schemas/TeamActiveResponse"),
					"403": errResp("Access denied"),
				}),
		},
		"/api/user/teams/{id}": map[string]any{
			"patch": op([]string{"Teams"}, "Update team", "Updates team settings. This is the canonical update route.", secCookie(), []any{paramRef("#/components/parameters/teamID")},
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":     map[string]any{"type": "string"},
						"logo_url": map[string]any{"type": "string"},
					},
					"required": []string{"name"},
				}),
				map[string]any{
					"200": jsonRefResp("Status", "#/components/schemas/Status"),
					"400": errResp("Invalid request"),
					"403": errResp("Access denied"),
				}),
			"put": op([]string{"Teams"}, "Update team (deprecated)", "Deprecated compatibility alias for team updates. Use PATCH /api/user/teams/{id}.", secCookie(), []any{paramRef("#/components/parameters/teamID")},
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":     map[string]any{"type": "string"},
						"logo_url": map[string]any{"type": "string"},
					},
					"required": []string{"name"},
				}),
				map[string]any{
					"200": jsonRefResp("Status", "#/components/schemas/Status"),
					"400": errResp("Invalid request"),
					"403": errResp("Access denied"),
				}),
		},
		"/api/user/teams/{id}/transfer-ownership": map[string]any{
			"post": op([]string{"Teams"}, "Transfer team ownership", "Transfers ownership from the current owner to another existing team member.", secCookie(), []any{paramRef("#/components/parameters/teamID")},
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"target_user_id": map[string]any{"type": "string", "format": "uuid"},
					},
					"required": []string{"target_user_id"},
				}),
				map[string]any{
					"200": jsonRefResp("Status", "#/components/schemas/Status"),
					"400": errResp("Invalid target user"),
					"403": errResp("Only owners can transfer ownership"),
					"409": errResp("Transfer conflict"),
				}),
		},
		"/api/user/teams/{id}/archive": map[string]any{
			"post": op([]string{"Teams"}, "Archive team", "Archives a non-default team after all sites have been transferred or removed.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil,
				map[string]any{
					"200": jsonRefResp("Archive team response", "#/components/schemas/TeamArchiveResponse"),
					"400": errResp("Cannot archive the default team or a team that still owns sites"),
					"403": errResp("Only owners can archive teams"),
				}),
		},
		"/api/user/teams/{id}/members": map[string]any{
			"get": op([]string{"Teams"}, "List team members", "Lists members for the specified team.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil,
				map[string]any{"200": jsonSchemaResp("Team members", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/TeamMember"}})}),
			"post": op([]string{"Teams"}, "Invite team member", "Creates a pending invite for a user or updates the role of an existing member.", secCookie(), []any{paramRef("#/components/parameters/teamID")},
				jsonBody(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"email": map[string]any{"type": "string", "format": "email"},
						"role":  map[string]any{"type": "string", "enum": []string{"owner", "admin", "member"}},
					},
					"required": []string{"email", "role"},
				}),
				map[string]any{
					"200": jsonSchemaResp("Invite or update response", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"status":    map[string]any{"type": "string"},
							"is_invite": map[string]any{"type": "boolean"},
							"invite":    map[string]any{"$ref": "#/components/schemas/TeamInvite"},
						},
					}),
					"403": errResp("Access denied"),
					"409": errResp("Invite already pending"),
				}),
		},
		"/api/user/teams/{id}/invites": map[string]any{
			"get": op([]string{"Teams"}, "List team invites", "Lists pending invites for the specified team.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil,
				map[string]any{
					"200": jsonSchemaResp("Team invites", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/TeamInvite"}}),
					"403": errResp("Access denied"),
				}),
		},
		"/api/user/teams/{id}/audit": map[string]any{
			"get": op([]string{"Teams"}, "List team audit log", "Lists recent audit events for team management actions.", secCookie(), []any{
				paramRef("#/components/parameters/teamID"),
				map[string]any{
					"name":        "action",
					"in":          "query",
					"description": "Optional exact audit action to filter by.",
					"schema":      map[string]any{"type": "string"},
				},
				map[string]any{
					"name":        "outcome",
					"in":          "query",
					"description": "Optional exact audit outcome to filter by.",
					"schema":      map[string]any{"type": "string"},
				},
				map[string]any{
					"name":        "target_type",
					"in":          "query",
					"description": "Optional exact audit target type to filter by.",
					"schema":      map[string]any{"type": "string"},
				},
				map[string]any{
					"name":        "query",
					"in":          "query",
					"description": "Searches actor, target, details, IP, country, and request ID fields.",
					"schema":      map[string]any{"type": "string"},
				},
				map[string]any{
					"name":        "from",
					"in":          "query",
					"description": "Inclusive RFC3339 start timestamp.",
					"schema":      map[string]any{"type": "string", "format": "date-time"},
				},
				map[string]any{
					"name":        "to",
					"in":          "query",
					"description": "Inclusive RFC3339 end timestamp.",
					"schema":      map[string]any{"type": "string", "format": "date-time"},
				},
				map[string]any{
					"name":        "limit",
					"in":          "query",
					"description": "Maximum number of audit rows to return (default 25, max 200).",
					"schema":      map[string]any{"type": "integer", "minimum": 1, "maximum": 200},
				},
				map[string]any{
					"name":        "offset",
					"in":          "query",
					"description": "Zero-based audit row offset for pagination.",
					"schema":      map[string]any{"type": "integer", "minimum": 0},
				},
			}, nil,
				map[string]any{
					"200": jsonRefResp("Team audit entries", "#/components/schemas/TeamAuditListResponse"),
					"400": errResp("Invalid query parameters"),
					"403": errResp("Access denied"),
				}),
		},
		"/api/user/teams/{id}/invites/{inviteId}/resend": map[string]any{
			"post": op([]string{"Teams"}, "Resend team invite", "Refreshes and resends a pending invite.", secCookie(), []any{
				paramRef("#/components/parameters/teamID"),
				map[string]any{"name": "inviteId", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
			}, nil,
				map[string]any{
					"200": jsonSchemaResp("Invite resend response", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"status": map[string]any{"type": "string"},
							"invite": map[string]any{"$ref": "#/components/schemas/TeamInvite"},
						},
					}),
					"403": errResp("Access denied"),
					"404": errResp("Invite not found"),
				}),
		},
		"/api/user/teams/{id}/invites/{inviteId}": map[string]any{
			"delete": op([]string{"Teams"}, "Revoke team invite", "Revokes a pending invite.", secCookie(), []any{
				paramRef("#/components/parameters/teamID"),
				map[string]any{"name": "inviteId", "in": "path", "required": true, "schema": map[string]any{"type": "string", "format": "uuid"}},
			}, nil,
				map[string]any{
					"200": jsonRefResp("Status", "#/components/schemas/Status"),
					"403": errResp("Access denied"),
					"404": errResp("Invite not found"),
				}),
		},
		"/api/user/teams/{id}/members/{userId}": map[string]any{
			"delete": op([]string{"Teams"}, "Remove team member", "Removes a member from the specified team.", secCookie(), []any{paramRef("#/components/parameters/teamID"), paramRef("#/components/parameters/userID")}, nil,
				map[string]any{
					"200": jsonRefResp("Status", "#/components/schemas/Status"),
					"400": errResp("Cannot remove last owner"),
					"403": errResp("Access denied"),
					"404": errResp("Team member not found"),
				}),
		},
		"/api/user/teams/{id}/leave": map[string]any{
			"delete": op([]string{"Teams"}, "Leave team", "Removes the authenticated user from the specified team and returns the new active team.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil,
				map[string]any{
					"200": jsonRefResp("Leave team response", "#/components/schemas/TeamLeaveResponse"),
					"400": errResp("Cannot leave your only team or last owner"),
					"403": errResp("Access denied"),
				}),
		},
		"/api/user/security": map[string]any{
			"get": op([]string{"User"}, "Get user security status", "Returns TOTP/passkey status.", secCookie(), nil, nil, map[string]any{"200": jsonRefResp("Security status", "#/components/schemas/UserSecurityStatus")}),
		},
		"/api/user/security/totp/setup/start": map[string]any{
			"post": op([]string{"User"}, "Start TOTP setup", "Starts TOTP enrollment and returns secret + OTPAuth URI.", secCookie(), nil, nil, map[string]any{"200": jsonRefResp("TOTP setup", "#/components/schemas/UserTOTPSetup")}),
		},
		"/api/user/security/totp/setup/verify": map[string]any{
			"post": op([]string{"User"}, "Verify TOTP setup", "Verifies TOTP code and enables TOTP.", secCookie(), nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"code": map[string]any{"type": "string"}}, "required": []string{"code"}}),
				map[string]any{"200": jsonRefResp("Security status", "#/components/schemas/UserSecurityStatus")}),
		},
		"/api/user/security/totp/disable": map[string]any{
			"post": op([]string{"User"}, "Disable TOTP", "Disables TOTP after current code verification.", secCookie(), nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"code": map[string]any{"type": "string"}}, "required": []string{"code"}}),
				map[string]any{"200": jsonRefResp("Security status", "#/components/schemas/UserSecurityStatus")}),
		},
		"/api/user/security/passkeys/register/start": map[string]any{
			"post": op([]string{"User"}, "Start passkey registration", "Creates passkey registration challenge and options.", secCookie(), nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}}),
				map[string]any{"200": jsonSchemaResp("Passkey creation options", map[string]any{"type": "object", "properties": map[string]any{"publicKey": map[string]any{"type": "object", "additionalProperties": true}}})}),
		},
		"/api/user/security/passkeys/register/finish": map[string]any{
			"post": op([]string{"User"}, "Finish passkey registration", "Verifies passkey attestation and stores credential.", secCookie(), nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "type": map[string]any{"type": "string"}, "rawId": map[string]any{"type": "string"}, "response": map[string]any{"type": "object", "additionalProperties": true}}, "required": []string{"id", "type", "rawId", "response"}}),
				map[string]any{"200": jsonRefResp("Security status", "#/components/schemas/UserSecurityStatus")}),
		},
		"/api/user/security/passkeys/{id}": map[string]any{
			"delete": op([]string{"User"}, "Delete passkey", "Deletes a registered passkey credential.", secCookie(), []any{paramRef("#/components/parameters/passkeyID")}, nil,
				map[string]any{"200": jsonRefResp("Security status", "#/components/schemas/UserSecurityStatus")}),
		},
		"/api/user/security/recovery-codes/regenerate": map[string]any{
			"post": op([]string{"User"}, "Regenerate recovery codes", "Generates a new one-time set of backup recovery codes for the authenticated user.", secCookie(), nil, nil,
				map[string]any{"200": jsonRefResp("Recovery codes", "#/components/schemas/UserRecoveryCodesResponse"), "409": errResp("Enable TOTP or register a passkey before generating recovery codes")}),
		},
		"/api/user/api-clients": map[string]any{
			"get": op([]string{"User"}, "List API clients", "Lists API clients for authenticated user.", secCookie(), nil, nil, map[string]any{"200": jsonSchemaResp("API clients", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/APIClient"}})}),
			"post": op([]string{"User"}, "Create API client", "Creates delegated API client and returns one-time token.", secCookie(), nil,
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{
					"name":          map[string]any{"type": "string"},
					"description":   map[string]any{"type": "string"},
					"instance_role": map[string]any{"type": "string"},
					"expires_at":    map[string]any{"type": "string", "format": "date-time"},
					"site_roles":    map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"site_id": map[string]any{"type": "string", "format": "uuid"}, "role": map[string]any{"type": "string"}}}},
				}, "required": []string{"name"}}),
				map[string]any{"201": jsonRefResp("Created API client", "#/components/schemas/APIClientCreateResponse")}),
		},
		"/api/user/api-clients/{id}": map[string]any{
			"put": op([]string{"User"}, "Update API client", "Updates delegated API client.", secCookie(), []any{paramRef("#/components/parameters/apiClientID")},
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{
					"name":          map[string]any{"type": "string"},
					"description":   map[string]any{"type": "string"},
					"instance_role": map[string]any{"type": "string"},
					"expires_at":    map[string]any{"type": "string", "format": "date-time"},
					"revoked":       map[string]any{"type": "boolean"},
					"site_roles":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/APIClientSiteRole"}},
				}, "required": []string{"name", "instance_role"}}),
				map[string]any{"200": jsonRefResp("Updated API client", "#/components/schemas/APIClient")}),
			"delete": op([]string{"User"}, "Delete API client", "Deletes delegated API client.", secCookie(), []any{paramRef("#/components/parameters/apiClientID")}, nil, map[string]any{"204": desc("Deleted")}),
		},
		"/api/user/teams/{id}/api-clients": map[string]any{
			"get": op([]string{"Teams"}, "List team API clients", "Lists team-owned API clients for a team owner or admin.", secCookie(), []any{paramRef("#/components/parameters/teamID")}, nil, map[string]any{"200": jsonSchemaResp("Team API clients", map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/APIClient"}})}),
			"post": op([]string{"Teams"}, "Create team API client", "Creates a team-owned API client and returns a one-time token. Team-owned clients are limited to delegated site scopes within the team.", secCookie(), []any{paramRef("#/components/parameters/teamID")},
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{
					"name":        map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
					"expires_at":  map[string]any{"type": "string", "format": "date-time"},
					"site_roles":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/APIClientSiteRole"}},
				}, "required": []string{"name"}}),
				map[string]any{"201": jsonRefResp("Created team API client", "#/components/schemas/APIClientCreateResponse")}),
		},
		"/api/user/teams/{id}/api-clients/{clientId}": map[string]any{
			"put": op([]string{"Teams"}, "Update team API client", "Updates a team-owned API client.", secCookie(), []any{paramRef("#/components/parameters/teamID"), paramRef("#/components/parameters/teamAPIClientID")},
				jsonBody(map[string]any{"type": "object", "properties": map[string]any{
					"name":        map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
					"expires_at":  map[string]any{"type": "string", "format": "date-time"},
					"revoked":     map[string]any{"type": "boolean"},
					"site_roles":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/APIClientSiteRole"}},
				}, "required": []string{"name"}}),
				map[string]any{"200": jsonRefResp("Updated team API client", "#/components/schemas/APIClient")}),
			"delete": op([]string{"Teams"}, "Delete team API client", "Deletes a team-owned API client.", secCookie(), []any{paramRef("#/components/parameters/teamID"), paramRef("#/components/parameters/teamAPIClientID")}, nil, map[string]any{"204": desc("Deleted")}),
		},

		"/api/user/permissions": map[string]any{
			"get": op([]string{"Permissions"}, "Get permission context", "Returns authenticated user's instance permissions.", secCookie(), nil, nil, map[string]any{"200": jsonRefResp("Permission context", "#/components/schemas/PermissionContext")}),
		},
	}
}
