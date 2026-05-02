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
				"id":                   map[string]any{"type": "string", "format": "uuid"},
				"team_id":              map[string]any{"type": "string", "format": "uuid"},
				"action":               map[string]any{"type": "string"},
				"details":              map[string]any{"type": "string"},
				"actor_user_id":        map[string]any{"type": "string", "format": "uuid"},
				"actor_email":          map[string]any{"type": "string", "format": "email"},
				"actor_email_snapshot": map[string]any{"type": "string"},
				"actor_role_snapshot":  map[string]any{"type": "string"},
				"target_user_id":       map[string]any{"type": "string", "format": "uuid"},
				"target_email":         map[string]any{"type": "string", "format": "email"},
				"target_type":          map[string]any{"type": "string"},
				"target_id":            map[string]any{"type": "string"},
				"target_label":         map[string]any{"type": "string"},
				"outcome":              map[string]any{"type": "string"},
				"ip_address":           map[string]any{"type": "string"},
				"ip_country_code":      map[string]any{"type": "string"},
				"user_agent":           map[string]any{"type": "string"},
				"request_id":           map[string]any{"type": "string"},
				"created_at":           map[string]any{"type": "string", "format": "date-time"},
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
		"SystemFeatureStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":     map[string]any{"type": "string", "description": "Stable feature key."},
				"enabled": map[string]any{"type": "boolean"},
				"detail":  map[string]any{"type": "string", "description": "Optional non-sensitive runtime detail, such as target type or path."},
			},
			"required": []string{"key", "enabled"},
		},
		"SystemInfo": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"version":          map[string]any{"type": "string"},
				"runtime_mode":     map[string]any{"type": "string"},
				"uptime":           map[string]any{"type": "string"},
				"public_url":       map[string]any{"type": "string"},
				"enabled_features": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SystemFeatureStatus"}},
				"config_flags":     map[string]any{"type": "object", "additionalProperties": true},
			},
		},
		"SystemHealth": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":    map[string]any{"type": "string"},
				"database":  map[string]any{"type": "string"},
				"workers":   map[string]any{"type": "string"},
				"is_leader": map[string]any{"type": "boolean"},
			},
		},
		"TenantDBInfo": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tenant_id": map[string]any{"type": "string", "format": "uuid"},
				"name":      map[string]any{"type": "string"},
				"bytes":     map[string]any{"type": "integer", "format": "int64"},
				"path":      map[string]any{"type": "string"},
			},
		},
		"SystemStorage": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"shared_db_path":       map[string]any{"type": "string"},
				"shared_db_bytes":      map[string]any{"type": "integer", "format": "int64"},
				"data_path":            map[string]any{"type": "string"},
				"tenant_db_count":      map[string]any{"type": "integer"},
				"tenant_dbs":           map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/TenantDBInfo"}},
				"spam_cache_path":      map[string]any{"type": "string"},
				"backup_path":          map[string]any{"type": "string"},
				"disk_available_bytes": map[string]any{"type": "integer", "format": "int64"},
				"disk_total_bytes":     map[string]any{"type": "integer", "format": "int64"},
			},
		},
		"SystemIngestStats": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"recent_hits":       map[string]any{"type": "integer"},
				"recent_events":     map[string]any{"type": "integer"},
				"recent_rejections": map[string]any{"type": "integer"},
				"recent_spam":       map[string]any{"type": "integer"},
				"hits_per_second":   map[string]any{"type": "number", "format": "double"},
			},
		},
		"SystemBackupStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"enabled":         map[string]any{"type": "boolean"},
				"config_path":     map[string]any{"type": "string"},
				"interval_min":    map[string]any{"type": "integer"},
				"retention":       map[string]any{"type": "integer"},
				"last_backup":     map[string]any{"type": "string", "format": "date-time"},
				"next_backup":     map[string]any{"type": "string", "format": "date-time"},
				"last_failed_at":  map[string]any{"type": "string", "format": "date-time"},
				"last_error":      map[string]any{"type": "string"},
				"recent_failures": map[string]any{"type": "integer"},
			},
		},
		"SystemSpamStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"db_path":      map[string]any{"type": "string"},
				"last_refresh": map[string]any{"type": "string", "format": "date-time"},
				"rule_count":   map[string]any{"type": "integer"},
				"auto_update":  map[string]any{"type": "boolean"},
				"last_error":   map[string]any{"type": "string"},
			},
		},
		"ImportStageCleanupRunResult": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"imports_cleaned":       map[string]any{"type": "integer"},
				"files_cleaned":         map[string]any{"type": "integer"},
				"bytes_cleaned":         map[string]any{"type": "integer", "format": "int64"},
				"imports_marked_failed": map[string]any{"type": "integer"},
				"errors":                map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
		"SystemImportStageCleanupStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"enabled":              map[string]any{"type": "boolean"},
				"retention_days":       map[string]any{"type": "integer"},
				"stale_imports":        map[string]any{"type": "integer"},
				"stale_files":          map[string]any{"type": "integer"},
				"stale_bytes":          map[string]any{"type": "integer", "format": "int64"},
				"last_run":             map[string]any{"type": "string", "format": "date-time"},
				"last_failed_at":       map[string]any{"type": "string", "format": "date-time"},
				"last_error":           map[string]any{"type": "string"},
				"recent_failures":      map[string]any{"type": "integer"},
				"last_cleaned_imports": map[string]any{"type": "integer"},
				"last_cleaned_files":   map[string]any{"type": "integer"},
				"last_cleaned_bytes":   map[string]any{"type": "integer", "format": "int64"},
				"last_marked_failed":   map[string]any{"type": "integer"},
			},
		},
		"SystemCacheEntry": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"size":     map[string]any{"type": "integer"},
				"max_size": map[string]any{"type": "integer"},
				"ttl":      map[string]any{"type": "string"},
			},
		},
		"SystemCacheStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"permissions_cache":  map[string]any{"$ref": "#/components/schemas/SystemCacheEntry"},
				"api_client_cache":   map[string]any{"$ref": "#/components/schemas/SystemCacheEntry"},
				"rate_limiter_cache": map[string]any{"$ref": "#/components/schemas/SystemCacheEntry"},
				"status":             map[string]any{"type": "string"},
			},
		},
		"SystemMailStatus": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"configured":   map[string]any{"type": "boolean"},
				"driver":       map[string]any{"type": "string"},
				"host":         map[string]any{"type": "string"},
				"port":         map[string]any{"type": "integer"},
				"encryption":   map[string]any{"type": "string"},
				"from_address": map[string]any{"type": "string"},
				"from_name":    map[string]any{"type": "string"},
				"username":     map[string]any{"type": "string"},
				"password_set": map[string]any{"type": "boolean"},
				"last_test_at": map[string]any{"type": "string", "format": "date-time"},
				"last_test_ok": map[string]any{"type": "boolean"},
			},
		},
		"SystemActionResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":  map[string]any{"type": "string"},
				"message": map[string]any{"type": "string"},
			},
		},
		"SystemImportStageCleanupRunResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":  map[string]any{"type": "string"},
				"message": map[string]any{"type": "string"},
				"result":  map[string]any{"$ref": "#/components/schemas/ImportStageCleanupRunResult"},
			},
		},
		"InstanceAuditEntry": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":                   map[string]any{"type": "string", "format": "uuid"},
				"created_at":           map[string]any{"type": "string", "format": "date-time"},
				"actor_id":             map[string]any{"type": "string", "format": "uuid"},
				"team_id":              map[string]any{"type": "string", "format": "uuid"},
				"target_user_id":       map[string]any{"type": "string", "format": "uuid"},
				"actor_email_snapshot": map[string]any{"type": "string"},
				"actor_role_snapshot":  map[string]any{"type": "string"},
				"action":               map[string]any{"type": "string"},
				"target_type":          map[string]any{"type": "string"},
				"target_id":            map[string]any{"type": "string"},
				"target_label":         map[string]any{"type": "string"},
				"outcome":              map[string]any{"type": "string"},
				"ip_address":           map[string]any{"type": "string"},
				"ip_country_code":      map[string]any{"type": "string"},
				"user_agent":           map[string]any{"type": "string"},
				"request_id":           map[string]any{"type": "string"},
				"details":              map[string]any{"type": "string"},
			},
		},
		"InstanceAuditListResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entries":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/InstanceAuditEntry"}},
				"total":    map[string]any{"type": "integer"},
				"limit":    map[string]any{"type": "integer"},
				"offset":   map[string]any{"type": "integer"},
				"has_more": map[string]any{"type": "boolean"},
			},
		},
		"SystemActivationRow": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"team_id":         map[string]any{"type": "string", "format": "uuid"},
				"team_name":       map[string]any{"type": "string"},
				"owner_email":     map[string]any{"type": "string", "format": "email"},
				"plan_code":       map[string]any{"type": "string"},
				"plan_name":       map[string]any{"type": "string"},
				"cloud_region":    map[string]any{"type": "string"},
				"site_id":         map[string]any{"type": "string", "format": "uuid"},
				"site_domain":     map[string]any{"type": "string"},
				"sites_count":     map[string]any{"type": "integer"},
				"active_sites":    map[string]any{"type": "integer"},
				"status":          map[string]any{"type": "string", "enum": []string{"waiting", "live", "dormant", "domain_mismatch"}},
				"first_hit_at":    map[string]any{"type": "string", "format": "date-time"},
				"last_hit_at":     map[string]any{"type": "string", "format": "date-time"},
				"last_event_at":   map[string]any{"type": "string", "format": "date-time"},
				"last_event_name": map[string]any{"type": "string"},
				"hits_last_24h":   map[string]any{"type": "integer"},
				"hits_last_7d":    map[string]any{"type": "integer"},
				"events_last_7d":  map[string]any{"type": "integer"},
				"tracker_source":  map[string]any{"type": "string"},
				"tracker_version": map[string]any{"type": "string"},
			},
		},
		"SystemActivationResponse": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"rows":     map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SystemActivationRow"}},
				"total":    map[string]any{"type": "integer"},
				"limit":    map[string]any{"type": "integer"},
				"offset":   map[string]any{"type": "integer"},
				"has_more": map[string]any{"type": "boolean"},
			},
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
				"default_locale":          map[string]any{"type": "string"},
				"dismissed_onboarding_at": map[string]any{"type": "string", "format": "date-time"},
			},
		},
		"OnboardingStep": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":         map[string]any{"type": "string", "enum": []string{"create_site", "verify_tracking", "automatic_events", "invite_teammate", "schedule_report"}},
				"complete":    map[string]any{"type": "boolean"},
				"current":     map[string]any{"type": "integer"},
				"target":      map[string]any{"type": "integer"},
				"site_id":     map[string]any{"type": "string", "format": "uuid"},
				"site_domain": map[string]any{"type": "string"},
			},
		},
		"UserOnboarding": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dismissed": map[string]any{"type": "boolean"},
				"complete":  map[string]any{"type": "boolean"},
				"steps":     map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/OnboardingStep"}},
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
				"permissions": map[string]any{
					"type":                 "object",
					"additionalProperties": map[string]any{"type": "string"},
				},
				"instance_permissions": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
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
