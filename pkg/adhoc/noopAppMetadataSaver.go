package adhoc

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type NoopMetadataSaver struct{}

func (NoopMetadataSaver) CreateOrUpdate(_ context.Context, _ storage.Application) error {
	return nil
}
