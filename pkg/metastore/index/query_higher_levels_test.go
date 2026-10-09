package index

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

func TestIndex_QueryHigherLevelsOutsidePartitionWindow(t *testing.T) {
	db := test.BoltDB(t)
	idx := NewIndex(util.Logger, NewStore(), DefaultConfig, nil)
	require.NoError(t, db.Update(idx.Init))
	// Compacted blocks inherit an early source ID, but later L3 sources can contain data
	// beyond 24h from that ID. A narrow query at the tail must still find it.
	start := test.Time("2024-09-01T00:00:00Z")
	end := start.Add(5 * 24 * time.Hour)
	md := &metastorev1.BlockMeta{
		Id: test.ULID(start.Format(time.RFC3339)), Tenant: 1, Shard: 1,
		CompactionLevel: 8, MinTime: start.UnixMilli(), MaxTime: end.UnixMilli(),
		StringTable: []string{"", "tenant-a", "service_name", "svc"},
		Datasets: []*metastorev1.Dataset{{
			Tenant: 1, MinTime: start.UnixMilli(), MaxTime: end.UnixMilli(), Labels: []int32{1, 2, 3},
		}},
	}
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error { return idx.InsertBlock(tx, md.CloneVT()) }))
	for _, cold := range []bool{false, true} {
		if cold {
			idx = NewIndex(util.Logger, NewStore(), DefaultConfig, nil)
			require.NoError(t, db.Update(idx.Init))
		}
		for _, tenant := range []string{"tenant-a", "tenant-b"} {
			q := MetadataQuery{Expr: `{service_name="svc"}`, StartTime: end.Add(-time.Minute), EndTime: end, Tenant: []string{tenant}, Labels: []string{"service_name"}}
			require.NoError(t, db.View(func(tx *bbolt.Tx) error {
				blocks, err := idx.QueryMetadata(tx, t.Context(), q)
				require.NoError(t, err)
				labels, err := idx.QueryMetadataLabels(tx, t.Context(), q)
				require.NoError(t, err)
				if tenant == "tenant-b" {
					require.Empty(t, blocks)
					require.Empty(t, labels)
					return nil
				}
				require.Len(t, blocks, 1)
				require.Equal(t, md.Id, blocks[0].Id)
				require.Equal(t, uint32(8), blocks[0].CompactionLevel)
				require.Len(t, labels, 1)
				require.Equal(t, "svc", labels[0].Labels[0].Value)
				return nil
			}))
		}
	}
}
