package index

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

func TestBlockMetaTimeBounds_FieldNumbers(t *testing.T) {
	fields := (&metastorev1.BlockMeta{}).ProtoReflect().Descriptor().Fields()
	for _, tc := range []struct {
		name   string
		number protowire.Number
	}{
		{name: "min_time", number: blockMetaMinTimeField},
		{name: "max_time", number: blockMetaMaxTimeField},
	} {
		field := fields.ByName(protoreflect.Name(tc.name))
		require.NotNil(t, field)
		assert.Equal(t, tc.number, field.Number())
		assert.Equal(t, protoreflect.Int64Kind, field.Kind())
	}
}

func TestBlockMetaTimeBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta *metastorev1.BlockMeta
	}{
		{name: "empty", meta: &metastorev1.BlockMeta{}},
		{name: "zero times", meta: &metastorev1.BlockMeta{Id: "x", Tenant: 1, Shard: 2}},
		{name: "min time only", meta: &metastorev1.BlockMeta{MinTime: 1}},
		{name: "max time only", meta: &metastorev1.BlockMeta{MaxTime: 1}},
		{name: "negative times", meta: &metastorev1.BlockMeta{MinTime: -2, MaxTime: -1}},
		{
			name: "all fields",
			meta: &metastorev1.BlockMeta{
				FormatVersion:   1,
				Id:              "01J9E3ZXCA1QNSSZY8Z7A09VGX",
				Tenant:          3,
				Shard:           7,
				CompactionLevel: 2,
				MinTime:         1727080000000,
				MaxTime:         1727083600000,
				CreatedBy:       4,
				MetadataOffset:  1024,
				Size:            1 << 20,
				Datasets: []*metastorev1.Dataset{
					{Tenant: 1, Name: 2, MinTime: 1727080000000, MaxTime: 1727083600000, Labels: []int32{2, 3, 4, 5, 6}},
					{Tenant: 1, Name: 5, MinTime: 1727081000000, MaxTime: 1727082600000, Labels: []int32{2, 3, 4, 5, 6}},
				},
				StringTable: []string{"", "a", "b", "service_name", "svc", "__profile_type__", "cpu"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := tc.meta.MarshalVT()
			require.NoError(t, err)
			minTime, maxTime, err := blockMetaTimeBounds(b)
			require.NoError(t, err)
			assert.Equal(t, tc.meta.MinTime, minTime)
			assert.Equal(t, tc.meta.MaxTime, maxTime)
		})
	}
}

func TestBlockMetaTimeBounds_Randomized(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for range 1000 {
		meta := &metastorev1.BlockMeta{
			FormatVersion:   r.Uint32(),
			Id:              test.ULID(time.UnixMilli(r.Int63n(1e13)).Format(time.RFC3339)),
			Tenant:          r.Int31(),
			Shard:           r.Uint32(),
			CompactionLevel: r.Uint32() % 3,
			MinTime:         r.Int63() - r.Int63(),
			MaxTime:         r.Int63() - r.Int63(),
			CreatedBy:       r.Int31(),
			MetadataOffset:  r.Uint64(),
			Size:            r.Uint64(),
		}
		for d := 0; d < r.Intn(20); d++ {
			meta.Datasets = append(meta.Datasets, &metastorev1.Dataset{
				Tenant:          r.Int31(),
				Name:            r.Int31(),
				MinTime:         r.Int63(),
				MaxTime:         r.Int63(),
				TableOfContents: []uint64{r.Uint64(), r.Uint64()},
				Size:            r.Uint64(),
				Labels:          []int32{r.Int31n(64), r.Int31n(64), r.Int31n(64)},
				Format:          r.Uint32() % 2,
			})
		}
		for s := 0; s < r.Intn(10); s++ {
			meta.StringTable = append(meta.StringTable, test.ULID(time.UnixMilli(r.Int63n(1e13)).Format(time.RFC3339)))
		}
		b, err := meta.MarshalVT()
		require.NoError(t, err)

		var full metastorev1.BlockMeta
		require.NoError(t, full.UnmarshalVT(b))
		minTime, maxTime, err := blockMetaTimeBounds(b)
		require.NoError(t, err)
		require.Equal(t, full.MinTime, minTime)
		require.Equal(t, full.MaxTime, maxTime)
	}
}

func TestBlockMetaTimeBounds_MalformedInput(t *testing.T) {
	meta := &metastorev1.BlockMeta{MinTime: 100, MaxTime: 200}
	b, err := meta.MarshalVT()
	require.NoError(t, err)
	for i := 0; i <= len(b); i++ {
		var full metastorev1.BlockMeta
		fullErr := full.UnmarshalVT(b[:i])
		_, _, boundsErr := blockMetaTimeBounds(b[:i])
		if fullErr != nil {
			require.Error(t, boundsErr)
		} else {
			require.NoError(t, boundsErr)
		}
	}
	for _, malformed := range [][]byte{
		{0xff, 0xff, 0xff, 0xff},
		{0},    // Field number zero.
		{0x0c}, // Unexpected end group.
	} {
		_, _, err = blockMetaTimeBounds(malformed)
		require.Error(t, err)
	}
}

// Blocks outside the query time range must not be decoded into the block cache.
func TestQueryMetadata_TimeFilterSkipsDecode(t *testing.T) {
	db := test.BoltDB(t)

	// Two blocks in the same 6h partition, hours apart.
	inRange := &metastorev1.BlockMeta{
		Id:      test.ULID("2024-09-23T08:00:00.001Z"),
		Tenant:  1,
		MinTime: test.UnixMilli("2024-09-23T08:00:00.000Z"),
		MaxTime: test.UnixMilli("2024-09-23T08:15:00.000Z"),
		Datasets: []*metastorev1.Dataset{{
			Tenant:  1,
			Name:    2,
			MinTime: test.UnixMilli("2024-09-23T08:00:00.000Z"),
			MaxTime: test.UnixMilli("2024-09-23T08:15:00.000Z"),
			Labels:  []int32{1, 3, 4},
		}},
		StringTable: []string{"", "tenant-a", "dataset-a", "service_name", "svc-a"},
	}
	outOfRange := &metastorev1.BlockMeta{
		Id:      test.ULID("2024-09-23T10:30:00.002Z"),
		Tenant:  1,
		MinTime: test.UnixMilli("2024-09-23T10:30:00.000Z"),
		MaxTime: test.UnixMilli("2024-09-23T10:45:00.000Z"),
		Datasets: []*metastorev1.Dataset{{
			Tenant:  1,
			Name:    2,
			MinTime: test.UnixMilli("2024-09-23T10:30:00.000Z"),
			MaxTime: test.UnixMilli("2024-09-23T10:45:00.000Z"),
			Labels:  []int32{1, 3, 4},
		}},
		StringTable: []string{"", "tenant-a", "dataset-a", "service_name", "svc-a"},
	}

	idx := NewIndex(util.Logger, NewStore(), DefaultConfig, nil)
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		if err := idx.Init(tx); err != nil {
			return err
		}
		if err := idx.InsertBlock(tx, inRange.CloneVT()); err != nil {
			return err
		}
		return idx.InsertBlock(tx, outOfRange.CloneVT())
	}))

	// A fresh index observes only the persisted state, with cold caches.
	idx = NewIndex(util.Logger, NewStore(), DefaultConfig, nil)
	require.NoError(t, db.Update(idx.Init))

	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		found, err := idx.QueryMetadata(tx, t.Context(), MetadataQuery{
			Expr:      `{}`,
			StartTime: time.UnixMilli(test.UnixMilli("2024-09-23T08:00:00.000Z")),
			EndTime:   time.UnixMilli(test.UnixMilli("2024-09-23T09:00:00.000Z")),
			Tenant:    []string{"tenant-a"},
		})
		require.NoError(t, err)
		require.Len(t, found, 1)
		require.Equal(t, inRange.Id, found[0].Id)
		return nil
	}))

	cached := func(md *metastorev1.BlockMeta) bool {
		return idx.blocks.read.Contains(blockCacheKey{tenant: "tenant-a", shard: 0, block: md.Id})
	}
	assert.True(t, cached(inRange))
	assert.False(t, cached(outOfRange))

	// Labels queries use the same prefilter and must also avoid caching the
	// out-of-range block.
	idx = NewIndex(util.Logger, NewStore(), DefaultConfig, nil)
	require.NoError(t, db.Update(idx.Init))
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		labels, err := idx.QueryMetadataLabels(tx, t.Context(), MetadataQuery{
			Expr:      `{}`,
			StartTime: time.UnixMilli(test.UnixMilli("2024-09-23T08:00:00.000Z")),
			EndTime:   time.UnixMilli(test.UnixMilli("2024-09-23T09:00:00.000Z")),
			Tenant:    []string{"tenant-a"},
			Labels:    []string{"service_name"},
		})
		require.NoError(t, err)
		require.NotEmpty(t, labels)
		return nil
	}))
	assert.True(t, cached(inRange))
	assert.False(t, cached(outOfRange))
}
