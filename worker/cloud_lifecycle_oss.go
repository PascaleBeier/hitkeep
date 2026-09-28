//go:build !billing

package worker

import (
	"context"

	"hitkeep/config"
	"hitkeep/database"
	"hitkeep/mailer"
)

type CloudLifecycleWorker struct{}

func NewCloudLifecycleWorker(_ *database.TenantStoreManager, _ *mailer.Mailer, _ *config.Config) *CloudLifecycleWorker {
	return &CloudLifecycleWorker{}
}

func (w *CloudLifecycleWorker) Start(_ context.Context) {}
