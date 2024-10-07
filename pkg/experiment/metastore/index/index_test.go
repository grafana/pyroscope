package index_test

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockindex"
	"github.com/grafana/pyroscope/pkg/util"
)

func TestIndex_FindBlocksInRange(t *testing.T) {
	tests := []struct {
		name       string
		blocks     []*metastorev1.BlockMeta
		queryStart int64
		queryEnd   int64
		want       int
	}{
		{
			name: "matching blocks",
			blocks: []*metastorev1.BlockMeta{
				createBlock("20240923T06.1h", 0),
				createBlock("20240923T07.1h", 0),
				createBlock("20240923T08.1h", 0),
				createBlock("20240923T09.1h", 0),
				createBlock("20240923T10.1h", 0),
			},
			queryStart: createTime("2024-09-23T08:00:00.000Z"),
			queryEnd:   createTime("2024-09-23T09:00:00.000Z"),
			want:       2,
		},
		{
			name: "no matching blocks",
			blocks: []*metastorev1.BlockMeta{
				createBlock("20240923T06.1h", 0),
				createBlock("20240923T07.1h", 0),
				createBlock("20240923T08.1h", 0),
				createBlock("20240923T09.1h", 0),
				createBlock("20240923T10.1h", 0),
			},
			queryStart: createTime("2024-09-23T04:00:00.000Z"),
			queryEnd:   createTime("2024-09-23T05:00:00.000Z"),
			want:       0,
		},
		{
			name: "out of order ingestion (behind on time)",
			blocks: []*metastorev1.BlockMeta{
				createBlock("20240923T06.1h", 0),
				createBlock("20240923T07.1h", -1*time.Hour), // in range
				createBlock("20240923T07.1h", -2*time.Hour), // in range
				createBlock("20240923T07.1h", -3*time.Hour), // too old
				createBlock("20240923T08.1h", -3*time.Hour), // // technically in range but we will not look here
				createBlock("20240923T10.1h", 0),
			},
			queryStart: createTime("2024-09-23T05:00:00.000Z"),
			queryEnd:   createTime("2024-09-23T06:00:00.000Z"),
			want:       3,
		},
		{
			name: "out of order ingestion (ahead of time)",
			blocks: []*metastorev1.BlockMeta{
				createBlock("20240923T06.1h", 2*time.Hour), // technically in range but we will not look here
				createBlock("20240923T07.1h", 1*time.Hour), // in range
				createBlock("20240923T07.1h", 3*time.Hour), // too new
				createBlock("20240923T08.1h", 0),           // in range
				createBlock("20240923T08.1h", 1*time.Hour), // in range
				createBlock("20240923T10.1h", 0),
			},
			queryStart: createTime("2024-09-23T08:00:00.000Z"),
			queryEnd:   createTime("2024-09-23T09:00:00.000Z"),
			want:       3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := mockindex.NewMockStore(t)
			store.On("ListShards", mock.Anything).Return([]uint32{})
			i := index.NewIndex(store, util.Logger, &index.Config{
				PartitionDuration:     time.Hour,
				PartitionCacheSize:    24,
				QueryLookaroundPeriod: time.Hour,
			})
			for _, b := range tt.blocks {
				i.InsertBlock(b)
			}
			tenantMap := map[string]struct{}{"tenant-1": {}}
			found, err := i.FindBlocksInRange(tt.queryStart, tt.queryEnd, tenantMap)
			require.NoError(t, err)
			require.Equal(t, tt.want, len(found))
			for _, b := range found {
				require.Truef(
					t,
					tt.queryStart < b.MaxTime && tt.queryEnd >= b.MinTime,
					"block %s is not in range, %v : %v", b.Id, time.UnixMilli(b.MinTime).UTC(), time.UnixMilli(b.MaxTime).UTC())
			}
		})
	}

}

func mockPartition(store *mockindex.MockStore, key index.PartitionKey, blocks []*metastorev1.BlockMeta) {
	store.On("ListShards", key).Return([]uint32{0}).Maybe()
	store.On("ListTenants", key, uint32(0)).Return([]string{""}).Maybe()
	store.On("ListBlocks", key, uint32(0), "").Return(blocks).Maybe()
}

func TestIndex_ForEachPartition(t *testing.T) {
	store := mockindex.NewMockStore(t)
	i := index.NewIndex(store, util.Logger, &index.Config{PartitionDuration: time.Hour})

	keys := []index.PartitionKey{
		"20240923T06.1h",
		"20240923T07.1h",
		"20240923T08.1h",
		"20240923T09.1h",
		"20240923T10.1h",
	}
	store.On("ListPartitions").Return(keys)
	for _, key := range keys {
		mockPartition(store, key, nil)
	}
	i.LoadPartitions()

	visited := make(map[index.PartitionKey]struct{})
	var mu sync.Mutex
	err := i.ForEachPartition(context.Background(), func(meta *index.PartitionMeta) error {
		mu.Lock()
		visited[meta.Key] = struct{}{}
		mu.Unlock()
		return nil
	})
	require.NoError(t, err)

	require.Len(t, visited, 5)
}

func TestIndex_GetPartitionKey(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		blockId  string
		want     index.PartitionKey
	}{
		{
			name:     "1d",
			duration: createDuration("1d"),
			blockId:  createUlidString("2024-07-15T16:13:43.245Z"),
			want:     index.PartitionKey("20240715.1d"),
		},
		{
			name:     "1h at start of the window",
			duration: createDuration("1h"),
			blockId:  createUlidString("2024-07-15T16:00:00.000Z"),
			want:     index.PartitionKey("20240715T16.1h"),
		},
		{
			name:     "1h in the middle of the window",
			duration: createDuration("1h"),
			blockId:  createUlidString("2024-07-15T16:13:43.245Z"),
			want:     index.PartitionKey("20240715T16.1h"),
		},
		{
			name:     "1h at the end of the window",
			duration: createDuration("1h"),
			blockId:  createUlidString("2024-07-15T16:59:59.999Z"),
			want:     index.PartitionKey("20240715T16.1h"),
		},
		{
			name:     "6h duration at midnight",
			duration: createDuration("6h"),
			blockId:  createUlidString("2024-07-15T00:00:00.000Z"),
			want:     index.PartitionKey("20240715T00.6h"),
		},
		{
			name:     "6h at the middle of a window",
			duration: createDuration("6h"),
			blockId:  createUlidString("2024-07-15T15:13:43.245Z"),
			want:     index.PartitionKey("20240715T12.6h"),
		},
		{
			name:     "6h at the end of the window",
			duration: createDuration("6h"),
			blockId:  createUlidString("2024-07-15T23:59:59.999Z"),
			want:     index.PartitionKey("20240715T18.6h"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := index.NewIndex(mockindex.NewMockStore(t), util.Logger, &index.Config{PartitionDuration: tt.duration})
			assert.Equalf(t, tt.want, i.CreatePartitionKey(tt.blockId), "CreatePartitionKey(%v)", tt.blockId)
		})
	}
}

func TestIndex_InsertBlock(t *testing.T) {
	store := mockindex.NewMockStore(t)
	store.On("ListShards", mock.Anything).Return([]uint32{})
	i := index.NewIndex(store, util.Logger, &index.Config{PartitionDuration: time.Hour, PartitionCacheSize: 1})
	block := &metastorev1.BlockMeta{
		Id:       createUlidString("2024-09-23T08:00:00.123Z"),
		TenantId: "tenant-1",
		MinTime:  createTime("2024-09-23T08:00:00.000Z"),
		MaxTime:  createTime("2024-09-23T08:05:00.000Z"),
	}

	i.InsertBlock(block)
	require.NotNil(t, i.FindBlock(0, "tenant-1", block.Id))
	blocks, err := i.FindBlocksInRange(createTime("2024-09-23T07:00:00.000Z"), createTime("2024-09-23T09:00:00.000Z"), map[string]struct{}{"tenant-1": {}})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Equal(t, block, blocks[0])

	// inserting the block again is a noop
	i.InsertBlock(block)
	blocks, err = i.FindBlocksInRange(createTime("2024-09-23T07:00:00.000Z"), createTime("2024-09-23T09:00:00.000Z"), map[string]struct{}{"tenant-1": {}})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Equal(t, block, blocks[0])
}

func TestIndex_LoadPartitions(t *testing.T) {
	store := mockindex.NewMockStore(t)
	i := index.NewIndex(store, util.Logger, &index.Config{PartitionDuration: time.Hour, PartitionCacheSize: 1})

	blocks := make([]*metastorev1.BlockMeta, 0, 420)
	for i := 0; i < 420; i++ {
		block := &metastorev1.BlockMeta{
			Id:    ulid.MustNew(ulid.Now(), rand.Reader).String(),
			Shard: 0,
		}
		blocks = append(blocks, block)
	}

	partitionKey := i.CreatePartitionKey(blocks[0].Id)
	store.On("ListPartitions").Return([]index.PartitionKey{partitionKey})
	store.On("ListShards", mock.Anything).Return([]uint32{0})
	store.On("ListTenants", mock.Anything, mock.Anything).Return([]string{""})
	store.On("ListBlocks", mock.Anything, mock.Anything, mock.Anything).Return(blocks)

	// restore from store
	i.LoadPartitions()

	for _, b := range blocks {
		require.NotNilf(t, i.FindBlock(b.Shard, b.TenantId, b.Id), "block %s not found", b.Id)
	}
}

func TestIndex_ReplaceBlocks(t *testing.T) {
	store := mockindex.NewMockStore(t)
	store.On("ListShards", mock.Anything).Return([]uint32{})
	i := index.NewIndex(store, util.Logger, &index.DefaultConfig)
	b1 := &metastorev1.BlockMeta{
		Id: createUlidString("2024-09-23T08:00:00.123Z"),
	}
	i.InsertBlock(b1)
	b2 := &metastorev1.BlockMeta{
		Id: createUlidString("2024-09-23T08:00:00.123Z"),
	}
	i.InsertBlock(b2)

	replacement := &metastorev1.BlockMeta{
		Id:              createUlidString("2024-09-23T08:00:00.123Z"),
		CompactionLevel: 1,
		TenantId:        "tenant-1",
	}
	i.ReplaceBlocks([]string{b1.Id, b2.Id}, 0, "", []*metastorev1.BlockMeta{replacement})

	require.Nil(t, i.FindBlock(0, "", b1.Id))
	require.Nil(t, i.FindBlock(0, "", b2.Id))
	require.NotNil(t, i.FindBlock(0, "tenant-1", replacement.Id))
}

func TestIndex_DurationChange(t *testing.T) {
	store := mockindex.NewMockStore(t)
	store.On("ListShards", mock.Anything).Return([]uint32{})
	i := index.NewIndex(store, util.Logger, &index.Config{PartitionDuration: 24 * time.Hour, PartitionCacheSize: 1})
	b := &metastorev1.BlockMeta{
		Id: createUlidString("2024-09-23T08:00:00.123Z"),
	}
	i.InsertBlock(b)
	require.NotNil(t, i.FindBlock(0, "", b.Id))

	i.Config.PartitionDuration = time.Hour
	require.NotNil(t, i.FindBlock(0, "", b.Id))
}

func TestIndex_UnloadPartitions(t *testing.T) {
	store := mockindex.NewMockStore(t)
	i := index.NewIndex(store, util.Logger, &index.Config{PartitionDuration: time.Hour, PartitionCacheSize: 3})

	keys := []index.PartitionKey{
		"20240923T06.1h",
		"20240923T07.1h",
		"20240923T08.1h",
		"20240923T09.1h",
		"20240923T10.1h",
	}
	store.On("ListPartitions").Return(keys)
	for _, key := range keys {
		mockPartition(store, key, nil)
	}
	i.LoadPartitions()
	require.True(t, store.AssertNumberOfCalls(t, "ListShards", 5))

	for _, key := range keys {
		start, _, _ := key.Parse()
		for c := 0; c < 10; c++ {
			_, err := i.FindBlocksInRange(start.UnixMilli(), start.Add(5*time.Minute).UnixMilli(), map[string]struct{}{"": {}})
			require.NoError(t, err)
		}
	}
	// multiple reads cause a single store access
	require.True(t, store.AssertNumberOfCalls(t, "ListShards", 10))

	for c := 0; c < 10; c++ {
		_, err := i.FindBlocksInRange(createTime("2024-09-23T08:00:00.000Z"), createTime("2024-09-23T08:05:00.000Z"), map[string]struct{}{"": {}})
		require.NoError(t, err)
	}
	// this partition is still loaded in memory
	require.True(t, store.AssertNumberOfCalls(t, "ListShards", 10))

	for c := 0; c < 10; c++ {
		_, err := i.FindBlocksInRange(createTime("2024-09-23T06:00:00.000Z"), createTime("2024-09-23T06:05:00.000Z"), map[string]struct{}{"": {}})
		require.NoError(t, err)
	}
	// this partition was unloaded
	require.True(t, store.AssertNumberOfCalls(t, "ListShards", 11))
}

func createUlidString(t string) string {
	parsed, _ := time.Parse(time.RFC3339, t)
	l := ulid.MustNew(ulid.Timestamp(parsed), rand.Reader)
	return l.String()
}

func createDuration(d string) time.Duration {
	parsed, _ := model.ParseDuration(d)
	return time.Duration(parsed)
}

func createTime(t string) int64 {
	ts, _ := time.Parse(time.RFC3339, t)
	return ts.UnixMilli()
}

func createBlock(key string, offset time.Duration) *metastorev1.BlockMeta {
	pKey := index.PartitionKey(key)
	ts, _, _ := pKey.Parse()
	return &metastorev1.BlockMeta{
		Id:       createUlidString(ts.Format(time.RFC3339)),
		MinTime:  ts.Add(offset).UnixMilli(),
		MaxTime:  ts.Add(offset).Add(5 * time.Minute).UnixMilli(),
		TenantId: "tenant-1",
	}
}
