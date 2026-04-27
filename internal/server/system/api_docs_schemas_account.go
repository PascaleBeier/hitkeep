package system

func openAPIV1AccountSchemas() map[string]any {
	return map[string]any{
		"SiteMember": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":       map[string]any{"type": "string", "format": "uuid"},
				"user_id":  map[string]any{"type": "string", "format": "uuid"},
				"email":    map[string]any{"type": "string", "format": "email"},
				"role":     map[string]any{"type": "string"},
				"added_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"Team": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           map[string]any{"type": "string", "format": "uuid"},
				"name":         map[string]any{"type": "string"},
				"logo_url":     map[string]any{"type": "string"},
				"role":         map[string]any{"type": "string"},
				"created_at":   map[string]any{"type": "string", "format": "date-time"},
				"usage":        map[string]any{"$ref": "#/components/schemas/TeamUsageSummary"},
				"entitlements": map[string]any{"$ref": "#/components/schemas/TeamEntitlements"},
				"plan":         map[string]any{"$ref": "#/components/schemas/TeamPlan"},
			},
		},
		"TeamUsageSummary": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"current_sites":           map[string]any{"type": "integer"},
				"current_members":         map[string]any{"type": "integer"},
				"current_pending_invites": map[string]any{"type": "integer"},
			},
		},
		"TeamEntitlements": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"max_sites_per_team":    map[string]any{"type": "integer"},
				"max_team_members":      map[string]any{"type": "integer"},
				"max_retention_days":    map[string]any{"type": "integer"},
				"allow_sso":             map[string]any{"type": "boolean"},
				"allow_custom_branding": map[string]any{"type": "boolean"},
			},
		},
		"TeamPlan": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code":        map[string]any{"type": "string"},
				"name":        map[string]any{"type": "string"},
				"upgrade_url": map[string]any{"type": "string"},
				"support_url": map[string]any{"type": "string"},
			},
		},
		"CloudStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hosted":         map[string]any{"type": "boolean"},
				"signup_enabled": map[string]any{"type": "boolean"},
				"jurisdiction":   map[string]any{"type": "string"},
				"region":         map[string]any{"type": "string"},
				"upgrade_url":    map[string]any{"type": "string"},
				"support_url":    map[string]any{"type": "string"},
			},
		},
		"TeamMember": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":       map[string]any{"type": "string", "format": "uuid"},
				"user_id":  map[string]any{"type": "string", "format": "uuid"},
				"email":    map[string]any{"type": "string", "format": "email"},
				"role":     map[string]any{"type": "string"},
				"added_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"TeamInvite": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":              map[string]any{"type": "string", "format": "uuid"},
				"team_id":         map[string]any{"type": "string", "format": "uuid"},
				"email":           map[string]any{"type": "string", "format": "email"},
				"role":            map[string]any{"type": "string"},
				"invited_user_id": map[string]any{"type": "string", "format": "uuid"},
				"status":          map[string]any{"type": "string"},
				"created_by":      map[string]any{"type": "string", "format": "uuid"},
				"created_at":      map[string]any{"type": "string", "format": "date-time"},
				"expires_at":      map[string]any{"type": "string", "format": "date-time"},
				"accepted_at":     map[string]any{"type": "string", "format": "date-time"},
				"revoked_at":      map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"TeamAuditEntry": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":             map[string]any{"type": "string", "format": "uuid"},
				"team_id":        map[string]any{"type": "string", "format": "uuid"},
				"action":         map[string]any{"type": "string"},
				"details":        map[string]any{"type": "string"},
				"actor_user_id":  map[string]any{"type": "string", "format": "uuid"},
				"actor_email":    map[string]any{"type": "string", "format": "email"},
				"target_user_id": map[string]any{"type": "string", "format": "uuid"},
				"target_email":   map[string]any{"type": "string", "format": "email"},
				"created_at":     map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"TeamAuditListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entries": map[string]any{
					"type":  "array",
					"items": map[string]any{"$ref": "#/components/schemas/TeamAuditEntry"},
				},
				"total":    map[string]any{"type": "integer"},
				"limit":    map[string]any{"type": "integer"},
				"offset":   map[string]any{"type": "integer"},
				"has_more": map[string]any{"type": "boolean"},
				"action":   map[string]any{"type": "string"},
			},
		},
		"TeamListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"active_team_id": map[string]any{"type": "string", "format": "uuid"},
				"recent_team_ids": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string", "format": "uuid"},
				},
				"teams": map[string]any{
					"type":  "array",
					"items": map[string]any{"$ref": "#/components/schemas/Team"},
				},
			},
		},
		"TeamActiveResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":         map[string]any{"type": "string"},
				"active_team_id": map[string]any{"type": "string", "format": "uuid"},
				"recent_team_ids": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string", "format": "uuid"},
				},
			},
		},
		"TeamCreateResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"team": map[string]any{"$ref": "#/components/schemas/Team"},
			},
		},
		"TeamLeaveResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":         map[string]any{"type": "string"},
				"active_team_id": map[string]any{"type": "string", "format": "uuid"},
				"recent_team_ids": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string", "format": "uuid"},
				},
			},
		},
		"TeamArchiveResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":         map[string]any{"type": "string"},
				"active_team_id": map[string]any{"type": "string", "format": "uuid"},
				"recent_team_ids": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string", "format": "uuid"},
				},
			},
		},
		"AdminDeleteTeamResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":  map[string]any{"type": "string"},
				"team_id": map[string]any{"type": "string", "format": "uuid"},
				"name":    map[string]any{"type": "string"},
			},
		},
		"AdminDisableUserMFAResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":               map[string]any{"type": "string"},
				"totp_disabled":        map[string]any{"type": "boolean"},
				"passkeys_deleted":     map[string]any{"type": "integer"},
				"sessions_invalidated": map[string]any{"type": "integer"},
			},
			"required": []string{"status", "totp_disabled", "passkeys_deleted", "sessions_invalidated"},
		},
		"IPExclusion": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":          map[string]any{"type": "string", "format": "uuid"},
				"site_id":     map[string]any{"type": "string", "format": "uuid"},
				"cidr":        map[string]any{"type": "string"},
				"description": map[string]any{"type": "string"},
				"created_at":  map[string]any{"type": "string", "format": "date-time"},
				"created_by":  map[string]any{"type": "string", "format": "uuid"},
			},
		},
		"IPExclusionCreateRequest": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cidr":        map[string]any{"type": "string", "description": "IP or CIDR value. Plain IP values are normalized to /32 (IPv4) or /128 (IPv6)."},
				"description": map[string]any{"type": "string", "maxLength": 255},
			},
			"required": []string{"cidr"},
		},
		"UserProfile": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           map[string]any{"type": "string", "format": "uuid"},
				"email":        map[string]any{"type": "string", "format": "email"},
				"given_name":   map[string]any{"type": "string"},
				"last_name":    map[string]any{"type": "string"},
				"display_name": map[string]any{"type": "string"},
				"avatar_url":   map[string]any{"type": "string"},
			},
		},
		"UserPreferences": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"default_locale": map[string]any{"type": "string"},
			},
		},
		"AuthSession": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expires_at":                map[string]any{"type": "string", "format": "date-time"},
				"issued_at":                 map[string]any{"type": "string", "format": "date-time"},
				"duration_seconds":          map[string]any{"type": "integer"},
				"warning_seconds":           map[string]any{"type": "integer"},
				"extendable":                map[string]any{"type": "boolean"},
				"timing_adjustable":         map[string]any{"type": "boolean"},
				"remembered":                map[string]any{"type": "boolean"},
				"remember_expires_at":       map[string]any{"type": "string", "format": "date-time"},
				"remember_me_duration_days": map[string]any{"type": "integer"},
			},
		},
		"UserPasskey": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":         map[string]any{"type": "string", "format": "uuid"},
				"name":       map[string]any{"type": "string"},
				"created_at": map[string]any{"type": "string", "format": "date-time"},
				"updated_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"UserSecurityStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"totp_enabled":             map[string]any{"type": "boolean"},
				"totp_pending":             map[string]any{"type": "boolean"},
				"passkeys":                 map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/UserPasskey"}},
				"recovery_codes_generated": map[string]any{"type": "boolean"},
				"recovery_codes_remaining": map[string]any{"type": "integer"},
			},
		},
		"UserRecoveryCodesResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"codes":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"remaining": map[string]any{"type": "integer"},
			},
		},
		"UserTOTPSetup": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"secret":      map[string]any{"type": "string"},
				"otpauth_url": map[string]any{"type": "string"},
				"expires_at":  map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"PermissionContext": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"instance_role": map[string]any{"type": "string"},
				"permissions":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
		"APIClientSiteRole": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"site_id": map[string]any{"type": "string", "format": "uuid"},
				"role":    map[string]any{"type": "string"},
			},
		},
		"APIClient": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":            map[string]any{"type": "string", "format": "uuid"},
				"user_id":       map[string]any{"type": "string", "format": "uuid"},
				"tenant_id":     map[string]any{"type": "string", "format": "uuid"},
				"owner_type":    map[string]any{"type": "string", "enum": []string{"personal", "team"}},
				"name":          map[string]any{"type": "string"},
				"description":   map[string]any{"type": "string"},
				"instance_role": map[string]any{"type": "string"},
				"expires_at":    map[string]any{"type": "string", "format": "date-time"},
				"last_used_at":  map[string]any{"type": "string", "format": "date-time"},
				"revoked_at":    map[string]any{"type": "string", "format": "date-time"},
				"created_at":    map[string]any{"type": "string", "format": "date-time"},
				"updated_at":    map[string]any{"type": "string", "format": "date-time"},
				"site_roles":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/APIClientSiteRole"}},
			},
		},
		"APIClientCreateResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"client": map[string]any{"$ref": "#/components/schemas/APIClient"},
				"token":  map[string]any{"type": "string"},
			},
		},
		"OpenAPIVersionList": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"latest": map[string]any{"type": "string"},
				"versions": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"version":     map[string]any{"type": "string"},
							"openapi_url": map[string]any{"type": "string"},
							"latest":      map[string]any{"type": "boolean"},
						},
					},
				},
			},
		},
		"DigestSubscription": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"daily":   map[string]any{"type": "boolean"},
				"weekly":  map[string]any{"type": "boolean"},
				"monthly": map[string]any{"type": "boolean"},
			},
		},
		"SiteReportSubscription": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"site_id": map[string]any{"type": "string", "format": "uuid"},
				"domain":  map[string]any{"type": "string"},
				"daily":   map[string]any{"type": "boolean"},
				"weekly":  map[string]any{"type": "boolean"},
				"monthly": map[string]any{"type": "boolean"},
			},
		},
		"ReportSubscriptions": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"digest": map[string]any{"$ref": "#/components/schemas/DigestSubscription"},
				"sites": map[string]any{
					"type":  "array",
					"items": map[string]any{"$ref": "#/components/schemas/SiteReportSubscription"},
				},
			},
		},
		"LoginResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":          map[string]any{"type": "string"},
				"challenge_token": map[string]any{"type": "string"},
				"factors":         map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"totp", "passkey", "recovery_code", "email_link"}}},
				"passkey":         map[string]any{"type": "object", "additionalProperties": true},
			},
		},
	}
}
