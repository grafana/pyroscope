package promquery

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/pyroscope-io/pyroscope/benchmark/config"
)

type promQuery struct {
	Config *config.PromQuery
}

func New(cfg *config.PromQuery) *promQuery {
	return &promQuery{
		Config: cfg,
	}
}

func (pq *promQuery) Instant(query string, t time.Time) (model.Value, v1.Warnings, error) {
	client, err := api.NewClient(api.Config{
		Address: pq.Config.PrometheusAddress,
	})

	if err != nil {
		return nil, nil, err
	}

	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return v1api.Query(ctx, query, t)
}
