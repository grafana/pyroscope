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
	// Timestamp int64

	Labels      phlaremodel.Labels
	Fingerprint model.Fingerprint
	*query.IteratorResult
}

func (r RowProfile) RowNumber() int64 {
	return r.IteratorResult.RowNumber[0]
}

type RowProfileIterator struct {
	rows iter.SeekIterator[*query.IteratorResult, query.RowNumberWithDefinitionLevel]

	includeLbls     bool
	currSeriesIndex int64
	series          map[int64]labelsInfo
}

func (it *RowProfileIterator) Next() bool {
	if it.rows.Next() {
		return true
	}
	return false
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

type LabelFilter func(name string) bool

var AllLabel = func(name string) bool { return false }

type labelsInfo struct {
	fp  model.Fingerprint
	lbs phlaremodel.Labels
}

// . todo custom parsing of labels  and grouping and skipping
func SelectProfiles(ctx context.Context, b BlockReader, matchers []*labels.Matcher, start, end model.Time, f LabelFilter) (iter.Iterator[RowProfile], error) {
	postings, err := PostingsForMatchers(b.Index(), nil, matchers...)
	if err != nil {
		return nil, err
	}

	var (
		chks        = make([]index.ChunkMeta, 1)
		lblsPerRef  = make(map[int64]labelsInfo)
		lbls        = make(phlaremodel.Labels, 0, 6)
		includeLbls = f != nil
		fp          uint64
	)

	// get all relevant labels/fingerprints
	for postings.Next() {
		if includeLbls {
			// todo: include filter in index's Series call
			fp, err = b.Index().Series(postings.At(), &lbls, &chks)
		} else {
			fp, err = b.Index().Series(postings.At(), nil, &chks)
		}
		if err != nil {
			return nil, err
		}
		_, ok := lblsPerRef[int64(chks[0].SeriesIndex)]
		if !ok {
			info := labelsInfo{}
			if includeLbls {
				info.lbs = make(phlaremodel.Labels, len(lbls))
				copy(info.lbs, lbls)
			}
			info.fp = model.Fingerprint(fp)
			lblsPerRef[int64(chks[0].SeriesIndex)] = info
		}
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
		includeLbls:     includeLbls,
		currSeriesIndex: -1,
		series:          lblsPerRef,
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
