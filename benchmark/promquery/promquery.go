package promquery

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
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

func (pq *promQuery) Instant(query string, t time.Time) (error, string, string) {
	client, err := api.NewClient(api.Config{
		Address: pq.Config.PrometheusAddress,
	})

	if err != nil {
		return err, "", ""
	}

	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, warnings, err := v1api.Query(ctx, query, t)
	if err != nil {
		return err, "", ""
	}

	return nil, fmt.Sprintf("Warnings: %v\n", warnings), fmt.Sprintf("Result:\n%v\n", result)
}
