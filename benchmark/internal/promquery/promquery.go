package promquery

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/pyroscope-io/pyroscope/benchmark/internal/config"
)

type PromQuery struct {
	Config *config.PromQuery
}

func New(cfg *config.PromQuery) *PromQuery {
	return &PromQuery{
		Config: cfg,
	}
}

func (pq *PromQuery) Instant(query string, t time.Time) (float64, error) {
	client, err := api.NewClient(api.Config{
		Address: pq.Config.PrometheusAddress,
	})

	if err != nil {
		return 0, err
	}

	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	v, _, err := v1api.Query(ctx, query, t)
	if err != nil {
		return 0, err
	}

	// TODO logrus
	//	if warning != nil {
	//		logrus.Warn(warning)
	//	}

	switch t := v.(type) {
	case model.Vector:
		vector := v.(model.Vector)
		if len(vector) <= 0 {
			return 0, fmt.Errorf("got 0 responses from prometheus")
		}
		return float64(vector[0].Value), nil
	case *model.Scalar:
		return float64(v.(*model.Scalar).Value), nil
	default:
		return 0, fmt.Errorf("invalid type %T", t)
	}
}
