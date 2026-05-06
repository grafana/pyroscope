package block

import (
	"context"
	"io"
	"sort"
	"sync"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"

	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/tsdb/index"
	memindex "github.com/grafana/pyroscope/v2/pkg/segmentwriter/memdb/index"
)

// datasetIndexEncBufferSize is the initial capacity hint for the
// underlying TSDB index writer's scratch encoding buffers. Dataset
// indexes have one entry per dataset within a tenant block, so a small
// hint is plenty; the buffers grow if needed.
const datasetIndexEncBufferSize = 1 << 14

// DatasetIndexWriter builds the per-tenant dataset TSDB index.
//
// The dataset index resembles the dataset (series) TSDB index, but uses
// the dataset position within the block (instead of the series ID) as the
// chunk SeriesIndex. The query backend uses this index to identify which
// datasets within a block match a given label selector without opening
// each dataset's TSDB index individually.
//
// Series are added one-at-a-time via AddSeries with an explicit dataset
// index. Callers that observe series row-by-row (e.g. compaction) are
// expected to deduplicate by fingerprint before calling AddSeries.
//
// Writers are pooled. Obtain one with NewDatasetIndexWriter and return
// it with Close once WriteTo has been called.
type DatasetIndexWriter struct {
	series  []seriesLabels
	chunks  []index.ChunkMeta
	symbols map[string]struct{}
}

var datasetIndexWriterPool = sync.Pool{
	New: func() any {
		return &DatasetIndexWriter{symbols: make(map[string]struct{})}
	},
}

// NewDatasetIndexWriter returns a DatasetIndexWriter ready for use,
// reusing one from a pool when possible. Callers must call Close to
// return it once they are done.
func NewDatasetIndexWriter() *DatasetIndexWriter {
	return datasetIndexWriterPool.Get().(*DatasetIndexWriter)
}

// Close returns the writer to the pool. After Close, the writer must
// not be used.
func (rw *DatasetIndexWriter) Close() {
	rw.reset()
	datasetIndexWriterPool.Put(rw)
}

func (rw *DatasetIndexWriter) reset() {
	rw.series = rw.series[:0]
	rw.chunks = rw.chunks[:0]
	clear(rw.symbols)
}

// AddSeries adds a single series at the given dataset index. The caller
// is responsible for ensuring fingerprints are unique within the
// writer (e.g. by deduplicating consecutive rows with the same
// fingerprint at compaction time).
func (rw *DatasetIndexWriter) AddSeries(idx uint32, labels phlaremodel.Labels, fp model.Fingerprint) {
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
		SeriesIndex: idx,
	})
}

// Empty reports whether the writer has no series.
func (rw *DatasetIndexWriter) Empty() bool { return len(rw.series) == 0 }

// WriteTo encodes the accumulated series into a TSDB index and writes
// the encoded bytes directly to dst, returning the number of bytes
// written. After WriteTo, the accumulated state is reset; the writer
// can be reused for another tenant or returned to the pool via Close.
func (rw *DatasetIndexWriter) WriteTo(dst io.Writer) (int64, error) {
	if len(rw.series) == 0 {
		return 0, nil
	}

	w, err := memindex.NewWriter(context.Background(), datasetIndexEncBufferSize)
	if err != nil {
		return 0, err
	}

	symbols := make([]string, 0, len(rw.symbols))
	for s := range rw.symbols {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	for _, symbol := range symbols {
		if err = w.AddSymbol(symbol); err != nil {
			return 0, err
		}
	}

	for i, series := range rw.series {
		if err = w.AddSeries(storage.SeriesRef(i), series.labels, series.fingerprint, rw.chunks[i]); err != nil {
			return 0, err
		}
	}

	if err = w.Close(); err != nil {
		return 0, err
	}

	bw := w.ReleaseIndexBuffer()
	defer memindex.PutBufferWriterToPool(bw)
	rw.reset()

	buf, _, _ := bw.Buffer()
	n, err := dst.Write(buf)
	return int64(n), err
}
