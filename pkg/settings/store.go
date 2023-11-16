package settings

import (
	"context"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

type Store interface {
	All(ctx context.Context, tenantIDs ...string) ([]*settingsv1.Setting, error)
	Set(ctx context.Context, setting *settingsv1.Setting, tenantIDs ...string) (*settingsv1.Setting, error)
}
