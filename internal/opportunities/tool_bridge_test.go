package opportunities

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/database"
)

func setupToolBridgeStore(t *testing.T) (*database.Store, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ownerID, err := store.CreateUser(context.Background(), "owner-tools@example.com", "hashed")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	viewerID, err := store.CreateUser(context.Background(), "viewer-tools@example.com", "hashed")
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	outsiderID, err := store.CreateUser(context.Background(), "outsider-tools@example.com", "hashed")
	if err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	site, err := store.CreateSite(context.Background(), ownerID, "tools.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	teamID, err := store.GetSiteTenantID(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("site team: %v", err)
	}
	if err := store.AddTeamMember(context.Background(), teamID, viewerID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add viewer team member: %v", err)
	}
	if err := store.AddSiteMember(context.Background(), site.ID, viewerID, auth.SiteViewer, ownerID); err != nil {
		t.Fatalf("add viewer site member: %v", err)
	}
	seedToolBridgeAggregates(t, store, site.ID)
	return store, site.ID, teamID, viewerID, outsiderID
}

func seedToolBridgeAggregates(t *testing.T, store *database.Store, siteID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	sessionID := uuid.New()
	isUnique := true
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    siteID,
		SessionID: sessionID,
		PageID:    uuid.New(),
		Path:      "/checkout",
		Timestamp: now.Add(-time.Hour),
		IsUnique:  &isUnique,
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}
	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:    siteID,
		SessionID: sessionID,
		Name:      "begin_checkout",
		Timestamp: now.Add(-45 * time.Minute),
		Properties: map[string]any{
			"items": []map[string]any{{"item_id": "pro", "quantity": 1, "price": 99.0}},
		},
	}); err != nil {
		t.Fatalf("create checkout event: %v", err)
	}
	if err := store.CreateAIFetch(ctx, &api.AIFetch{
		SiteID:          siteID,
		Timestamp:       now.Add(-30 * time.Minute),
		AssistantName:   "ChatGPT",
		AssistantFamily: "openai",
		Path:            "/checkout",
		StatusCode:      200,
		ResourceType:    "page",
	}); err != nil {
		t.Fatalf("create ai fetch: %v", err)
	}
}

func TestToolBridgeAllowsViewerAggregateToolsWithoutRawRows(t *testing.T) {
	store, siteID, teamID, viewerID, _ := setupToolBridgeStore(t)
	bridge := NewToolBridge(ToolBridgeConfig{
		Shared:            store,
		Analytics:         store,
		TeamID:            teamID,
		SiteID:            siteID,
		ActorID:           viewerID,
		ActorType:         "user",
		EffectiveUserID:   viewerID,
		EffectiveSiteRole: auth.SiteViewer,
		From:              time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:                time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})

	for _, toolName := range []string{"hitkeep_get_site_overview", "hitkeep_get_ecommerce", "hitkeep_get_ai_visibility"} {
		raw := executeBridgeTool(t, bridge, toolName)
		if containsRawRowField(raw) {
			t.Fatalf("%s returned raw row field: %s", toolName, raw)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			t.Fatalf("%s returned invalid JSON: %v", toolName, err)
		}
		if len(body) == 0 {
			t.Fatalf("%s returned empty aggregate body", toolName)
		}
	}
}

func TestToolBridgeDeniesUnauthorizedActor(t *testing.T) {
	store, siteID, teamID, _, outsiderID := setupToolBridgeStore(t)
	bridge := NewToolBridge(ToolBridgeConfig{
		Shared:    store,
		Analytics: store,
		TeamID:    teamID,
		SiteID:    siteID,
		ActorID:   outsiderID,
		ActorType: "user",
		From:      time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:        time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})

	_, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview")
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected access denied, got %v", err)
	}
}

func TestToolBridgeDeniesUserWhenEffectivePrincipalDoesNotMatchActor(t *testing.T) {
	store, siteID, teamID, viewerID, outsiderID := setupToolBridgeStore(t)
	bridge := NewToolBridge(ToolBridgeConfig{
		Shared:            store,
		Analytics:         store,
		TeamID:            teamID,
		SiteID:            siteID,
		ActorID:           outsiderID,
		ActorType:         "user",
		EffectiveUserID:   viewerID,
		EffectiveSiteRole: auth.SiteViewer,
		From:              time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:                time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})

	_, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview")
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected mismatched effective principal denial, got %v", err)
	}
}

func TestToolBridgeDeniesViewerWhenConfiguredTeamDoesNotOwnSite(t *testing.T) {
	store, siteID, _, viewerID, _ := setupToolBridgeStore(t)
	bridge := NewToolBridge(ToolBridgeConfig{
		Shared:            store,
		Analytics:         store,
		TeamID:            uuid.New(),
		SiteID:            siteID,
		ActorID:           viewerID,
		ActorType:         "user",
		EffectiveUserID:   viewerID,
		EffectiveSiteRole: auth.SiteViewer,
		From:              time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:                time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})

	_, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview")
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected configured team/site scope denial, got %v", err)
	}
}

func TestToolBridgeAllowsInstanceActorWithSiteViewPermission(t *testing.T) {
	store, siteID, teamID, _, _ := setupToolBridgeStore(t)
	adminID, err := store.CreateUser(context.Background(), "instance-admin-tools@example.com", "hashed")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := store.UpdateInstanceRole(context.Background(), adminID, auth.InstanceAdmin, adminID); err != nil {
		t.Fatalf("make admin: %v", err)
	}
	bridge := NewToolBridge(ToolBridgeConfig{
		Shared:                store,
		Analytics:             store,
		TeamID:                teamID,
		SiteID:                siteID,
		ActorID:               adminID,
		ActorType:             "user",
		EffectiveUserID:       adminID,
		EffectiveInstanceRole: auth.InstanceAdmin,
		From:                  time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:                    time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})

	if _, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview"); err != nil {
		t.Fatalf("expected instance admin to run bridge tool: %v", err)
	}
}

func TestToolBridgeAllowsAPIClientWhenEffectiveRoleAndDelegationCanViewSite(t *testing.T) {
	store, siteID, teamID, _, _ := setupToolBridgeStore(t)
	clientID := uuid.New()
	bridge := NewToolBridge(ToolBridgeConfig{
		Shared:            store,
		Analytics:         store,
		TeamID:            teamID,
		SiteID:            siteID,
		ActorID:           clientID,
		ActorType:         "api_client",
		EffectiveUserID:   uuid.Nil,
		EffectiveSiteRole: auth.SiteViewer,
		APIClientAuth: &database.APIClientAuth{
			ClientID:  clientID,
			TenantID:  teamID,
			SiteRoles: map[uuid.UUID]auth.SiteRole{siteID: auth.SiteViewer},
		},
		From: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})

	if _, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview"); err != nil {
		t.Fatalf("expected api client to run bridge tool: %v", err)
	}
}

func TestToolBridgeDeniesAPIClientWhenDelegationDoesNotMatchActor(t *testing.T) {
	store, siteID, teamID, _, _ := setupToolBridgeStore(t)
	bridge := NewToolBridge(ToolBridgeConfig{
		Shared:            store,
		Analytics:         store,
		TeamID:            teamID,
		SiteID:            siteID,
		ActorID:           uuid.New(),
		ActorType:         "api_client",
		EffectiveUserID:   uuid.Nil,
		EffectiveSiteRole: auth.SiteViewer,
		APIClientAuth: &database.APIClientAuth{
			ClientID:  uuid.New(),
			TenantID:  teamID,
			SiteRoles: map[uuid.UUID]auth.SiteRole{siteID: auth.SiteViewer},
		},
		From: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})

	_, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview")
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected api client actor mismatch denial, got %v", err)
	}
}

func TestToolBridgeDeniesAPIClientWithoutDelegatedSiteView(t *testing.T) {
	store, siteID, teamID, _, _ := setupToolBridgeStore(t)
	clientID := uuid.New()
	bridge := NewToolBridge(ToolBridgeConfig{
		Shared:            store,
		Analytics:         store,
		TeamID:            teamID,
		SiteID:            siteID,
		ActorID:           clientID,
		ActorType:         "api_client",
		EffectiveUserID:   uuid.Nil,
		EffectiveSiteRole: auth.SiteViewer,
		APIClientAuth: &database.APIClientAuth{
			ClientID:  clientID,
			TenantID:  teamID,
			SiteRoles: map[uuid.UUID]auth.SiteRole{uuid.New(): auth.SiteViewer},
		},
		From: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})

	_, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview")
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected api client site delegation denial, got %v", err)
	}
}

func TestToolBridgeDeniesAPIClientWithoutEffectiveSiteView(t *testing.T) {
	store, siteID, teamID, _, _ := setupToolBridgeStore(t)
	clientID := uuid.New()
	bridge := NewToolBridge(ToolBridgeConfig{
		Shared:          store,
		Analytics:       store,
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         clientID,
		ActorType:       "api_client",
		EffectiveUserID: uuid.Nil,
		APIClientAuth: &database.APIClientAuth{
			ClientID:  clientID,
			TenantID:  teamID,
			SiteRoles: map[uuid.UUID]auth.SiteRole{siteID: auth.SiteViewer},
		},
		From: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})

	_, err := executeBridgeToolErr(bridge, "hitkeep_get_site_overview")
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected api client effective site role denial, got %v", err)
	}
}

func TestToolBridgeSchedulerIsScopedToTeamAndSite(t *testing.T) {
	store, siteID, teamID, _, _ := setupToolBridgeStore(t)
	allowed := NewToolBridge(ToolBridgeConfig{
		Shared:          store,
		Analytics:       store,
		TeamID:          teamID,
		SiteID:          siteID,
		ActorType:       "ai_scheduler",
		SchedulerTeamID: teamID,
		SchedulerSiteID: siteID,
		From:            time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:              time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})
	if _, err := executeBridgeToolErr(allowed, "hitkeep_get_site_overview"); err != nil {
		t.Fatalf("expected scoped scheduler to run: %v", err)
	}

	denied := NewToolBridge(ToolBridgeConfig{
		Shared:          store,
		Analytics:       store,
		TeamID:          teamID,
		SiteID:          siteID,
		ActorType:       "ai_scheduler",
		SchedulerTeamID: teamID,
		SchedulerSiteID: uuid.New(),
		From:            time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:              time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})
	_, err := executeBridgeToolErr(denied, "hitkeep_get_site_overview")
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected scheduler scope denial, got %v", err)
	}

	deniedTeam := NewToolBridge(ToolBridgeConfig{
		Shared:          store,
		Analytics:       store,
		TeamID:          teamID,
		SiteID:          siteID,
		ActorType:       "ai_scheduler",
		SchedulerTeamID: uuid.New(),
		SchedulerSiteID: siteID,
		From:            time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:              time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	})
	_, err = executeBridgeToolErr(deniedTeam, "hitkeep_get_site_overview")
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected scheduler team scope denial, got %v", err)
	}
}

func executeBridgeTool(t *testing.T, bridge ToolBridge, name string) string {
	t.Helper()
	raw, err := executeBridgeToolErr(bridge, name)
	if err != nil {
		t.Fatalf("execute %s: %v", name, err)
	}
	return raw
}

func executeBridgeToolErr(bridge ToolBridge, name string) (string, error) {
	for _, tool := range bridge.Tools() {
		if tool.Name == name {
			return tool.Execute(context.Background(), nil)
		}
	}
	return "", errors.New("tool not found")
}

func containsRawRowField(raw string) bool {
	for _, field := range []string{"session_id", "visitor_ip", "ip_address", "user_agent", "page_id"} {
		if strings.Contains(raw, field) {
			return true
		}
	}
	return false
}
