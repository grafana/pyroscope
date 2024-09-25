package index_test

import (
	"context"
	"crypto/rand"
	"math"
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
	store := mockindex.NewMockStore(t)
	i := index.NewIndex(store, util.Logger, &index.DefaultConfig)

	type partitionBlocks struct {
		key    index.PartitionKey
		blocks []*metastorev1.BlockMeta
	}

	partitions := []*partitionBlocks{
		{key: "20240923T06.1h", blocks: []*metastorev1.BlockMeta{
			{Id: createUlidString("2024-09-23T06:13:23.123Z")},
			{Id: createUlidString("2024-09-23T06:24:23.123Z")},
		}},
		{key: "20240923T07.1h", blocks: []*metastorev1.BlockMeta{
			{Id: createUlidString("2024-09-23T07:13:23.123Z")},
			{Id: createUlidString("2024-09-23T07:24:23.123Z")},
		}},
		{key: "20240923T08.1h", blocks: []*metastorev1.BlockMeta{
			{Id: createUlidString("2024-09-23T08:13:23.123Z")},
			{Id: createUlidString("2024-09-23T08:24:23.123Z")},
		}},
		{key: "20240923T09.1h", blocks: []*metastorev1.BlockMeta{
			{Id: createUlidString("2024-09-23T09:13:23.123Z")},
			{Id: createUlidString("2024-09-23T09:24:23.123Z")},
		}},
		{key: "20240923T10.1h", blocks: []*metastorev1.BlockMeta{
			{Id: createUlidString("2024-09-23T10:13:23.123Z")},
			{Id: createUlidString("2024-09-23T10:24:23.123Z")},
		}},
	}
	keys := make([]index.PartitionKey, 0, len(partitions))
	for _, partition := range partitions {
		keys = append(keys, partition.key)
	}
	store.On("ListPartitions").Return(keys)
	for _, p := range partitions {
		mockPartition(store, p.key, p.blocks)
	}
	i.LoadPartitions()

	found, err := i.FindBlocksInRange(createTime("2024-09-23T08:00:00.123Z"), createTime("2024-09-23T09:00:00.123Z"), map[string]struct{}{})
	require.NoError(t, err)
	require.Len(t, found, 8)
}

func mockPartition(store *mockindex.MockStore, key index.PartitionKey, blocks []*metastorev1.BlockMeta) {
	t, d, _ := key.Parse()
	store.On("ReadPartitionMeta", key).Return(&index.PartitionMeta{
		Key:      key,
		Ts:       t,
		Duration: d,
	}, nil).Maybe()
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
	i.ForEachPartition(context.Background(), func(meta *index.PartitionMeta) {
		mu.Lock()
		visited[meta.Key] = struct{}{}
		mu.Unlock()
	})

	require.Len(t, visited, 5)
}

func TestIndex_GetOrCreatePartitionMeta(t *testing.T) {
	store := mockindex.NewMockStore(t)
	i := index.NewIndex(store, util.Logger, &index.Config{PartitionDuration: time.Hour})

	block := &metastorev1.BlockMeta{
		Id:       createUlidString("2024-09-23T08:00:00.123Z"),
		TenantId: "tenant-1",
	}
	pMeta, err := i.GetOrCreatePartitionMeta(block)
	require.NoError(t, err)
	require.Equal(t, index.PartitionKey("20240923T08.1h"), pMeta.Key)
	require.Equal(t, time.UnixMilli(createTime("2024-09-23T08:00:00.000Z")).UTC(), pMeta.Ts)
	require.Equal(t, []string{"tenant-1"}, pMeta.Tenants)
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
			assert.Equalf(t, tt.want, i.GetPartitionKey(tt.blockId), "GetPartitionKey(%v)", tt.blockId)
		})
	}
}

func TestIndex_InsertBlock(t *testing.T) {
	store := mockindex.NewMockStore(t)
	i := index.NewIndex(store, util.Logger, &index.Config{PartitionDuration: time.Hour})
	block := &metastorev1.BlockMeta{
		Id:       createUlidString("2024-09-23T08:00:00.123Z"),
		TenantId: "tenant-1",
	}

	err := i.InsertBlock(block)
	require.NoError(t, err)
	require.NotNil(t, i.FindBlock(0, "tenant-1", block.Id))
	blocks, err := i.FindBlocksInRange(0, math.MaxInt64, map[string]struct{}{"tenant-1": {}})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Equal(t, block, blocks[0])

	// inserting the block again is a noop
	err = i.InsertBlock(block)
	require.NoError(t, err)
	blocks, err = i.FindBlocksInRange(0, math.MaxInt64, map[string]struct{}{"tenant-1": {}})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Equal(t, block, blocks[0])
}

func TestIndex_LoadPartitions(t *testing.T) {
	store := mockindex.NewMockStore(t)
	i := index.NewIndex(store, util.Logger, &index.Config{PartitionDuration: time.Hour})

	blocks := make([]*metastorev1.BlockMeta, 0, 420)
	for i := 0; i < 420; i++ {
		block := &metastorev1.BlockMeta{
			Id:    ulid.MustNew(ulid.Now(), rand.Reader).String(),
			Shard: 0,
		}
		blocks = append(blocks, block)
	}

	partitionKey := i.GetPartitionKey(blocks[0].Id)
	store.On("ListPartitions").Return([]index.PartitionKey{partitionKey})
	store.On("ReadPartitionMeta", mock.Anything).Return(&index.PartitionMeta{
		Key:      partitionKey,
		Ts:       time.Now().UTC(),
		Duration: time.Hour,
		Tenants:  []string{""},
	}, nil)
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
	i := index.NewIndex(store, util.Logger, &index.Config{PartitionDuration: time.Hour})
	b1 := &metastorev1.BlockMeta{
		Id: createUlidString("2024-09-23T08:00:00.123Z"),
	}
	_ = i.InsertBlock(b1)
	b2 := &metastorev1.BlockMeta{
		Id: createUlidString("2024-09-23T08:00:00.123Z"),
	}
	_ = i.InsertBlock(b2)

	replacement := &metastorev1.BlockMeta{
		Id:              createUlidString("2024-09-23T08:00:00.123Z"),
		CompactionLevel: 1,
		TenantId:        "tenant-1",
	}
	err := i.ReplaceBlocks([]string{b1.Id, b2.Id}, 0, "", []*metastorev1.BlockMeta{replacement})
	require.NoError(t, err)

	require.Nil(t, i.FindBlock(0, "", b1.Id))
	require.Nil(t, i.FindBlock(0, "", b2.Id))
	require.NotNil(t, i.FindBlock(0, "tenant-1", replacement.Id))
}

func TestIndex_StartCleanupLoop(t *testing.T) {
	store := mockindex.NewMockStore(t)
	i := index.NewIndex(store, util.Logger, &index.Config{
		PartitionDuration: time.Hour,
		CleanupInterval:   time.Second,
		PartitionTTL:      time.Second})
	b := &metastorev1.BlockMeta{
		Id: createUlidString("2024-09-23T08:00:00.123Z"),
	}
	_ = i.InsertBlock(b)
	require.NotNil(t, i.FindBlock(0, "", b.Id))
	store.AssertNotCalled(t, "ListBlocks", mock.Anything, mock.Anything, mock.Anything)

	mockPartition(store, "20240923T08.1h", []*metastorev1.BlockMeta{b})

	go i.StartCleanupLoop(context.Background())
	time.Sleep(2 * time.Second)

	require.NotNil(t, i.FindBlock(0, "", b.Id))
	store.AssertCalled(t, "ListBlocks", index.PartitionKey("20240923T08.1h"), uint32(0), "")
}

func TestIndex_DurationChange(t *testing.T) {
	store := mockindex.NewMockStore(t)
	i := index.NewIndex(store, util.Logger, &index.Config{PartitionDuration: 24 * time.Hour})
	b := &metastorev1.BlockMeta{
		Id: createUlidString("2024-09-23T08:00:00.123Z"),
	}
	_ = i.InsertBlock(b)
	require.NotNil(t, i.FindBlock(0, "", b.Id))

	i.Config.PartitionDuration = time.Hour
	require.NotNil(t, i.FindBlock(0, "", b.Id))
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
