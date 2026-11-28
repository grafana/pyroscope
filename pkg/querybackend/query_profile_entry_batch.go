package querybackend

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"

	"github.com/grafana/pyroscope/v2/pkg/iter"
	parquetquery "github.com/grafana/pyroscope/v2/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
)

// Independent of both the async ProfileEntry queue and repeated sample buffers.
const profileFilterBatchSize = 256

func batchProfileEntryIterator(q *queryContext, series map[uint32]series, opts iteratorOpts) (iter.Iterator[ProfileEntry], bool, error) {
	// Explicit ID selection has higher priority than series/time in the scalar
	// plan. Retain that plan instead of scanning filters before a selective ID.
	if len(opts.profileIDSelector) > 0 {
		return nil, false, nil
	}
	profiles := q.ds.Profiles()
	if opts.fetchProfileIDs {
		id, ok := profiles.Schema().Lookup(schemav1.IDColumnName)
		if !ok || id.MaxDefinitionLevel != 0 || id.MaxRepetitionLevel != 0 || id.Node.Type().Kind() != parquet.FixedLenByteArray {
			return nil, false, nil
		}
	}
	si, _ := parquetquery.GetColumnIndexByPath(profiles.Root(), schemav1.SeriesIndexColumnName)
	ti, _ := parquetquery.GetColumnIndexByPath(profiles.Root(), schemav1.TimeNanosColumnName)
	var partitions []int
	if opts.fetchPartition {
		pi, _ := parquetquery.GetColumnIndexByPath(profiles.Root(), schemav1.StacktracePartitionColumnName)
		partitions = []int{pi}
	}
	matches := make(map[uint32]struct{}, len(series))
	for id := range series {
		matches[id] = struct{}{}
	}
	scanner, err := parquetquery.NewProfileBatchScanner(q.ctx, profiles.RowGroups(), si, ti, profileFilterBatchSize, matches, q.req.startTime, q.req.endTime, partitions...)
	if errors.Is(err, parquetquery.ErrUnsupportedProfileBatchSchema) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	entries := &batchProfileEntries{scanner: scanner, series: series}
	if opts.fetchProfileIDs && len(profiles.RowGroups()) > 0 {
		entries.ids = profiles.Column(q.ctx, schemav1.IDColumnName, nil)
	}
	// Preserve the existing async queue/backpressure boundary. The input adapter
	// expands typed batches, not IteratorResults or per-row filter-column joins.
	return iter.NewAsyncBatchIterator[ProfileEntry, ProfileEntry](entries, bigBatchSize,
		func(e ProfileEntry) ProfileEntry { return e }, func([]ProfileEntry) {}), true, nil
}

type batchProfileEntries struct {
	scanner  *parquetquery.ProfileBatchScanner
	series   map[uint32]series
	ids      parquetquery.Iterator
	batch    parquetquery.ProfileBatch
	position int
	entry    ProfileEntry
	err      error
	closed   bool
}

func (i *batchProfileEntries) Next() bool {
	if i.closed || i.err != nil {
		return false
	}
	if i.position == len(i.batch.RowNumbers) {
		if !i.scanner.Next() {
			i.err = i.scanner.Err()
			return false
		}
		i.batch = i.scanner.At()
		i.position = 0
	}
	n := i.position
	i.position++
	row := i.batch.RowNumbers[n]
	info := i.series[i.batch.SeriesIndexes[n]]
	i.entry = ProfileEntry{RowNum: row, Timestamp: model.TimeFromUnixNano(i.batch.TimeNanos[n]), Fingerprint: info.fingerprint, Labels: info.labels}
	if i.batch.Partitions != nil {
		i.entry.Partition = i.batch.Partitions[n]
	}
	if i.ids != nil {
		target := parquetquery.EmptyRowNumber()
		target[0] = row
		if !i.ids.Seek(parquetquery.RowNumberWithDefinitionLevel{RowNumber: target, DefinitionLevel: 0}) {
			i.err = i.ids.Err()
			if i.err == nil {
				i.err = fmt.Errorf("missing profile ID at row %d", row)
			}
			return false
		}
		result := i.ids.At()
		if result.RowNumber[0] != row {
			i.err = fmt.Errorf("missing profile ID at row %d", row)
			return false
		}
		value := result.ColumnValue(schemav1.IDColumnName).Bytes()
		// Match the scalar projection's treatment of non-UUID values.
		if len(value) == 16 {
			var id uuid.UUID
			copy(id[:], value)
			i.entry.ID = id.String()
		}
	}
	return true
}
func (i *batchProfileEntries) At() ProfileEntry { return i.entry }
func (i *batchProfileEntries) Err() error       { return i.err }
func (i *batchProfileEntries) Close() error {
	if i.closed {
		return nil
	}
	i.closed = true
	err := i.scanner.Close()
	if i.ids != nil {
		err = errors.Join(err, i.ids.Close())
	}
	i.batch = parquetquery.ProfileBatch{}
	i.entry = ProfileEntry{}
	i.series = nil
	return err
}
