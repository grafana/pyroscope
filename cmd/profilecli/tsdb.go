package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prometheus/prometheus/model/labels"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
)

func tsdbSeries(ctx context.Context, path string) error {
	r, err := index.NewFileReader(path)
	if err != nil {
		return err
	}

	it, err := phlaredb.PostingsForMatchers(r, nil, labels.MustNewMatcher(labels.MatchNotEqual, "__name__", ""))
	if err != nil {
		return err
	}

	var (
		lbls      phlaremodel.Labels
		chunkMeta []index.ChunkMeta
	)
	line := struct {
		SeriesRef   uint64
		SeriesIndex *uint32
		Labels      json.RawMessage
	}{}
	enc := json.NewEncoder(output(ctx))

	for it.Next() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		_, err = r.Series(it.At(), &lbls, &chunkMeta)
		if err != nil {
			return fmt.Errorf("error retrieving seriesRef: %w", err)
		}

		line.Labels, err = json.Marshal(lbls)
		if err != nil {
			return fmt.Errorf("error marshalling labels: %w", err)
		}

		if len(chunkMeta) > 0 {
			line.SeriesIndex = &chunkMeta[0].SeriesIndex
		} else {
			line.SeriesIndex = nil
		}
		line.SeriesRef = uint64(it.At())
		if err := enc.Encode(&line); err != nil {
			return fmt.Errorf("error writing line: %w", err)
		}
	}

	return nil
}
