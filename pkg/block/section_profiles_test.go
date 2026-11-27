package block

import (
	"bytes"
	"context"
	"io"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/parquet-go/parquet-go"
	"github.com/stretchr/testify/require"
	thanosobjstore "github.com/thanos-io/objstore"

	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/testutil"
)

func TestProfileRowIteratorSnapshotsLabels(t *testing.T) {
	var it profileRowIterator
	labels := phlaremodel.LabelsFromStrings("service_name", "original")
	first := it.snapshotLabels(labels)
	view := (ProfileEntry{labels: first}).View()
	w := NewDatasetIndexWriter()
	t.Cleanup(w.Close)
	w.addSeries(0, first, 1)

	// Reusing decoded labels and advancing series must not mutate retained
	// dataset-index labels or observer views.
	labels[0].Value = "next"
	second := it.snapshotLabels(labels)
	require.Equal(t, "original", first[0].value)
	require.Equal(t, "original", view.Labels.Get("service_name"))
	require.Equal(t, "original", w.series[0].labels[0].value)
	require.Equal(t, "next", second[0].value)
	require.NotSame(t, &first[0], &second[0])
	// A snapshot must not share an oversized accumulator backing array.
	require.Equal(t, len(first), cap(first))
	require.Equal(t, len(second), cap(second))
}

// BenchmarkProfileRowIteratorSnapshotLabels models merging overlapping series:
// every input snapshots its labels, but the output index retains only one
// snapshot per unique series. Keep the iterators alive through the merge, as
// the real merge tree does. In particular, do not benchmark repeated calls on
// one iterator across b.Loop iterations: that makes the old accumulator grow
// without bound rather than modeling independent compaction jobs.
func BenchmarkProfileRowIteratorSnapshotLabels(b *testing.B) {
	const seriesPerInput = 1000
	series := benchSeries(seriesPerInput)
	for _, inputs := range []int{1, 10} {
		b.Run("inputs="+strconv.Itoa(inputs), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				iterators := make([]profileRowIterator, inputs)
				retained := make([]immutableLabels, seriesPerInput)
				for s, labels := range series {
					for i := range iterators {
						snapshot := iterators[i].snapshotLabels(labels)
						iterators[i].currentRow.labels = snapshot
						if i == 0 {
							retained[s] = snapshot
						}
					}
				}
				runtime.KeepAlive(retained)
				runtime.KeepAlive(iterators)
			}
		})
	}
}

func createParquetFile[T any](t testing.TB, f io.Writer, rows []T, rowGroups int) {
	perRG := len(rows) / rowGroups

	w := parquet.NewGenericWriter[T](f)
	for i := 0; i < (rowGroups - 1); i++ {
		_, err := w.Write(rows[0:perRG])
		require.NoError(t, err)
		require.NoError(t, w.Flush())
		rows = rows[perRG:]
	}

	_, err := w.Write(rows)
	require.NoError(t, err)
	require.NoError(t, w.Flush())

	require.NoError(t, w.Close())
}

func createParquetTestFile(t testing.TB, f io.Writer, count int) {
	type T struct{ A int }

	rows := []T{}
	for i := 0; i < count; i++ {
		rows = append(rows, T{i})
	}

	createParquetFile(t, f, rows, 4)
}

func validateColumnIndex(t *testing.T, pf *parquet.File, count int) {
	rgs := pf.RowGroups()
	require.Equal(t, 4, len(rgs))

	// check last row groups column index
	ci, err := rgs[3].ColumnChunks()[0].ColumnIndex()
	require.NoError(t, err)

	pages := ci.NumPages()
	require.Equal(t, int64(count-1), ci.MaxValue(pages-1).Int64())
}

func Test_openParquetFile(t *testing.T) {
	path := "test.parquet"
	ctx := context.Background()
	bucket, _ := testutil.NewFilesystemBucket(t, ctx, t.TempDir())

	buf := bytes.NewBuffer(nil)
	count := 100
	createParquetTestFile(t, buf, count)

	actualFooterSize := footerSize(buf.Bytes())

	err := bucket.Upload(ctx, path, bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	pathOffset := "test.offset.parquet"
	require.NoError(t, bucket.Upload(ctx, pathOffset, bytes.NewReader(append(bytes.Repeat([]byte{0xab}, 16), buf.Bytes()...))))

	opts := []parquet.FileOption{
		parquet.SkipBloomFilters(true),
	}

	t.Run("withFooterSizeSmallerThanEstimate", func(t *testing.T) {
		pf, err := openParquetFile(bucket, path, 0, int64(buf.Len()), actualFooterSize*2, opts...)
		require.NoError(t, err)

		validateColumnIndex(t, pf.File, count)
	})

	t.Run("withFooterSizeExactEstimate", func(t *testing.T) {
		pf, err := openParquetFile(bucket, path, 0, int64(buf.Len()), actualFooterSize, opts...)
		require.NoError(t, err)

		validateColumnIndex(t, pf.File, count)
	})
	t.Run("withFooterSizeSmallerEstimate", func(t *testing.T) {
		pf, err := openParquetFile(bucket, path, 0, int64(buf.Len()), 200, opts...)
		require.NoError(t, err)

		validateColumnIndex(t, pf.File, count)
	})
	t.Run("withFooterSizeVerySmall", func(t *testing.T) {
		pf, err := openParquetFile(bucket, path, 0, int64(buf.Len()), 1, opts...)
		require.NoError(t, err)

		validateColumnIndex(t, pf.File, count)
	})

	t.Run("withOffsetAndFooterSmallerThanEstimate", func(t *testing.T) {
		pf, err := openParquetFile(bucket, pathOffset, 16, int64(buf.Len()), actualFooterSize*2, opts...)
		require.NoError(t, err)
		validateColumnIndex(t, pf.File, count)
	})

}

type countingBucket struct {
	thanosobjstore.Bucket
	getRanges atomic.Int64
}

func (c *countingBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	c.getRanges.Add(1)
	return c.Bucket.GetRange(ctx, name, off, length)
}

func TestParquetFile_PrefetchColumnsCoalescesReads(t *testing.T) {
	type multiColRow struct {
		A int64
		B int64
		C string
		D int64
	}
	ctx := context.Background()
	mem := thanosobjstore.NewInMemBucket()
	counting := &countingBucket{Bucket: mem}
	bucket := &objstore.ReaderAtBucket{Bucket: counting}

	var buf bytes.Buffer
	rows := make([]multiColRow, 400)
	for i := range rows {
		rows[i] = multiColRow{A: int64(i), B: int64(i * 10), C: "unused-payload", D: int64(i * 100)}
	}
	createParquetFile(t, &buf, rows, 4)

	const path = "multi.parquet"
	require.NoError(t, mem.Upload(ctx, path, bytes.NewReader(buf.Bytes())))

	counting.getRanges.Store(0)
	pf, err := openParquetFileWithCoalescer(
		bucket, path, 0, int64(buf.Len()), estimateFooterSize(int64(buf.Len())),
		parquet.SkipBloomFilters(true),
		parquet.FileReadMode(parquet.ReadModeSync),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pf.Close() })
	// Footer + ColumnIndex + OffsetIndex all fit in the single tail prefetch.
	require.Equal(t, int64(1), counting.getRanges.Load())

	// Prefetch columns 0, 1, and 3 across row groups 1 and 2; because the file is small,
	// all 6 column chunks coalesce into a single GetRange call.
	counting.getRanges.Store(0)
	require.NoError(t, pf.PrefetchColumns(ctx, []int{1, 2}, []int{0, 1, 3}))
	require.Equal(t, int64(1), counting.getRanges.Load())

	// Reading pages from those row groups and columns now requires zero additional GetRange calls.
	counting.getRanges.Store(0)
	valBuf := make([]parquet.Value, 128)
	for _, rgIdx := range []int{1, 2} {
		chunks := pf.RowGroups()[rgIdx].ColumnChunks()
		for _, colIdx := range []int{0, 1, 3} {
			pages := chunks[colIdx].Pages()
			for {
				pg, pErr := pages.ReadPage()
				if pErr == io.EOF {
					break
				}
				require.NoError(t, pErr)
				vr := pg.Values()
				for {
					n, vErr := vr.ReadValues(valBuf)
					if n > 0 && vErr == io.EOF {
						parquet.Release(pg)
						break
					}
					if vErr == io.EOF {
						parquet.Release(pg)
						break
					}
					require.NoError(t, vErr)
				}
			}
			require.NoError(t, pages.Close())
		}
	}
	require.Equal(t, int64(0), counting.getRanges.Load())
}
