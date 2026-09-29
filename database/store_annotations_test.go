package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

func TestAnnotationCRUDAndSiteScope(t *testing.T) {
	store, userID, siteID := setupExclusionStore(t)
	defer store.Close()
	ctx := context.Background()

	otherSite, err := store.CreateSite(ctx, userID, "other-annotations.example")
	if err != nil {
		t.Fatalf("create other site: %v", err)
	}

	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	created, err := store.CreateAnnotation(ctx, siteID, api.AnnotationInput{StartsAt: day, Body: "  Launch  "}, userID)
	if err != nil {
		t.Fatalf("create annotation: %v", err)
	}
	if created.Body != "Launch" || created.EndsAt != nil {
		t.Fatalf("created annotation = %+v", created)
	}

	end := day.Add(72 * time.Hour)
	updated, err := store.UpdateAnnotation(ctx, siteID, created.ID, api.AnnotationInput{StartsAt: day, EndsAt: &end, Body: "Campaign"})
	if err != nil {
		t.Fatalf("update annotation: %v", err)
	}
	if updated.Body != "Campaign" || updated.EndsAt == nil || !updated.EndsAt.Equal(end) {
		t.Fatalf("updated annotation = %+v", updated)
	}

	// Another site's ID must never reach this note.
	if _, err := store.UpdateAnnotation(ctx, otherSite.ID, created.ID, api.AnnotationInput{StartsAt: day, Body: "x"}); !errors.Is(err, ErrAnnotationNotFound) {
		t.Fatalf("cross-site update err = %v", err)
	}
	if err := store.DeleteAnnotation(ctx, otherSite.ID, created.ID); !errors.Is(err, ErrAnnotationNotFound) {
		t.Fatalf("cross-site delete err = %v", err)
	}
	if list, _ := store.ListAnnotations(ctx, otherSite.ID, day.AddDate(0, -1, 0), day.AddDate(0, 1, 0)); len(list) != 0 {
		t.Fatalf("cross-site list = %+v", list)
	}

	if err := store.DeleteAnnotation(ctx, siteID, created.ID); err != nil {
		t.Fatalf("delete annotation: %v", err)
	}
	if err := store.DeleteAnnotation(ctx, siteID, created.ID); !errors.Is(err, ErrAnnotationNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}

func TestListAnnotationsReturnsOverlappingNotes(t *testing.T) {
	store, userID, siteID := setupExclusionStore(t)
	defer store.Close()
	ctx := context.Background()

	d := func(day int) time.Time { return time.Date(2026, 9, day, 12, 0, 0, 0, time.UTC) }
	ptr := func(t time.Time) *time.Time { return &t }
	notes := []api.AnnotationInput{
		{StartsAt: d(1), Body: "before window"},
		{StartsAt: d(3), EndsAt: ptr(d(12)), Body: "starts before, ends inside"},
		{StartsAt: d(12), Body: "point inside"},
		{StartsAt: d(18), EndsAt: ptr(d(28)), Body: "starts inside, ends after"},
		{StartsAt: d(2), EndsAt: ptr(d(29)), Body: "spans whole window"},
		{StartsAt: d(29), Body: "after window"},
	}
	for _, n := range notes {
		if _, err := store.CreateAnnotation(ctx, siteID, n, userID); err != nil {
			t.Fatalf("create %q: %v", n.Body, err)
		}
	}

	got, err := store.ListAnnotations(ctx, siteID, d(10), d(20))
	if err != nil {
		t.Fatalf("list annotations: %v", err)
	}
	want := []string{"spans whole window", "starts before, ends inside", "point inside", "starts inside, ends after"}
	if len(got) != len(want) {
		t.Fatalf("got %d notes, want %d: %+v", len(got), len(want), got)
	}
	for i, body := range want {
		if got[i].Body != body {
			t.Fatalf("note %d = %q, want %q", i, got[i].Body, body)
		}
	}
}

func TestAnnotationSchemaRejectsInvalidRows(t *testing.T) {
	store, _, siteID := setupExclusionStore(t)
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	before := now.Add(-time.Hour)

	for name, input := range map[string]api.AnnotationInput{
		"empty body":       {StartsAt: now, Body: "   "},
		"too long body":    {StartsAt: now, Body: strings.Repeat("a", 281)},
		"end before start": {StartsAt: now, EndsAt: &before, Body: "x"},
	} {
		if _, err := store.CreateAnnotation(ctx, siteID, input, uuid.Nil); err == nil {
			t.Fatalf("%s: expected schema rejection", name)
		}
	}
}

func TestDeleteSiteRemovesAnnotations(t *testing.T) {
	store, userID, siteID := setupExclusionStore(t)
	defer store.Close()
	ctx := context.Background()

	note, err := store.CreateAnnotation(ctx, siteID, api.AnnotationInput{StartsAt: time.Now().UTC(), Body: "Outage"}, userID)
	if err != nil {
		t.Fatalf("create annotation: %v", err)
	}

	var createdBy *uuid.UUID
	if err := store.DB().QueryRowContext(ctx, `SELECT created_by FROM site_annotations WHERE id = ?`, note.ID).Scan(&createdBy); err != nil || createdBy == nil || *createdBy != userID {
		t.Fatalf("created_by before user delete = %v, %v", createdBy, err)
	}

	if err := store.DeleteSite(ctx, siteID); err != nil {
		t.Fatalf("delete site: %v", err)
	}
	assertTableCount(t, ctx, store, "site_annotations", "site_id", siteID, 0)
}

func TestDeleteUserKeepsTheirAnnotations(t *testing.T) {
	store, _, siteID := setupExclusionStore(t)
	defer store.Close()
	ctx := context.Background()

	authorID, err := store.CreateUser(ctx, "author@example.com", "hashed-secret")
	if err != nil {
		t.Fatalf("create author: %v", err)
	}
	note, err := store.CreateAnnotation(ctx, siteID, api.AnnotationInput{StartsAt: time.Now().UTC(), Body: "Release"}, authorID)
	if err != nil {
		t.Fatalf("create annotation: %v", err)
	}
	if err := store.DeleteUser(ctx, authorID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	var createdBy *uuid.UUID
	if err := store.DB().QueryRowContext(ctx, `SELECT created_by FROM site_annotations WHERE id = ?`, note.ID).Scan(&createdBy); err != nil {
		t.Fatalf("annotation should survive user deletion: %v", err)
	}
	if createdBy != nil {
		t.Fatalf("created_by after user delete = %v, want NULL", *createdBy)
	}
}
