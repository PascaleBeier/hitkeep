package hitkeepcmd

import (
	"strings"
	"testing"

	"hitkeep/internal/config"
)

func TestValidateLiveDatabasePathsRejectsS3(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		conf config.Config
	}{
		{name: "control", conf: config.Config{DBPath: "s3://bucket/hitkeep.db", DataPath: "data"}},
		{name: "tenant data", conf: config.Config{DBPath: "hitkeep.db", DataPath: "s3://bucket/data"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLiveDatabasePaths(&tc.conf)
			if err == nil || !strings.Contains(err.Error(), "HITKEEP_BACKUP_PATH") {
				t.Fatalf("expected remote live database path to be rejected, got %v", err)
			}
		})
	}
	if err := validateLiveDatabasePaths(&config.Config{DBPath: "hitkeep.db", DataPath: "data"}); err != nil {
		t.Fatalf("expected local live database paths to be accepted: %v", err)
	}
}
