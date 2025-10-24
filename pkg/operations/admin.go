package operations

import (
	"context"
	"net/http"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"

	"github.com/grafana/pyroscope/pkg/objstore"
)

type Admin struct {
	services.Service
	logger   log.Logger
	handlers *Handlers
}

func NewAdmin(bucketClient objstore.Bucket, logger log.Logger, maxBlockDuration time.Duration) (*Admin, error) {
	a := &Admin{
		logger: logger,
		handlers: &Handlers{
			Logger:           logger,
			Bucket:           bucketClient,
			MaxBlockDuration: maxBlockDuration,
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
	http.Error(w, "Dataset details not available in v1 storage", http.StatusNotFound)
}

func (a *Admin) DatasetProfilesHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Dataset profiles not available in v1 storage", http.StatusNotFound)
}

func (a *Admin) ProfileDownloadHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Profile download not available in v1 storage", http.StatusNotFound)
}

func (a *Admin) ProfileCallTreeHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Profile call tree not available in v1 storage", http.StatusNotFound)
}

func (a *Admin) DatasetTSDBIndexHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Dataset TSDB index not available in v1 storage", http.StatusNotFound)
}

func (a *Admin) DatasetSymbolsHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Dataset symbols not available in v1 storage", http.StatusNotFound)
}
