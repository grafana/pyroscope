package api

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/model"
)

type ApplicationService interface {
	List(context.Context) ([]model.Application, error)               // GET /apps
	Get(ctx context.Context, name string) (model.Application, error) // GET /apps/{name}
	CreateOrUpdate(context.Context, model.Application) error         // PUT /apps // Should be idempotent.
	Delete(ctx context.Context, name string) error                   // DELETE /apps/{name}
}
