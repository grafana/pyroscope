package v2

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

func NewAdmin(metastoreClient MetastoreClient, bucket objstore.Bucket, logger log.Logger) (*Admin, error) {
	a := &Admin{
		logger: logger,
		handlers: &Handlers{
			Logger:          logger,
			MetastoreClient: metastoreClient,
			Bucket:          bucket,
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

func (a *Admin) DatasetHandler(w http.ResponseWriter, r *http.Request) {
	a.handlers.CreateDatasetDetailsHandler()(w, r)
}

func (a *Admin) DatasetProfilesHandler(w http.ResponseWriter, r *http.Request) {
	a.handlers.CreateDatasetProfilesHandler()(w, r)
}

func (a *Admin) ProfileDownloadHandler(w http.ResponseWriter, r *http.Request) {
	a.handlers.CreateDatasetProfileDownloadHandler()(w, r)
}

func (a *Admin) ProfileCallTreeHandler(w http.ResponseWriter, r *http.Request) {
	a.handlers.CreateDatasetProfileCallTreeHandler()(w, r)
}

func (a *Admin) DatasetTSDBIndexHandler(w http.ResponseWriter, r *http.Request) {
	a.handlers.CreateDatasetTSDBIndexHandler()(w, r)
}

func (a *Admin) DatasetSymbolsHandler(w http.ResponseWriter, r *http.Request) {
	a.handlers.CreateDatasetSymbolsHandler()(w, r)
}
