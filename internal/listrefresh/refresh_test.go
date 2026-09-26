package listrefresh

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"hitkeep/internal/aianalytics"
	"hitkeep/internal/blocking/spamfeed"
)

func TestAIUnchangedTimestampPreservesFileAndMetadataChangeWrites(t *testing.T) {
	original, err := aianalytics.LoadEmbeddedAIAgentData()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ai.json")
	if err := aianalytics.SaveAIAgentData(path, original); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(100, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	candidate := original
	candidate.GeneratedAt = original.GeneratedAt.Add(time.Hour)
	validated := false
	got, changed, err := run(context.Background(), path,
		func(context.Context) (aianalytics.AIAgentData, error) { return candidate, nil },
		func(data aianalytics.AIAgentData) error {
			validated = true
			return aianalytics.ValidateEmbeddedAIAgentData(data)
		},
		aianalytics.LoadAIAgentData, aianalytics.SaveAIAgentData, sameAI,
		"fetch", "validate", "save")
	if err != nil || changed || !validated || !reflect.DeepEqual(got, candidate) {
		t.Fatalf("no-op: changed=%v validated=%v err=%v", changed, validated, err)
	}
	assertUntouched(t, path, before, stamp)
	candidate.SourceMetadata["test"] = aianalytics.AIAgentSourceMetadata{License: "changed"}
	_, changed, err = run(context.Background(), path,
		func(context.Context) (aianalytics.AIAgentData, error) { return candidate, nil },
		aianalytics.ValidateEmbeddedAIAgentData, aianalytics.LoadAIAgentData, aianalytics.SaveAIAgentData, sameAI,
		"fetch", "validate", "save")
	if err != nil || !changed {
		t.Fatalf("metadata change: changed=%v err=%v", changed, err)
	}
}

func TestSpamUnchangedTimestampPreservesFileAndEntryChangeWrites(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "blocking", "default_spam_filter.json"))
	if err != nil {
		t.Fatal(err)
	}
	original, err := spamfeed.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "spam.json")
	if err := spamfeed.SaveSpamFeedData(path, original); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(100, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	candidate := original
	candidate.GeneratedAt = original.GeneratedAt.Add(time.Hour)
	validated := false
	_, changed, err := run(context.Background(), path,
		func(context.Context) (spamfeed.SpamFeedData, error) { return candidate, nil },
		func(data spamfeed.SpamFeedData) error {
			validated = true
			return spamfeed.ValidateEmbeddedSpamFeedData(data)
		},
		spamfeed.LoadSpamFeedData, spamfeed.SaveSpamFeedData, sameSpam,
		"fetch", "validate", "save")
	if err != nil || changed || !validated {
		t.Fatalf("no-op: changed=%v validated=%v err=%v", changed, validated, err)
	}
	assertUntouched(t, path, before, stamp)
	candidate.ReferrerHostDenylist = append(append([]string(nil), candidate.ReferrerHostDenylist...), "new.example")
	_, changed, err = run(context.Background(), path,
		func(context.Context) (spamfeed.SpamFeedData, error) { return candidate, nil },
		spamfeed.ValidateEmbeddedSpamFeedData, spamfeed.LoadSpamFeedData, spamfeed.SaveSpamFeedData, sameSpam,
		"fetch", "validate", "save")
	if err != nil || !changed {
		t.Fatalf("entry change: changed=%v err=%v", changed, err)
	}
}

func TestRunRejectsCorruptExistingAndCanceledFetch(t *testing.T) {
	data, err := aianalytics.LoadEmbeddedAIAgentData()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ai.json")
	if err := os.WriteFile(path, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	fetch := func(context.Context) (aianalytics.AIAgentData, error) { return data, nil }
	_, changed, err := run(context.Background(), path, fetch, aianalytics.ValidateEmbeddedAIAgentData,
		aianalytics.LoadAIAgentData, aianalytics.SaveAIAgentData, sameAI, "fetch", "validate", "save")
	if err == nil || changed {
		t.Fatalf("corrupt output: changed=%v err=%v", changed, err)
	}
	if raw, readErr := os.ReadFile(path); readErr != nil || string(raw) != "corrupt" {
		t.Fatalf("corrupt output changed: %q, %v", raw, readErr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	missing := filepath.Join(t.TempDir(), "missing.json")
	_, changed, err = run(ctx, missing, fetch, aianalytics.ValidateEmbeddedAIAgentData,
		aianalytics.LoadAIAgentData, aianalytics.SaveAIAgentData, sameAI, "fetch", "validate", "save")
	if err == nil || changed {
		t.Fatalf("canceled refresh: changed=%v err=%v", changed, err)
	}
	if _, statErr := os.Stat(missing); !os.IsNotExist(statErr) {
		t.Fatalf("canceled refresh wrote output: %v", statErr)
	}
	_, changed, err = run(context.Background(), missing, fetch, aianalytics.ValidateEmbeddedAIAgentData,
		aianalytics.LoadAIAgentData, aianalytics.SaveAIAgentData, sameAI, "fetch", "validate", "save")
	if err != nil || !changed {
		t.Fatalf("missing output: changed=%v err=%v", changed, err)
	}
}

func TestRunDoesNotReportNoOpAfterCancellation(t *testing.T) {
	data, err := aianalytics.LoadEmbeddedAIAgentData()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ai.json")
	if err := aianalytics.SaveAIAgentData(path, data); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, changed, err := run(ctx, path,
		func(context.Context) (aianalytics.AIAgentData, error) { return data, nil },
		func(data aianalytics.AIAgentData) error { cancel(); return nil },
		aianalytics.LoadAIAgentData, aianalytics.SaveAIAgentData, sameAI, "fetch", "validate", "save")
	if err == nil || changed {
		t.Fatalf("canceled no-op reported success: changed=%v err=%v", changed, err)
	}
}

func assertUntouched(t *testing.T, path string, before []byte, stamp time.Time) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || !info.ModTime().Equal(stamp) || info.Mode().Perm() != 0o640 {
		t.Fatalf("unchanged data rewrote file: bytes=%v mtime=%v mode=%04o", string(after) == string(before), info.ModTime(), info.Mode().Perm())
	}
}
