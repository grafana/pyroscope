package storage

import (
	"context"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type QueryExemplarsInput struct {
	Query     *flameql.Query
	StartTime time.Time
	EndTime   time.Time

	MinValue uint64
	MaxValue uint64

	HeatmapParams HeatmapParams
}

type QueryExemplarsOutput struct {
	Tree          *tree.Tree
	Count         uint64
	Metadata      metadata.Metadata
	HeatmapSketch HeatmapSketch

	Telemetry map[string]interface{}
}

func (*Storage) QueryExemplars(context.Context, QueryExemplarsInput) (QueryExemplarsOutput, error) {
	// FIXME(kolesnikovae): Not implemented.
	return QueryExemplarsOutput{Tree: tree.New()}, nil
}
