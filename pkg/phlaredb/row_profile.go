package phlaredb

import (
	"context"
	"fmt"

	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

type RowProfile struct {
	Timestamp int64

	Labels      phlaremodel.Labels
	Fingerprint model.Fingerprint
	*query.IteratorResult
}

func (r RowProfile) RowNumber() int64 {
	return r.IteratorResult.RowNumber[0]
}

type LabelGrouper interface {
	ForSeries()
}

type RowProfileIterator struct {
	rows iter.SeekIterator[*query.IteratorResult, query.RowNumberWithDefinitionLevel]

	currSeriesIndex int64
	series          map[int64]struct {
		labels phlaremodel.Labels
		fp     model.Fingerprint
	}
}

func (it *RowProfileIterator) Next() bool {
	return it.rows.Next()
}

func (it *RowProfileIterator) At() RowProfile {
	return RowProfile{}
}

func (it *RowProfileIterator) Err() error {
	return it.rows.Close()
}

func (it *RowProfileIterator) Close() error {
	return it.rows.Close()
}

// . todo custom parsing of labels  and grouping and skipping
func SelectProfiles(ctx context.Context, b BlockReader, matchers []*labels.Matcher, start, end model.Time, columns []string) (iter.Iterator[RowProfile], error) {
	postings, err := PostingsForMatchers(b.Index(), nil, matchers...)
	if err != nil {
		return nil, err
	}

	var (
		chks       = make([]index.ChunkMeta, 1)
		lblsPerRef = make(map[int64]struct{})
	)

	// get all relevant labels/fingerprints
	for postings.Next() {
		_, err := b.Index().Series(postings.At(), nil, &chks)
		if err != nil {
			return nil, err
		}
		lblsPerRef[int64(chks[0].SeriesIndex)] = struct{}{}
	}

	lhs, err := columnIter(ctx, b.Profiles(), "SeriesIndex", query.NewMapPredicate(lblsPerRef))
	if err != nil {
		return nil, err
	}
	rhs, err := columnIter(ctx, b.Profiles(), "TimeNanos", query.NewIntBetweenPredicate(start.UnixNano(), end.UnixNano()))
	if err != nil {
		return nil, err
	}

	return &RowProfileIterator{
		rows: query.NewBinaryJoinIterator(
			0,
			lhs,
			rhs,
		),
	}, nil
}

func columnIter(ctx context.Context, reader ProfileReader, columnName string, predicate query.Predicate) (query.Iterator, error) {
	index, _ := query.GetColumnIndexByPath(reader.Root(), columnName)
	if index == -1 {
		return nil, fmt.Errorf("column '%s' not found in parquet schema '%s'", columnName, reader.Schema())
	}
	return query.NewSyncIterator(ctx, reader.RowGroups(), index, columnName, 1000, predicate, columnName), nil
}

// func (ps *ProfileSelector) WithColumn(, new func(v parquet.Value)) *ProfileSelector {
// 	ps.column = append(ps.column, column)
// 	return ps
// }

// func (r *parquetReader[M, P]) columnIter(ctx context.Context, columnName string, predicate query.Predicate, alias string) query.Iterator {
// 	index, _ := query.GetColumnIndexByPath(r.file.File, columnName)
// 	if index == -1 {
// 		return query.NewErrIterator(fmt.Errorf("column '%s' not found in parquet file '%s'", columnName, r.relPath()))
// 	}
// 	ctx = query.AddMetricsToContext(ctx, r.metrics.query)
// 	return query.NewSyncIterator(ctx, r.file.RowGroups(), index, columnName, 1000, predicate, alias)
// }
