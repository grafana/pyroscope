package adhoc

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type NoopMetadataSaver struct{}

func (NoopMetadataSaver) CreateOrUpdate(ctx context.Context, application storage.Application) error {
	return nil
}
