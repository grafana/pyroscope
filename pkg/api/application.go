package api

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type ApplicationService interface {
	List(context.Context) ([]storage.ApplicationMetadata, error)               // GET /apps
	Get(ctx context.Context, name string) (storage.ApplicationMetadata, error) // GET /apps/{name}
	CreateOrUpdate(context.Context, storage.ApplicationMetadata) error         // PUT /apps // Should be idempotent.
	Delete(ctx context.Context, name string) error                             // DELETE /apps/{name}
}
