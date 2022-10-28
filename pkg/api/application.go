package api

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type ApplicationService interface {
	List(context.Context) ([]storage.Application, error)               // GET /apps
	Get(ctx context.Context, name string) (storage.Application, error) // GET /apps/{name}
	CreateOrUpdate(context.Context, storage.Application) error         // PUT /apps // Should be idempotent.
	Delete(ctx context.Context, name string) error                     // DELETE /apps/{name}
}
