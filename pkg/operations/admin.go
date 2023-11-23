package operations

import (
	"context"
	"net/http"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"

	"github.com/grafana/pyroscope/pkg/objstore"
)

type Admin struct {
	services.Service
	logger   log.Logger
	handlers *Handlers
}

func NewAdmin(bucketClient objstore.Bucket, logger log.Logger) (*Admin, error) {
	a := &Admin{
		logger: logger,
		handlers: &Handlers{
			Context: context.Background(),
			Logger:  logger,
			Bucket:  bucketClient,
		},
	}
	a.Service = services.NewBasicService(nil, a.running, nil)
	return a, nil
}
func (a *Admin) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (a *Admin) TenantsHandler(w http.ResponseWriter, r *http.Request) {
	a.handlers.CreateIndexHandler()(w, r)
}

func (a *Admin) BlocksHandler(w http.ResponseWriter, r *http.Request) {
	a.handlers.CreateBlocksHandler()(w, r)
}

func (a *Admin) BlockHandler(w http.ResponseWriter, r *http.Request) {
	a.handlers.CreateBlockDetailsHandler()(w, r)
}
