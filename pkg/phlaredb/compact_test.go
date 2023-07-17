package phlaredb

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	_ "net/http/pprof"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/segmentio/parquet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	phlareobj "github.com/grafana/phlare/pkg/objstore"
	"github.com/grafana/phlare/pkg/objstore/client"
	"github.com/grafana/phlare/pkg/objstore/providers/filesystem"
	"github.com/grafana/phlare/pkg/objstore/providers/gcs"
	"github.com/grafana/phlare/pkg/phlaredb/block"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
	"github.com/grafana/phlare/pkg/phlaredb/tsdb/index"
)

func init() {
	go func() {
		_ = http.ListenAndServe("localhost:6060", nil)
	}()
}

func TestCompact(t *testing.T) {
	t.TempDir()
	ctx := context.Background()
	bkt, err := client.NewBucket(ctx, client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.GCS,
			GCS: gcs.Config{
				BucketName: "dev-us-central-0-profiles-dev-001-data",
			},
		},
		StoragePrefix: "1218/phlaredb/",
	}, "test")
	require.NoError(t, err)
	now := time.Now()
	var (
		meta []*block.Meta
		mtx  sync.Mutex
	)

	err = block.IterBlockMetas(ctx, bkt, now.Add(-24*time.Hour), now, func(m *block.Meta) {
		mtx.Lock()
		defer mtx.Unlock()
		meta = append(meta, m)
	})
	require.NoError(t, err)
	dst := t.TempDir()

	sort.Slice(meta, func(i, j int) bool {
		return meta[i].MinTime.Before(meta[j].MinTime)
	})

	// only test on the 4 latest blocks.
	meta = meta[len(meta)-4:]
	testCompact(t, meta, bkt, dst)
}

// to download the blocks:
// gsutil -m cp -r \
// "gs://dev-us-central-0-profiles-dev-001-data/1218/phlaredb/01H53WJEAB43S3GJ26XMSNRSJA" \
// "gs://dev-us-central-0-profiles-dev-001-data/1218/phlaredb/01H5454JBEV80V2J7CKYHPCBG8" \
// "gs://dev-us-central-0-profiles-dev-001-data/1218/phlaredb/01H54553SYKH43FNJN5BVR1M2H" \
// "gs://dev-us-central-0-profiles-dev-001-data/1218/phlaredb/01H5457Q89WYYH9FCK8PZ6XG75" \
// .
func TestCompactLocal(t *testing.T) {
	t.TempDir()
	ctx := context.Background()
	bkt, err := client.NewBucket(ctx, client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.Filesystem,
			Filesystem: filesystem.Config{
				Directory: "/Users/cyril/work/phlare-data/",
			},
		},
		StoragePrefix: "",
	}, "test")
	require.NoError(t, err)
	var metas []*block.Meta

	metaMap, err := block.ListBlocks("/Users/cyril/work/phlare-data/", time.Time{})
	require.NoError(t, err)
	for _, m := range metaMap {
		metas = append(metas, m)
	}
	dst := t.TempDir()
	testCompact(t, metas, bkt, dst)
}

func testCompact(t *testing.T, metas []*block.Meta, bkt phlareobj.Bucket, dst string) {
	t.Helper()
	g, ctx := errgroup.WithContext(context.Background())
	var src []BlockReader
	now := time.Now()
	for i, m := range metas {
		t.Log("src block(#", i, ")",
			"ID", m.ULID.String(),
			"minTime", m.MinTime.Time().Format(time.RFC3339Nano),
			"maxTime", m.MaxTime.Time().Format(time.RFC3339Nano),
			"numSeries", m.Stats.NumSeries,
			"numProfiles", m.Stats.NumProfiles,
			"numSamples", m.Stats.NumSamples)
		b := NewSingleBlockQuerierFromMeta(ctx, bkt, m)
		g.Go(func() error {
			return b.Open(ctx)
		})

		src = append(src, b)
	}

	require.NoError(t, g.Wait())

	new, err := Compact(context.Background(), src, dst)
	require.NoError(t, err)
	t.Log(new, dst)
	t.Log("Compaction duration", time.Since(now))
	t.Log("numSeries", new.Stats.NumSeries,
		"numProfiles", new.Stats.NumProfiles,
		"numSamples", new.Stats.NumSamples)
}

func TestProfileRowIterator(t *testing.T) {
	filePath := t.TempDir() + "/index.tsdb"
	idxw, err := index.NewWriter(context.Background(), filePath)
	require.NoError(t, err)
	require.NoError(t, idxw.AddSymbol("a"))
	require.NoError(t, idxw.AddSymbol("b"))
	require.NoError(t, idxw.AddSymbol("c"))
	addSeries(t, idxw, 0, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "a", Value: "b"},
	})
	addSeries(t, idxw, 1, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "a", Value: "c"},
	})
	addSeries(t, idxw, 2, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "b", Value: "a"},
	})
	require.NoError(t, idxw.Close())
	idxr, err := index.NewFileReader(filePath)
	require.NoError(t, err)

	it, err := newProfileRowIterator(schemav1.NewInMemoryProfilesRowReader(
		[]schemav1.InMemoryProfile{
			{SeriesIndex: 0, TimeNanos: 1},
			{SeriesIndex: 1, TimeNanos: 2},
			{SeriesIndex: 2, TimeNanos: 3},
		},
	), idxr)
	require.NoError(t, err)

	assert.True(t, it.Next())
	require.Equal(t, it.At().labels, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "a", Value: "b"},
	})
	require.Equal(t, it.At().timeNanos, int64(1))
	require.Equal(t, it.At().seriesRef, uint32(0))

	assert.True(t, it.Next())
	require.Equal(t, it.At().labels, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "a", Value: "c"},
	})
	require.Equal(t, it.At().timeNanos, int64(2))
	require.Equal(t, it.At().seriesRef, uint32(1))

	assert.True(t, it.Next())
	require.Equal(t, it.At().labels, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "b", Value: "a"},
	})
	require.Equal(t, it.At().timeNanos, int64(3))
	require.Equal(t, it.At().seriesRef, uint32(2))

	assert.False(t, it.Next())
	require.NoError(t, it.Err())
	require.NoError(t, it.Close())
}

func addSeries(t *testing.T, idxw *index.Writer, idx int, labels phlaremodel.Labels) {
	t.Helper()
	require.NoError(t, idxw.AddSeries(storage.SeriesRef(idx), labels, model.Fingerprint(labels.Hash()), index.ChunkMeta{SeriesIndex: uint32(idx)}))
}

func TestMetaFilesFromDir(t *testing.T) {
	dst := t.TempDir()
	generateParquetFile(t, filepath.Join(dst, "foo.parquet"))
	generateParquetFile(t, filepath.Join(dst, "symbols", "bar.parquet"))
	generateFile(t, filepath.Join(dst, "symbols", "index.symdb"), 100)
	generateFile(t, filepath.Join(dst, "symbols", "stacktraces.symdb"), 200)
	generateIndexFile(t, dst)
	actual, err := metaFilesFromDir(dst)

	require.NoError(t, err)
	require.Equal(t, 5, len(actual))
	require.Equal(t, []block.File{
		{
			Parquet: &block.ParquetFile{
				NumRows:      100,
				NumRowGroups: 10,
			},
			RelPath:   "foo.parquet",
			SizeBytes: fileSize(t, filepath.Join(dst, "foo.parquet")),
		},
		{
			RelPath:   block.IndexFilename,
			SizeBytes: fileSize(t, filepath.Join(dst, block.IndexFilename)),
			TSDB: &block.TSDBFile{
				NumSeries: 3,
			},
		},
		{
			Parquet: &block.ParquetFile{
				NumRows:      100,
				NumRowGroups: 10,
			},
			RelPath:   filepath.Join("symbols", "bar.parquet"),
			SizeBytes: fileSize(t, filepath.Join(dst, "symbols", "bar.parquet")),
		},
		{
			RelPath:   filepath.Join("symbols", "index.symdb"),
			SizeBytes: fileSize(t, filepath.Join(dst, "symbols", "index.symdb")),
		},
		{
			RelPath:   filepath.Join("symbols", "stacktraces.symdb"),
			SizeBytes: fileSize(t, filepath.Join(dst, "symbols", "stacktraces.symdb")),
		},
	}, actual)
}

func fileSize(t *testing.T, path string) uint64 {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	return uint64(fi.Size())
}

func generateFile(t *testing.T, path string, size int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, f.Truncate(int64(size)))
}

func generateIndexFile(t *testing.T, dir string) {
	t.Helper()
	filePath := filepath.Join(dir, block.IndexFilename)
	idxw, err := index.NewWriter(context.Background(), filePath)
	require.NoError(t, err)
	require.NoError(t, idxw.AddSymbol("a"))
	require.NoError(t, idxw.AddSymbol("b"))
	require.NoError(t, idxw.AddSymbol("c"))
	addSeries(t, idxw, 0, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "a", Value: "b"},
	})
	addSeries(t, idxw, 1, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "a", Value: "c"},
	})
	addSeries(t, idxw, 2, phlaremodel.Labels{
		&typesv1.LabelPair{Name: "b", Value: "a"},
	})
	require.NoError(t, idxw.Close())
}

func generateParquetFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	require.NoError(t, err)
	defer file.Close()

	writer := parquet.NewGenericWriter[struct{ Name string }](file, parquet.MaxRowsPerRowGroup(10))
	defer writer.Close()
	for i := 0; i < 100; i++ {
		_, err := writer.Write([]struct{ Name string }{
			{Name: fmt.Sprintf("name-%d", i)},
		})
		require.NoError(t, err)
	}
}
