package block

import (
	"context"
	"sort"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"

	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/tsdb/index"
	memindex "github.com/grafana/pyroscope/v2/pkg/segmentwriter/memdb/index"
)

// DatasetIndexWriter builds the per-tenant dataset TSDB index.
//
// The dataset index resembles the dataset (series) TSDB index, but uses
// the dataset position within the block (instead of the series ID) as the
// chunk SeriesIndex. The query backend uses this index to identify which
// datasets within a block match a given label selector without opening
// each dataset's TSDB index individually.
//
// It is used both at compaction time (where series labels are discovered
// row-by-row via WriteRow) and at segment flush time (where series are
// already known and added in bulk via AddSeries).
type DatasetIndexWriter struct {
	series   []seriesLabels
	chunks   []index.ChunkMeta
	previous model.Fingerprint
	symbols  map[string]struct{}
	idx      uint32
	buf      []byte
}

func NewDatasetIndexWriter() *DatasetIndexWriter {
	return &DatasetIndexWriter{
		symbols: make(map[string]struct{}),
	}
}

// SetIndex sets the dataset index assigned to subsequent series added
// via WriteRow or AddSeries.
func (rw *DatasetIndexWriter) SetIndex(i uint32) { rw.idx = i }

// WriteRow ingests a profile row, deduplicating series by fingerprint.
// It is used by compaction, which iterates rows in series order.
func (rw *DatasetIndexWriter) WriteRow(e ProfileEntry) {
	if rw.previous != e.Fingerprint || len(rw.series) == 0 {
		rw.addSeries(e.Labels, e.Fingerprint)
		rw.previous = e.Fingerprint
	}
}

// AddSeries adds a single series at the current dataset index. The caller
// is responsible for ensuring fingerprints are unique within the dataset.
// It is used by the segment writer, where heads already provide a
// deduplicated series list.
func (rw *DatasetIndexWriter) AddSeries(labels phlaremodel.Labels, fp model.Fingerprint) {
	rw.addSeries(labels, fp)
}

func (rw *DatasetIndexWriter) addSeries(labels phlaremodel.Labels, fp model.Fingerprint) {
	cloned := labels.Clone()
	for _, l := range cloned {
		rw.symbols[l.Name] = struct{}{}
		rw.symbols[l.Value] = struct{}{}
	}
	rw.series = append(rw.series, seriesLabels{
		labels:      cloned,
		fingerprint: fp,
	})
	rw.chunks = append(rw.chunks, index.ChunkMeta{
		SeriesIndex: rw.idx,
	})
}

// Empty reports whether the writer has no series.
func (rw *DatasetIndexWriter) Empty() bool { return len(rw.series) == 0 }

// Buf returns the serialised index bytes. Only valid after Flush.
func (rw *DatasetIndexWriter) Buf() []byte { return rw.buf }

func (rw *DatasetIndexWriter) Flush() error {
	// TODO(kolesnikovae):
	//  * Estimate size.
	//  * Use buffer pool.
	w, err := memindex.NewWriter(context.Background(), 1<<20)
	if err != nil {
		return err
	}

	// Sort symbols
	symbols := make([]string, 0, len(rw.symbols))
	for s := range rw.symbols {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	// Add symbols
	for _, symbol := range symbols {
		if err = w.AddSymbol(symbol); err != nil {
			return err
		}
	}

	// Add Series
	for i, series := range rw.series {
		if err = w.AddSeries(storage.SeriesRef(i), series.labels, series.fingerprint, rw.chunks[i]); err != nil {
			return err
		}
	}

	err = w.Close()
	rw.buf = w.ReleaseIndex()
	return err
}
