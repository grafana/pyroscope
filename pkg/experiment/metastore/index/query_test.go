package index

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/util"
)

func TestIndex_Query(t *testing.T) {
	db := test.BoltDB(t)

	minT := test.UnixMilli("2024-09-23T08:00:00.000Z")
	maxT := test.UnixMilli("2024-09-23T09:00:00.000Z")

	md := &metastorev1.BlockMeta{
		Id:        test.ULID("2024-09-23T08:00:00.001Z"),
		Tenant:    0,
		MinTime:   minT,
		MaxTime:   maxT,
		CreatedBy: 1,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 2, Name: 3, MinTime: minT, MaxTime: minT, Labels: []int32{2, 4, 3, 5, 6}},
			{Tenant: 7, Name: 8, MinTime: maxT, MaxTime: maxT, Labels: []int32{2, 4, 8, 5, 9}},
		},
		StringTable: []string{
			"", "ingester",
			"tenant-a", "dataset-a", "service_name", "__profile_type__", "1",
			"tenant-b", "dataset-b", "4",
		},
	}

	md2 := &metastorev1.BlockMeta{
		Id:        test.ULID("2024-09-23T08:00:00.002Z"),
		Tenant:    1,
		Shard:     1,
		MinTime:   minT,
		MaxTime:   maxT,
		CreatedBy: 2,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 1, Name: 3, MinTime: minT, MaxTime: minT, Labels: []int32{2, 4, 3, 5, 6}},
		},
		StringTable: []string{
			"", "tenant-a", "ingester", "dataset-a", "service_name", "__profile_type__", "1",
		},
	}

	md3 := &metastorev1.BlockMeta{
		Id:        test.ULID("2024-09-23T08:30:00.003Z"),
		Tenant:    1,
		Shard:     1,
		MinTime:   minT,
		MaxTime:   maxT,
		CreatedBy: 2,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 1, Name: 3, MinTime: minT, MaxTime: minT, Labels: []int32{2, 4, 3, 5, 6}},
		},
		StringTable: []string{
			"", "tenant-a", "ingester", "dataset-a", "service_name", "__profile_type__", "1",
		},
	}

	query := func(t *testing.T, tx *bbolt.Tx, index *Index) {
		t.Run("FindBlocks", func(t *testing.T) {
			found, err := index.FindBlocks(tx, &metastorev1.BlockList{Blocks: []string{md.Id}})
			require.NoError(t, err)
			require.NotEmpty(t, found)
			require.Equal(t, md, found[0])

			found, err = index.FindBlocks(tx, &metastorev1.BlockList{
				Tenant: "tenant-a",
				Shard:  1,
				Blocks: []string{md2.Id, md3.Id},
			})
			require.NoError(t, err)
			require.NotEmpty(t, found)
			require.Equal(t, md2, found[0])
			require.Equal(t, md3, found[1])

			found, err = index.FindBlocks(tx, &metastorev1.BlockList{
				Tenant: "tenant-b",
				Shard:  1,
				Blocks: []string{md.Id},
			})
			require.NoError(t, err)
			require.Empty(t, found)

			found, err = index.FindBlocks(tx, &metastorev1.BlockList{
				Shard:  1,
				Blocks: []string{md.Id},
			})
			require.NoError(t, err)
			require.Empty(t, found)
		})

		t.Run("DatasetFilter", func(t *testing.T) {
			expected := []*metastorev1.BlockMeta{
				{
					Id:          md.Id,
					Tenant:      0,
					MinTime:     minT,
					MaxTime:     maxT,
					CreatedBy:   1,
					Datasets:    []*metastorev1.Dataset{{Tenant: 2, Name: 3, MinTime: minT, MaxTime: minT}},
					StringTable: []string{"", "ingester", "tenant-a", "dataset-a"},
				},
				{
					Id:          md2.Id,
					Tenant:      1,
					Shard:       1,
					MinTime:     minT,
					MaxTime:     maxT,
					CreatedBy:   2,
					Datasets:    []*metastorev1.Dataset{{Tenant: 1, Name: 3, MinTime: minT, MaxTime: minT}},
					StringTable: []string{"", "tenant-a", "ingester", "dataset-a"},
				},
				{
					Id:          md3.Id,
					Tenant:      1,
					Shard:       1,
					MinTime:     minT,
					MaxTime:     maxT,
					CreatedBy:   2,
					Datasets:    []*metastorev1.Dataset{{Tenant: 1, Name: 3, MinTime: minT, MaxTime: minT}},
					StringTable: []string{"", "tenant-a", "ingester", "dataset-a"},
				},
			}

			found, err := iter.Slice(index.QueryMetadata(tx, MetadataQuery{
				Expr:      `{service_name=~"dataset-a"}`,
				StartTime: time.UnixMilli(minT),
				EndTime:   time.UnixMilli(maxT),
				Tenant:    []string{"tenant-a", "tenant-b"},
			}))
			require.NoError(t, err)
			require.Equal(t, expected, found)
		})

		t.Run("DatasetFilter_KeepLabels", func(t *testing.T) {
			expected := []*metastorev1.BlockMeta{
				{
					Id:        md.Id,
					Tenant:    0,
					MinTime:   minT,
					MaxTime:   maxT,
					CreatedBy: 1,
					Datasets: []*metastorev1.Dataset{{
						Tenant:  2,
						Name:    3,
						MinTime: minT,
						MaxTime: minT,
						Labels:  []int32{1, 4, 3},
					}},
					StringTable: []string{"", "ingester", "tenant-a", "dataset-a", "service_name"},
				},
				{
					Id:        md2.Id,
					Tenant:    1,
					Shard:     1,
					MinTime:   minT,
					MaxTime:   maxT,
					CreatedBy: 2,
					Datasets: []*metastorev1.Dataset{{
						Tenant:  1,
						Name:    3,
						MinTime: minT,
						MaxTime: minT,
						Labels:  []int32{1, 4, 3},
					}},
					StringTable: []string{"", "tenant-a", "ingester", "dataset-a", "service_name"},
				},
				{
					Id:        md3.Id,
					Tenant:    1,
					Shard:     1,
					MinTime:   minT,
					MaxTime:   maxT,
					CreatedBy: 2,
					Datasets: []*metastorev1.Dataset{{
						Tenant:  1,
						Name:    3,
						MinTime: minT,
						MaxTime: minT,
						Labels:  []int32{1, 4, 3},
					}},
					StringTable: []string{"", "tenant-a", "ingester", "dataset-a", "service_name"},
				},
			}

			found, err := iter.Slice(index.QueryMetadata(tx, MetadataQuery{
				Expr:      `{service_name=~"dataset-a"}`,
				StartTime: time.UnixMilli(minT),
				EndTime:   time.UnixMilli(maxT),
				Tenant:    []string{"tenant-a", "tenant-b"},
				Labels:    []string{"service_name"},
			}))
			require.NoError(t, err)
			require.Equal(t, expected, found)
		})

		t.Run("TimeRangeFilter", func(t *testing.T) {
			found, err := iter.Slice(index.QueryMetadata(tx, MetadataQuery{
				Expr:      `{service_name=~"dataset-b"}`,
				StartTime: time.UnixMilli(minT),
				EndTime:   time.UnixMilli(maxT),
				Tenant:    []string{"tenant-b"},
			}))
			require.NoError(t, err)
			require.Empty(t, found)
		})

		t.Run("Labels", func(t *testing.T) {
			labels, err := index.QueryMetadataLabels(tx, MetadataQuery{
				Expr:      `{service_name=~"dataset.*"}`,
				StartTime: time.UnixMilli(minT),
				EndTime:   time.UnixMilli(maxT),
				Tenant:    []string{"tenant-a"},
				Labels: []string{
					model.LabelNameProfileType,
					model.LabelNameServiceName,
				},
			})
			require.NoError(t, err)
			require.NotEmpty(t, labels)
			assert.Equal(t, []*typesv1.Labels{{Labels: []*typesv1.LabelPair{
				{Name: model.LabelNameProfileType, Value: "1"},
				{Name: model.LabelNameServiceName, Value: "dataset-a"},
			}}}, labels)
		})
	}

	idx := NewIndex(util.Logger, NewStore(), &DefaultConfig)
	tx, err := db.Begin(true)
	require.NoError(t, err)
	require.NoError(t, idx.Init(tx))
	require.NoError(t, idx.InsertBlock(tx, md.CloneVT()))
	require.NoError(t, idx.InsertBlock(tx, md2.CloneVT()))
	require.NoError(t, idx.InsertBlock(tx, md3.CloneVT()))
	require.NoError(t, tx.Commit())

	t.Run("BeforeRestore", func(t *testing.T) {
		tx, err := db.Begin(false)
		require.NoError(t, err)
		query(t, tx, idx)
		require.NoError(t, tx.Rollback())
	})

	t.Run("Restored", func(t *testing.T) {
		idx = NewIndex(util.Logger, NewStore(), &DefaultConfig)
		tx, err = db.Begin(false)
		defer func() {
			require.NoError(t, tx.Rollback())
		}()
		require.NoError(t, err)
		require.NoError(t, idx.Restore(tx))
		query(t, tx, idx)
	})
}
