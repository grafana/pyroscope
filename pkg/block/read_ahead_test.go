package block

import (
	"bytes"
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
)

type readAheadTestRow struct {
	Series int64   `parquet:"series"`
	Time   int64   `parquet:"time"`
	Values []int64 `parquet:"values,list"`
}

type recordingBucket struct {
	objstore.Bucket
	delay  time.Duration
	ranges atomic.Int64
}

func (b *recordingBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	b.ranges.Add(1)
	if b.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(b.delay):
		}
	}
	return b.Bucket.GetRange(ctx, name, off, length)
}

const (
	readAheadTestPath      = "profiles.parquet"
	readAheadTestRowGroups = 4
	readAheadTestColumns   = 3
)

func newReadAheadTestFile(t *testing.T, delay time.Duration) (*ParquetFile, *recordingBucket) {
	rows := make([]readAheadTestRow, 4000)
	for i := range rows {
		rows[i] = readAheadTestRow{Series: int64(i / 100), Time: int64(i)}
		for j := 0; j < 1+i%32; j++ {
			rows[i].Values = append(rows[i].Values, int64(i*j))
		}
	}
	var buf bytes.Buffer
	createParquetFile(t, &buf, rows, readAheadTestRowGroups)

	mem := memory.NewInMemBucket()
	mem.Set(readAheadTestPath, buf.Bytes())
	bucket := &recordingBucket{Bucket: objstore.NewBucket(mem), delay: delay}
	f, err := openParquetFile(context.Background(),
		&objstore.ReaderAtBucket{Bucket: bucket}, readAheadTestPath, 0, int64(buf.Len()),
		estimateFooterSize(int64(buf.Len())),
		parquet.SkipBloomFilters(true),
		parquet.FileReadMode(parquet.ReadModeAsync),
		// Small buffers make the pages of a column chunk be read
		// in many requests, as large chunks are.
		parquet.ReadBufferSize(1<<10))
	require.NoError(t, err)
	f.remote = true
	t.Cleanup(func() { _ = f.Close() })
	require.Len(t, f.RowGroups(), readAheadTestRowGroups)
	require.Len(t, f.RowGroups()[0].ColumnChunks(), readAheadTestColumns)
	bucket.ranges.Store(0)
	return f, bucket
}

// readColumns reads the row groups in order, and in each the given columns,
// as the query iterators do.
func readColumns(t *testing.T, rowGroups []parquet.RowGroup, columns ...int) [][]int64 {
	values := make([][]int64, len(columns))
	for _, rg := range rowGroups {
		for i, col := range columns {
			pages := rg.ColumnChunks()[col].Pages()
			for {
				p, err := pages.ReadPage()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				buf := make([]parquet.Value, p.NumValues())
				n, err := p.Values().ReadValues(buf)
				if err != io.EOF {
					require.NoError(t, err)
				}
				for _, v := range buf[:n] {
					values[i] = append(values[i], v.Int64())
				}
				parquet.Release(p)
			}
			require.NoError(t, pages.Close())
		}
	}
	return values
}

func allColumns() []int {
	columns := make([]int, readAheadTestColumns)
	for i := range columns {
		columns[i] = i
	}
	return columns
}

func TestReadAhead(t *testing.T) {
	f, bucket := newReadAheadTestFile(t, 0)
	expected := readColumns(t, f.RowGroups(), allColumns()...)
	baseline := bucket.ranges.Load()
	chunks := int64(readAheadTestRowGroups * readAheadTestColumns)
	require.Greater(t, baseline, chunks, "the baseline is expected to read each chunk in multiple requests")

	for _, tc := range []struct {
		name      string
		rowGroups []int
		opts      ReadAheadOptions
		ranges    int64
	}{
		{
			name:      "one row group",
			rowGroups: []int{0, 1, 2, 3},
			opts:      ReadAheadOptions{RowGroups: 1, MaxBytes: 1 << 20},
			ranges:    chunks,
		},
		{
			name:      "all row groups",
			rowGroups: []int{0, 1, 2, 3},
			opts:      ReadAheadOptions{RowGroups: 4, MaxBytes: 1 << 20},
			ranges:    chunks,
		},
		{
			name:      "limit allows a single chunk",
			rowGroups: []int{0, 1, 2, 3},
			opts:      ReadAheadOptions{RowGroups: 4, MaxBytes: maxChunkSize(f)},
			ranges:    chunks,
		},
		{
			name:      "chunks over limit",
			rowGroups: []int{0, 1, 2, 3},
			opts:      ReadAheadOptions{RowGroups: 4, MaxBytes: 1},
			ranges:    baseline,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bucket.ranges.Store(0)
			r := f.ReadAhead(context.Background(), tc.rowGroups, tc.opts).AddColumns(allColumns()...)
			assert.Equal(t, expected, readColumns(t, r.RowGroups(), allColumns()...))
			r.Close()
			assert.Equal(t, tc.ranges, bucket.ranges.Load())
			assert.Zero(t, r.pending)
		})
	}

	t.Run("row groups not planned are read as usual", func(t *testing.T) {
		r := f.ReadAhead(context.Background(), []int{1, 3}, ReadAheadOptions{RowGroups: 2, MaxBytes: 1 << 20}).AddColumns(allColumns()...)
		assert.Equal(t, expected, readColumns(t, r.RowGroups(), allColumns()...))
		r.Close()
		assert.Zero(t, r.pending)
	})

	t.Run("columns added later", func(t *testing.T) {
		bucket.ranges.Store(0)
		r := f.ReadAhead(context.Background(), []int{0, 1, 2, 3}, ReadAheadOptions{RowGroups: 2, MaxBytes: 1 << 20}).AddColumns(0)
		r.AddColumns(1, 2)
		assert.Equal(t, expected, readColumns(t, r.RowGroups(), allColumns()...))
		r.Close()
		assert.Equal(t, chunks, bucket.ranges.Load())
		assert.Zero(t, r.pending)
	})

	t.Run("skipped chunks are released", func(t *testing.T) {
		r := f.ReadAhead(context.Background(), []int{0, 1, 2, 3}, ReadAheadOptions{RowGroups: 4, MaxBytes: 1 << 20}).AddColumns(allColumns()...)
		values := readColumns(t, r.RowGroups()[2:], 1)
		var n int
		for _, rg := range f.RowGroups()[:2] {
			n += int(rg.ColumnChunks()[1].NumValues())
		}
		assert.Equal(t, expected[1][n:], values[0])
		r.mu.Lock()
		for _, rg := range r.plan[:2] {
			assert.Equal(t, chunkReleased, rg.ahead[1].state)
		}
		r.mu.Unlock()
		r.Close()
		assert.Zero(t, r.pending)
	})

	t.Run("not remote", func(t *testing.T) {
		f.remote = false
		defer func() { f.remote = true }()
		r := f.ReadAhead(context.Background(), []int{0, 1, 2, 3}, ReadAheadOptions{RowGroups: 4, MaxBytes: 1 << 20}).AddColumns(allColumns()...)
		assert.Equal(t, f.RowGroups(), r.RowGroups())
		r.Close()
	})
}

func TestReadAhead_Close(t *testing.T) {
	f, bucket := newReadAheadTestFile(t, 0)
	expected := readColumns(t, f.RowGroups(), 0)
	bucket.delay = time.Hour

	r := f.ReadAhead(context.Background(), []int{0, 1, 2, 3}, ReadAheadOptions{RowGroups: 4, MaxBytes: 1 << 20}).AddColumns(allColumns()...)
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Close()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not cancel the fetches")
	}
	assert.Zero(t, r.pending)

	// The chunks are read as usual after Close.
	bucket.delay = 0
	assert.Equal(t, expected, readColumns(t, r.RowGroups(), 0))
}

func TestReadAhead_FetchError(t *testing.T) {
	f, bucket := newReadAheadTestFile(t, 0)
	expected := readColumns(t, f.RowGroups(), 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := f.ReadAhead(ctx, []int{0, 1, 2, 3}, ReadAheadOptions{RowGroups: 4, MaxBytes: 1 << 20}).AddColumns(0)
	bucket.ranges.Store(0)
	assert.Equal(t, expected, readColumns(t, r.RowGroups(), 0))
	assert.Greater(t, bucket.ranges.Load(), int64(0))
	r.Close()
	assert.Zero(t, r.pending)
}

func maxChunkSize(f *ParquetFile) int64 {
	var size int64
	for _, rg := range f.Metadata().RowGroups {
		for _, c := range rg.Columns {
			size = max(size, c.MetaData.TotalCompressedSize)
		}
	}
	return size
}
