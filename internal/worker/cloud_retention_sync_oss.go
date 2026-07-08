//go:build !billing

package worker

import (
	"context"

	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
)

type CloudRetentionSyncWorker struct{}

func NewCloudRetentionSyncWorker(_ *database.TenantStoreManager, _ *entitlements.Service, _ *config.Config) *CloudRetentionSyncWorker {
	return &CloudRetentionSyncWorker{}
}

func (w *CloudRetentionSyncWorker) Start(_ context.Context) {}
