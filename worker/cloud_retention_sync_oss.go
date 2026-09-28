//go:build !billing

package worker

import (
	"context"

	"hitkeep/config"
	"hitkeep/database"
	"hitkeep/entitlements"
)

type CloudRetentionSyncWorker struct{}

func NewCloudRetentionSyncWorker(_ *database.TenantStoreManager, _ *entitlements.Service, _ *config.Config) *CloudRetentionSyncWorker {
	return &CloudRetentionSyncWorker{}
}

func (w *CloudRetentionSyncWorker) Start(_ context.Context) {}
