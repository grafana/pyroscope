package phlaredb

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	_ "net/http/pprof"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/phlare/pkg/model"
	"github.com/grafana/phlare/pkg/objstore/client"
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
		src []BlockReader
		mtx sync.Mutex
	)

	err = block.IterBlockMetas(ctx, bkt, now.Add(-24*time.Hour), now, func(m *block.Meta) {
		mtx.Lock()
		defer mtx.Unlock()
		// only test on the 3 latest blocks.
		if len(src) >= 3 {
			return
		}
		b := NewSingleBlockQuerierFromMeta(ctx, bkt, m)
		err := b.Open(ctx)
		require.NoError(t, err)
		src = append(src, b)
	})
	require.NoError(t, err)
	dst := t.TempDir()
	new, err := Compact(ctx, src, dst)
	require.NoError(t, err)
	t.Log(new, dst)
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
