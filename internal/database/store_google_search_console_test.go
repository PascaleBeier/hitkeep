package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGoogleSearchConsoleConnectionRoundTripAndDisconnect(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID, teamID := createGoogleSearchConsoleTestTeam(t, store)

	expiresAt := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	input := GoogleSearchConsoleConnectionInput{
		TeamID:             teamID,
		ConnectedByUserID:  userID,
		GoogleAccountEmail: "owner@example.com",
		GoogleAccountID:    "google-account-1",
		AccessToken:        "access-token-value",
		RefreshToken:       "refresh-token-value",
		TokenType:          "Bearer",
		Scope:              "https://www.googleapis.com/auth/webmasters.readonly",
		TokenExpiry:        expiresAt,
		ConnectedAt:        expiresAt.Add(-time.Minute),
	}
	if err := store.UpsertGoogleSearchConsoleConnection(ctx, input); err != nil {
		t.Fatalf("upsert connection: %v", err)
	}

	conn := requireDatabaseGoogleSearchConsoleConnection(t, store, teamID)
	if conn.GoogleAccountEmail != "owner@example.com" {
		t.Fatalf("expected account email, got %q", conn.GoogleAccountEmail)
	}
	if conn.AccessToken != "access-token-value" || conn.RefreshToken != "refresh-token-value" {
		t.Fatalf("expected token material to round-trip for worker use")
	}
	if !conn.TokenExpiry.Equal(expiresAt) {
		t.Fatalf("expected token expiry %s, got %s", expiresAt, conn.TokenExpiry)
	}

	disconnectedAt := expiresAt.Add(time.Minute)
	if err := store.DisconnectGoogleSearchConsoleConnection(ctx, teamID, disconnectedAt); err != nil {
		t.Fatalf("disconnect connection: %v", err)
	}

	conn, err := store.GetGoogleSearchConsoleConnection(ctx, teamID)
	requireNoDatabaseGoogleSearchConsoleError(t, err)
	if conn == nil {
		t.Fatalf("expected disconnected metadata to remain")
	}
	if conn.Connected {
		t.Fatalf("expected connected=false after disconnect")
	}
	if conn.AccessToken != "" || conn.RefreshToken != "" {
		t.Fatalf("expected token material to be cleared on disconnect")
	}
	if conn.DisconnectedAt == nil || !conn.DisconnectedAt.Equal(disconnectedAt) {
		t.Fatalf("expected disconnected_at %s, got %v", disconnectedAt, conn.DisconnectedAt)
	}
}

func TestGetGoogleSearchConsoleConnectionMissing(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	conn, err := store.GetGoogleSearchConsoleConnection(ctx, uuid.New())
	if err != nil {
		t.Fatalf("get missing connection: %v", err)
	}
	if conn != nil {
		t.Fatalf("expected nil connection, got %+v", conn)
	}
}

func TestGoogleSearchConsoleMappingWithAuditRollsBackWhenAuditFails(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	userID, teamID := createGoogleSearchConsoleTestTeam(t, store)
	site, err := store.CreateSite(ctx, userID, "audit-rollback.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	err = store.UpsertGoogleSearchConsoleSiteMappingWithAudit(ctx, GoogleSearchConsoleSiteMappingInput{
		SiteID:      site.ID,
		TeamID:      teamID,
		PropertyURI: "sc-domain:audit-rollback.example.com",
		MappedBy:    userID,
		MappedAt:    time.Now().UTC(),
	}, AuditEntryParams{
		TeamID:      teamID,
		ActorID:     userID,
		TargetType:  "site",
		TargetID:    site.ID.String(),
		TargetLabel: site.Domain,
		Outcome:     "success",
		Details:     "old_property_uri=;new_property_uri=sc-domain:audit-rollback.example.com",
	})
	if err == nil {
		t.Fatalf("expected missing audit action to fail")
	}

	mapping, err := store.GetGoogleSearchConsoleSiteMapping(ctx, site.ID)
	if err != nil {
		t.Fatalf("get mapping: %v", err)
	}
	if mapping != nil {
		t.Fatalf("expected mapping rollback when audit append fails, got %+v", mapping)
	}
	entries, total, err := store.ListTeamAuditEntries(ctx, teamID, "", 5, 0)
	if err != nil {
		t.Fatalf("list audit entries: %v", err)
	}
	if total != 0 || len(entries) != 0 {
		t.Fatalf("expected no partial audit entry, got total=%d entries=%+v", total, entries)
	}
}

func TestListGoogleSearchConsoleSyncCandidatesSelectsDueConnectedMappings(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	connectedUserID, connectedTeamID := createGoogleSearchConsoleNamedTeam(t, store, "gsc-connected@test.dev", "Connected GSC Team")
	disconnectedUserID, disconnectedTeamID := createGoogleSearchConsoleNamedTeam(t, store, "gsc-disconnected@test.dev", "Disconnected GSC Team")
	if err := store.UpsertGoogleSearchConsoleConnection(ctx, GoogleSearchConsoleConnectionInput{
		TeamID:       connectedTeamID,
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		TokenExpiry:  now.Add(time.Hour),
		ConnectedAt:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed connected connection: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleConnection(ctx, GoogleSearchConsoleConnectionInput{
		TeamID:       disconnectedTeamID,
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		TokenExpiry:  now.Add(time.Hour),
		ConnectedAt:  now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed disconnected connection: %v", err)
	}
	if err := store.DisconnectGoogleSearchConsoleConnection(ctx, disconnectedTeamID, now); err != nil {
		t.Fatalf("disconnect connection: %v", err)
	}

	pendingSite := seedGoogleSearchConsoleCandidateSite(t, store, connectedUserID, connectedTeamID, "pending-candidate.example.com")
	overdueSite := seedGoogleSearchConsoleCandidateSite(t, store, connectedUserID, connectedTeamID, "overdue-candidate.example.com")
	futureRetrySite := seedGoogleSearchConsoleCandidateSite(t, store, connectedUserID, connectedTeamID, "future-retry-candidate.example.com")
	attentionSite := seedGoogleSearchConsoleCandidateSite(t, store, connectedUserID, connectedTeamID, "attention-candidate.example.com")
	disconnectedSite := seedGoogleSearchConsoleCandidateSite(t, store, disconnectedUserID, disconnectedTeamID, "disconnected-candidate.example.com")

	lastSuccess := now.Add(-25 * time.Hour)
	if err := store.UpsertGoogleSearchConsoleSyncState(ctx, GoogleSearchConsoleSyncStateInput{
		SiteID:        pendingSite,
		TeamID:        connectedTeamID,
		State:         "pending",
		LastAttemptAt: ptrTime(now.Add(-time.Hour)),
		Manual:        true,
	}); err != nil {
		t.Fatalf("seed pending sync state: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSyncState(ctx, GoogleSearchConsoleSyncStateInput{
		SiteID:        overdueSite,
		TeamID:        connectedTeamID,
		State:         "succeeded",
		LastSuccessAt: &lastSuccess,
	}); err != nil {
		t.Fatalf("seed overdue sync state: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSyncState(ctx, GoogleSearchConsoleSyncStateInput{
		SiteID:            futureRetrySite,
		TeamID:            connectedTeamID,
		State:             "failed",
		LastErrorCategory: "quota_limited",
		NextRetryAt:       ptrTime(now.Add(time.Hour)),
	}); err != nil {
		t.Fatalf("seed future retry sync state: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSyncState(ctx, GoogleSearchConsoleSyncStateInput{
		SiteID:            attentionSite,
		TeamID:            connectedTeamID,
		State:             "needs_attention",
		LastErrorCategory: "authorization_revoked",
	}); err != nil {
		t.Fatalf("seed attention sync state: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSyncState(ctx, GoogleSearchConsoleSyncStateInput{
		SiteID: disconnectedSite,
		TeamID: disconnectedTeamID,
		State:  "pending",
		Manual: true,
	}); err != nil {
		t.Fatalf("seed disconnected sync state: %v", err)
	}

	candidates, err := store.ListGoogleSearchConsoleSyncCandidates(ctx, now, 10)
	if err != nil {
		t.Fatalf("list sync candidates: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected pending and overdue candidates only, got %+v", candidates)
	}
	if candidates[0].SiteID != pendingSite || !candidates[0].Manual {
		t.Fatalf("expected manual pending candidate first, got %+v", candidates[0])
	}
	if candidates[1].SiteID != overdueSite || candidates[1].Manual {
		t.Fatalf("expected overdue recurring candidate second, got %+v", candidates[1])
	}
}

func createGoogleSearchConsoleTestTeam(t *testing.T, store *Store) (uuid.UUID, uuid.UUID) {
	t.Helper()
	return createGoogleSearchConsoleTestTeamWithEmail(t, store, "gsc-owner@test.dev")
}

func createGoogleSearchConsoleTestTeamWithEmail(t *testing.T, store *Store, email string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	userID, err := store.CreateUser(context.Background(), email, "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	teamID, err := store.GetActiveTenantID(context.Background(), userID)
	if err != nil {
		t.Fatalf("active team: %v", err)
	}
	return userID, teamID
}

func createGoogleSearchConsoleNamedTeam(t *testing.T, store *Store, email, name string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	userID, err := store.CreateUser(ctx, email, "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	team, err := store.CreateTenant(ctx, userID, name, "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return userID, team.ID
}

func seedGoogleSearchConsoleCandidateSite(t *testing.T, store *Store, userID, teamID uuid.UUID, domain string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if err := store.SetActiveTenantID(ctx, userID, teamID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, domain)
	if err != nil {
		t.Fatalf("create candidate site: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSiteMapping(ctx, GoogleSearchConsoleSiteMappingInput{
		SiteID:      site.ID,
		TeamID:      teamID,
		PropertyURI: "sc-domain:" + domain,
		MappedBy:    userID,
		MappedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed candidate mapping: %v", err)
	}
	return site.ID
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func requireDatabaseGoogleSearchConsoleConnection(t *testing.T, store *Store, teamID uuid.UUID) *GoogleSearchConsoleConnection {
	t.Helper()
	conn, err := store.GetGoogleSearchConsoleConnection(context.Background(), teamID)
	requireNoDatabaseGoogleSearchConsoleError(t, err)
	if conn == nil {
		t.Fatalf("expected connection")
	}
	if !conn.Connected {
		t.Fatalf("expected connected=true")
	}
	return conn
}

func requireNoDatabaseGoogleSearchConsoleError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected Google Search Console store error: %v", err)
	}
}
