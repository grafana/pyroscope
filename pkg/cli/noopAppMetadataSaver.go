// Package cli ... contains NoopMetadataSaver which should be removed
// after the funcionality is made GA (ie not feature flagged anymore)
package cli

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type NoopMetadataSaver struct{}

func (NoopMetadataSaver) CreateOrUpdate(_ context.Context, _ storage.Application) error {
	return nil
}
