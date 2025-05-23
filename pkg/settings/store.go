package settings

import (
	"context"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

type store interface {
	// Get settings for a tenant.
	Get(ctx context.Context, tenantID string) ([]*settingsv1.Setting, error)

	// Set a setting for a tenant.
	Set(ctx context.Context, tenantID string, setting *settingsv1.Setting) (*settingsv1.Setting, error)

	// Delete a setting for a tenant.
	Delete(ctx context.Context, tenantID string, name string, modifiedAtMs int64) error

	// Flush the store to disk.
	Flush(ctx context.Context) error

	// Close the store.
	Close() error
}
