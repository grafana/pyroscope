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

	MinValue            uint64
	MaxValue            uint64
	HeatmapTimeBuckets  uint64
	HeatmapValueBuckets uint64
}

type QueryExemplarsOutput struct {
	Tree     *tree.Tree
	Count    uint64
	Heatmap  *Heatmap
	Metadata metadata.Metadata

	Telemetry map[string]interface{}
}

func (s *Storage) QueryExemplars(context.Context, QueryExemplarsInput) (QueryExemplarsOutput, error) {
	// FIXME: Not implemented.
	return QueryExemplarsOutput{Tree: tree.New()}, nil
}
