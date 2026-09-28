package webhooks

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

type Scope string

const (
	ScopeInstance Scope = "instance"
	ScopeSite     Scope = "site"
)

const (
	EventSiteCreated       = "site.created"
	EventSiteUpdated       = "site.updated"
	EventSiteDeleted       = "site.deleted"
	EventGoalCreated       = "goal.created"
	EventGoalUpdated       = "goal.updated"
	EventGoalDeleted       = "goal.deleted"
	EventGoalConverted     = "goal.converted"
	EventImportCompleted   = "import.completed"
	EventImportFailed      = "import.failed"
	EventWebhookTest       = "webhook.test"
	EventSystemUserUpdated = "system.user.updated"
	EventSystemUserDeleted = "system.user.deleted"
	EventTeamCreated       = "team.created"
	EventTeamUpdated       = "team.updated"
	EventTeamArchived      = "team.archived"
	EventTeamMemberAdded   = "team.member.added"
	EventTeamMemberRemoved = "team.member.removed"
)

var ErrInvalidEventSelection = errors.New("invalid webhook event selection")

type EventDescriptor struct {
	Type       string  `json:"type"`
	SiteScoped bool    `json:"site_scoped"`
	Scopes     []Scope `json:"scopes"`
}

var catalog = []EventDescriptor{
	{Type: EventSiteCreated, Scopes: []Scope{ScopeInstance}},
	{Type: EventSiteUpdated, SiteScoped: true, Scopes: []Scope{ScopeInstance, ScopeSite}},
	{Type: EventSiteDeleted, SiteScoped: true, Scopes: []Scope{ScopeInstance, ScopeSite}},
	{Type: EventGoalCreated, SiteScoped: true, Scopes: []Scope{ScopeInstance, ScopeSite}},
	{Type: EventGoalUpdated, SiteScoped: true, Scopes: []Scope{ScopeInstance, ScopeSite}},
	{Type: EventGoalDeleted, SiteScoped: true, Scopes: []Scope{ScopeInstance, ScopeSite}},
	{Type: EventGoalConverted, SiteScoped: true, Scopes: []Scope{ScopeInstance, ScopeSite}},
	{Type: EventImportCompleted, SiteScoped: true, Scopes: []Scope{ScopeInstance, ScopeSite}},
	{Type: EventImportFailed, SiteScoped: true, Scopes: []Scope{ScopeInstance, ScopeSite}},
	{Type: EventWebhookTest, SiteScoped: true, Scopes: []Scope{ScopeInstance, ScopeSite}},
	{Type: EventSystemUserUpdated, Scopes: []Scope{ScopeInstance}},
	{Type: EventSystemUserDeleted, Scopes: []Scope{ScopeInstance}},
	{Type: EventTeamCreated, Scopes: []Scope{ScopeInstance}},
	{Type: EventTeamUpdated, Scopes: []Scope{ScopeInstance}},
	{Type: EventTeamArchived, Scopes: []Scope{ScopeInstance}},
	{Type: EventTeamMemberAdded, Scopes: []Scope{ScopeInstance}},
	{Type: EventTeamMemberRemoved, Scopes: []Scope{ScopeInstance}},
}

func Catalog(scope Scope) []EventDescriptor {
	result := make([]EventDescriptor, 0, len(catalog))
	for _, event := range catalog {
		if slices.Contains(event.Scopes, scope) {
			event.Scopes = slices.Clone(event.Scopes)
			result = append(result, event)
		}
	}
	return result
}

func EventAllowedForScope(eventType string, scope Scope) bool {
	eventType = strings.TrimSpace(eventType)
	for _, event := range catalog {
		if event.Type == eventType {
			return slices.Contains(event.Scopes, scope)
		}
	}
	return false
}

func ValidateEventSelection(scope Scope, eventTypes []string) error {
	if scope != ScopeInstance && scope != ScopeSite {
		return fmt.Errorf("%w: unknown scope %q", ErrInvalidEventSelection, scope)
	}
	if len(eventTypes) == 0 {
		return fmt.Errorf("%w: select at least one event", ErrInvalidEventSelection)
	}

	seen := make(map[string]struct{}, len(eventTypes))
	for _, raw := range eventTypes {
		eventType := strings.TrimSpace(raw)
		if !EventAllowedForScope(eventType, scope) {
			return fmt.Errorf("%w: event %q is not available for %s webhooks", ErrInvalidEventSelection, eventType, scope)
		}
		if _, ok := seen[eventType]; ok {
			return fmt.Errorf("%w: duplicate event %q", ErrInvalidEventSelection, eventType)
		}
		seen[eventType] = struct{}{}
	}
	return nil
}

func ScopeForSiteID(hasSiteID bool) Scope {
	if hasSiteID {
		return ScopeSite
	}
	return ScopeInstance
}
