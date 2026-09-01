package user

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	json "hitkeep/jsonapi"
)

func TestRegisteredTeamExclusionRoutesEnforceSettingsCapabilityAndEffectiveMetadata(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()
	ctx := context.Background()
	teamID, err := store.GetActiveTenantID(ctx, ownerID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	adminID, err := store.CreateUser(ctx, "exclusion-admin@example.test", "hash")
	if err != nil {
		t.Fatalf("create team admin: %v", err)
	}
	memberID, err := store.CreateUser(ctx, "exclusion-member@example.test", "hash")
	if err != nil {
		t.Fatalf("create team member: %v", err)
	}
	if err := store.AddTeamMember(ctx, teamID, adminID, database.TenantRoleAdmin, ownerID); err != nil {
		t.Fatalf("add team admin: %v", err)
	}
	if err := store.AddTeamMember(ctx, teamID, memberID, database.TenantRoleMember, ownerID); err != nil {
		t.Fatalf("add team member: %v", err)
	}
	instanceRule, err := store.CreateInstanceTrafficExclusion(ctx, database.TrafficExclusionValues{Type: "cidr", CIDR: "203.0.113.9/32"}, ownerID)
	if err != nil {
		t.Fatalf("create inherited instance rule: %v", err)
	}

	mux := http.NewServeMux()
	Register(mux, h.ctx)
	createReq := withTestUser(httptest.NewRequest(
		http.MethodPost,
		"/api/user/teams/"+teamID.String()+"/exclusions",
		bytes.NewReader([]byte(`{"type":"path","path":" /admin//users/../ ","description":"Admin traffic"}`)),
	), ownerID)
	createW := httptest.NewRecorder()
	mux.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d: %s", http.StatusCreated, createW.Code, createW.Body.String())
	}
	var created api.IPExclusion
	if err := json.UnmarshalRead(createW.Body, &created); err != nil {
		t.Fatalf("decode created exclusion: %v", err)
	}
	if created.Scope != "team" || created.TeamID == nil || *created.TeamID != teamID || created.Type != "path" || created.Path != "/admin" || created.Inherited {
		t.Fatalf("unexpected created team exclusion: %#v", created)
	}

	listReq := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+teamID.String()+"/exclusions?effective=true", nil), adminID)
	listW := httptest.NewRecorder()
	mux.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("expected admin effective list status %d, got %d: %s", http.StatusOK, listW.Code, listW.Body.String())
	}
	var rules []api.IPExclusion
	if err := json.UnmarshalRead(listW.Body, &rules); err != nil {
		t.Fatalf("decode effective exclusions: %v", err)
	}
	if len(rules) != 2 || rules[0].ID != instanceRule.ID || !rules[0].Inherited || rules[0].CreatedBy != nil || rules[1].ID != created.ID || rules[1].Inherited {
		t.Fatalf("unexpected effective team exclusions: %#v", rules)
	}

	memberReq := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+teamID.String()+"/exclusions", nil), memberID)
	memberW := httptest.NewRecorder()
	mux.ServeHTTP(memberW, memberReq)
	if memberW.Code != http.StatusForbidden {
		t.Fatalf("expected member status %d, got %d: %s", http.StatusForbidden, memberW.Code, memberW.Body.String())
	}

	deleteInheritedReq := withTestUser(httptest.NewRequest(http.MethodDelete, "/api/user/teams/"+teamID.String()+"/exclusions/"+instanceRule.ID.String(), nil), ownerID)
	deleteInheritedW := httptest.NewRecorder()
	mux.ServeHTTP(deleteInheritedW, deleteInheritedReq)
	if deleteInheritedW.Code != http.StatusNotFound {
		t.Fatalf("expected inherited delete status %d, got %d: %s", http.StatusNotFound, deleteInheritedW.Code, deleteInheritedW.Body.String())
	}

	audit, _, err := store.ListTeamAuditEntries(ctx, teamID, "site.exclusion_created", 10, 0)
	if err != nil {
		t.Fatalf("list team exclusion audit: %v", err)
	}
	if len(audit) == 0 || audit[0].TargetID != created.ID.String() || !strings.Contains(audit[0].Details, "scope=team") || !strings.Contains(audit[0].Details, "type=path") {
		t.Fatalf("unexpected team exclusion audit entry: %#v", audit)
	}
}

func TestTeamExclusionRouteRejectsInvalidEffectiveQuery(t *testing.T) {
	h, store, ownerID := setupUserSecurityTestEnv(t)
	defer store.Close()
	teamID, err := store.GetActiveTenantID(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("get active team: %v", err)
	}
	mux := http.NewServeMux()
	Register(mux, h.ctx)
	req := withTestUser(httptest.NewRequest(http.MethodGet, "/api/user/teams/"+teamID.String()+"/exclusions?effective=maybe", nil), ownerID)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid effective status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}
