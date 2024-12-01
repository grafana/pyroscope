package index

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index/store"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/util"
)

func TestIndex_Insert(t *testing.T) {
	db := test.BoltDB(t)

	minT := test.Time("2024-09-23T08:00:00.000Z")
	maxT := test.Time("2024-09-23T09:00:00.000Z")

	id := test.ULID("2024-09-23T08:00:00.000Z")
	md := &metastorev1.BlockMeta{
		Id:        1,
		Tenant:    0,
		MinTime:   minT,
		MaxTime:   maxT,
		CreatedBy: 2,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 3, Name: 4, ProfileTypes: []int32{5, 6, 7}, MinTime: minT, MaxTime: minT},
			{Tenant: 8, Name: 9, ProfileTypes: []int32{6, 7, 10}, MinTime: maxT, MaxTime: maxT},
		},
		StringTable: []string{
			"", id, "ingester",
			"tenant-a", "dataset-a", "1", "2", "3",
			"tenant-b", "dataset-b", "4",
		},
	}

	s := store.NewIndexStore()
	idx := NewIndex(util.Logger, s, &DefaultConfig)

	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, idx.Init(tx))
	require.NoError(t, idx.InsertBlock(tx, md.CloneVT()))
	require.NoError(t, tx.Commit())

	idx = NewIndex(util.Logger, s, &DefaultConfig)
	tx, err = db.Begin(false)
	defer func() {
		require.NoError(t, tx.Rollback())
	}()
	require.NoError(t, err)
	require.NoError(t, idx.Restore(tx))

	found := idx.FindBlocks(tx, &metastorev1.BlockList{
		Tenant: "",
		Shard:  0,
		Blocks: []string{id},
	})
	require.NotEmpty(t, found)
	require.Equal(t, md, found[0])

	found, err = iter.Slice(idx.QueryMetadata(tx, MetadataQuery{
		Expr:      `{service_name="dataset-a"}`,
		StartTime: time.UnixMilli(minT),
		EndTime:   time.UnixMilli(maxT),
		Tenant:    []string{"tenant-a"},
	}))
	require.NoError(t, err)
	require.NotEmpty(t, found)

	require.Equal(t, md, found[0])
}
