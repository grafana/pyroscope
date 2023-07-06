package phlaredb

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/phlare/pkg/objstore/client"
	"github.com/grafana/phlare/pkg/objstore/providers/gcs"
	"github.com/grafana/phlare/pkg/phlaredb/block"
)

func TestCompact(t *testing.T) {
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

	err = block.IterBlockMetas(ctx, bkt, now.Add(-6*time.Hour), now, func(m *block.Meta) {
		mtx.Lock()
		defer mtx.Unlock()
		// todo: meta to blockreader
		// src = append(src, NewSingleBlockQuerierFromMeta(ctx, bkt, m))
	})
	require.NoError(t, err)
	new, err := Compact(ctx, src, "test/")
	require.NoError(t, err)
	t.Log(new)
}
