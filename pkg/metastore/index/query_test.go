package index

import (
	"context"
	"crypto/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"go.uber.org/atomic"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/util"
)

func TestIndex_Query(t *testing.T) {
	db := test.BoltDB(t)
	ctx := context.Background()

	minT := test.UnixMilli("2024-09-23T08:00:00.000Z")
	maxT := test.UnixMilli("2024-09-23T09:00:00.000Z")

	md := &metastorev1.BlockMeta{
		Id:        test.ULID("2024-09-23T08:00:00.001Z"),
		Tenant:    0,
		MinTime:   minT,
		MaxTime:   maxT,
		CreatedBy: 1,
		Datasets: []*metastorev1.Dataset{
			{Tenant: 2, Name: 3, MinTime: minT, MaxTime: maxT, Labels: []int32{2, 4, 3, 5, 6}},
			{Tenant: 7, Name: 8, MinTime: minT, MaxTime: maxT, Labels: []int32{2, 4, 8, 5, 9}},
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
			{Tenant: 1, Name: 3, MinTime: minT, MaxTime: maxT, Labels: []int32{2, 4, 3, 5, 6}},
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
			{Tenant: 1, Name: 3, MinTime: minT, MaxTime: maxT, Labels: []int32{2, 4, 3, 5, 6}},
		},
		StringTable: []string{
			"", "tenant-a", "ingester", "dataset-a", "service_name", "__profile_type__", "1",
		},
	}

	query := func(t *testing.T, tx *bbolt.Tx, index *Index) {
		t.Run("GetBlocks", func(t *testing.T) {
			found, err := index.GetBlocks(tx, &metastorev1.BlockList{Blocks: []string{md.Id}})
			require.NoError(t, err)
			require.NotEmpty(t, found)
			require.Equal(t, md, found[0])

			found, err = index.GetBlocks(tx, &metastorev1.BlockList{
				Tenant: "tenant-a",
				Shard:  1,
				Blocks: []string{md2.Id, md3.Id},
			})
			require.NoError(t, err)
			require.NotEmpty(t, found)
			require.Equal(t, md2, found[0])
			require.Equal(t, md3, found[1])

			found, err = index.GetBlocks(tx, &metastorev1.BlockList{
				Tenant: "tenant-b",
				Shard:  1,
				Blocks: []string{md.Id},
			})
			require.NoError(t, err)
			require.Empty(t, found)

			found, err = index.GetBlocks(tx, &metastorev1.BlockList{
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
					Datasets:    []*metastorev1.Dataset{{Tenant: 2, Name: 3, MinTime: minT, MaxTime: maxT}},
					StringTable: []string{"", "ingester", "tenant-a", "dataset-a"},
				},
				{
					Id:          md2.Id,
					Tenant:      1,
					Shard:       1,
					MinTime:     minT,
					MaxTime:     maxT,
					CreatedBy:   2,
					Datasets:    []*metastorev1.Dataset{{Tenant: 1, Name: 3, MinTime: minT, MaxTime: maxT}},
					StringTable: []string{"", "tenant-a", "ingester", "dataset-a"},
				},
				{
					Id:          md3.Id,
					Tenant:      1,
					Shard:       1,
					MinTime:     minT,
					MaxTime:     maxT,
					CreatedBy:   2,
					Datasets:    []*metastorev1.Dataset{{Tenant: 1, Name: 3, MinTime: minT, MaxTime: maxT}},
					StringTable: []string{"", "tenant-a", "ingester", "dataset-a"},
				},
			}

			found, err := index.QueryMetadata(tx, ctx, MetadataQuery{
				Expr:      `{service_name=~"dataset-a"}`,
				StartTime: time.UnixMilli(minT),
				EndTime:   time.UnixMilli(maxT),
				Tenant:    []string{"tenant-a", "tenant-b"},
			})
			require.NoError(t, err)
			require.Equal(t, expected, found)
		})

		t.Run("DatasetTenantFilter", func(t *testing.T) {
			expected := []*metastorev1.BlockMeta{
				{
					Id:          md.Id,
					Tenant:      0,
					MinTime:     minT,
					MaxTime:     maxT,
					CreatedBy:   1,
					Datasets:    []*metastorev1.Dataset{{Tenant: 2, Name: 3, MinTime: minT, MaxTime: maxT}},
					StringTable: []string{"", "ingester", "tenant-b", "dataset-b"},
				},
			}

			found, err := index.QueryMetadata(tx, ctx, MetadataQuery{
				Expr:      `{}`,
				StartTime: time.UnixMilli(minT),
				EndTime:   time.UnixMilli(maxT + 1),
				Tenant:    []string{"tenant-b"},
			})
			require.NoError(t, err)
			require.Equal(t, expected, found)
		})

		t.Run("DatasetTenantFilterNotExisting", func(t *testing.T) {
			found, err := index.QueryMetadata(tx, ctx, MetadataQuery{
				Expr:      `{}`,
				StartTime: time.UnixMilli(minT),
				EndTime:   time.UnixMilli(maxT + 1),
				Tenant:    []string{"tenant-not-found"},
			})
			require.NoError(t, err)
			require.Empty(t, found)
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
						MaxTime: maxT,
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
						MaxTime: maxT,
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
						MaxTime: maxT,
						Labels:  []int32{1, 4, 3},
					}},
					StringTable: []string{"", "tenant-a", "ingester", "dataset-a", "service_name"},
				},
			}

			found, err := index.QueryMetadata(tx, ctx, MetadataQuery{
				Expr:      `{service_name=~"dataset-a"}`,
				StartTime: time.UnixMilli(minT),
				EndTime:   time.UnixMilli(maxT),
				Tenant:    []string{"tenant-a", "tenant-b"},
				Labels:    []string{"service_name"},
			})
			require.NoError(t, err)
			require.Equal(t, expected, found)
		})

		t.Run("TimeRangeFilter", func(t *testing.T) {
			found, err := index.QueryMetadata(tx, ctx, MetadataQuery{
				Expr:      `{service_name=~"dataset-b"}`,
				StartTime: time.UnixMilli(minT - 3),
				EndTime:   time.UnixMilli(minT - 1), // dataset-b starts at minT
				Tenant:    []string{"tenant-b"},
			})
			require.NoError(t, err)
			require.Empty(t, found)
		})

		t.Run("Labels", func(t *testing.T) {
			labels, err := index.QueryMetadataLabels(tx, ctx, MetadataQuery{
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

		t.Run("LabelsTenantFilter", func(t *testing.T) {
			labels, err := index.QueryMetadataLabels(tx, ctx, MetadataQuery{
				Expr:      "{}",
				StartTime: time.UnixMilli(minT),
				EndTime:   time.UnixMilli(maxT),
				Tenant:    []string{"tenant-b"},
				Labels: []string{
					model.LabelNameProfileType,
					model.LabelNameServiceName,
				},
			})
			require.NoError(t, err)
			require.NotEmpty(t, labels)
			assert.Equal(t, []*typesv1.Labels{{Labels: []*typesv1.LabelPair{
				{Name: model.LabelNameProfileType, Value: "4"},
				{Name: model.LabelNameServiceName, Value: "dataset-b"},
			}}}, labels)
		})
	}

	idx := NewIndex(util.Logger, NewStore(), DefaultConfig)
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
		idx = NewIndex(util.Logger, NewStore(), DefaultConfig)
		tx, err = db.Begin(false)
		defer func() {
			require.NoError(t, tx.Rollback())
		}()
		require.NoError(t, err)
		require.NoError(t, idx.Restore(tx))
		query(t, tx, idx)
	})
}

func TestIndex_QueryConcurrency(t *testing.T) {
	const N = 10
	for i := 0; i < N && !t.Failed(); i++ {
		q := new(queryTestSuite)
		q.run(t)
	}
}

type queryTestSuite struct {
	db     *bbolt.DB
	idx    *Index
	blocks atomic.Pointer[metastorev1.BlockList]

	from   string
	tenant string
	shard  uint32

	wg     sync.WaitGroup
	stop   chan struct{}
	doStop func()

	writes  atomic.Int32
	queries atomic.Int32
}

// Possible invariants:
// 1. (001) No blocks are found.
// 2. (010) Only source blocks are found (1-10).
// 3. (100) Only compacted blocks are found (always 4).

const (
	noBlocks = 1 << iota
	sourceBlocks
	compactedBlocks

	all = noBlocks | sourceBlocks | compactedBlocks
)

func (s *queryTestSuite) setup(t *testing.T) {
	var once sync.Once
	s.stop = make(chan struct{})
	s.doStop = func() {
		once.Do(func() {
			close(s.stop)
		})
	}

	s.from = "2024-09-23T08:00:00.000Z"
	s.tenant = "tenant"
	s.shard = 1
	s.blocks.Store(&metastorev1.BlockList{})

	s.db = test.BoltDB(t)
	s.idx = NewIndex(util.Logger, NewStore(), DefaultConfig)
	// Enforce aggressive cache evictions:
	s.idx.config.partitionDuration = time.Minute * 30
	s.idx.config.ShardCacheSize = 3
	s.idx.config.BlockReadCacheSize = 3
	s.idx.config.BlockWriteCacheSize = 3
	require.NoError(t, s.db.Update(s.idx.Init))
}

func (s *queryTestSuite) teardown(t *testing.T) {
	require.NoError(t, s.db.Close())
}

func (s *queryTestSuite) run(t *testing.T) {
	s.setup(t)
	defer s.teardown(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-s.stop:
				return
			default:
				s.writeBlocks(t)
			}
		}
	}()

	ctx := context.Background()
	s.runQuery(t, ctx, s.queryBlocks)
	s.runQuery(t, ctx, s.queryLabels)
	s.runQuery(t, ctx, s.getBlocks)

	go func() {
		select {
		case <-s.stop:
		case <-time.After(30 * time.Second):
			t.Error("test time out: query consistency not confirmed")
			s.doStop()
		}
	}()

	s.wg.Wait()
	// If we haven't failed the test, we can conclude that
	// no races, no deadlocks, no inconsistencies were found.
	s.doStop()
	// Wait for the write goroutine to finish, so we can
	// safely tear down the test.
	<-done
	t.Logf("writes: %d, queries: %d", s.writes.Load(), s.queries.Load())
}

func (s *queryTestSuite) createBlock(id ulid.ULID, dur time.Duration, tenant string, shard, level uint32) *metastorev1.BlockMeta {
	minT := ulid.Time(id.Time()).UnixMilli()
	maxT := minT + dur.Milliseconds()
	tid := int32(0)
	if level > 0 {
		tid = 1
	}
	return &metastorev1.BlockMeta{
		Id:              id.String(),
		Tenant:          tid,
		Shard:           shard,
		MinTime:         minT,
		MaxTime:         maxT,
		CompactionLevel: level,
		Datasets:        []*metastorev1.Dataset{{Tenant: 1, MinTime: minT, MaxTime: maxT, Labels: []int32{1, 2, 3}}},
		StringTable:     []string{"", tenant, "service_name", "service"},
	}
}

func (s *queryTestSuite) createBlocks(from time.Time, dur time.Duration, n int, tenant string, shard, level uint32) (blocks []*metastorev1.BlockMeta) {
	cur := from
	for i := 0; i < n; i++ {
		b := s.createBlock(ulid.MustNew(ulid.Timestamp(cur), rand.Reader), dur, tenant, shard, level)
		blocks = append(blocks, b)
		cur = cur.Add(dur)
	}
	return blocks
}

func (s *queryTestSuite) writeBlocks(t *testing.T) {
	t.Helper()

	// Create source blocks.
	source := s.createBlocks(test.Time(s.from), time.Minute*10, 10, s.tenant, s.shard, 0)
	sourceList := &metastorev1.BlockList{
		// Tenant: s.tenant, // O level blocks are anonymous.
		Shard:  s.shard,
		Blocks: make([]string, len(source)),
	}
	for i, b := range source {
		sourceList.Blocks[i] = b.Id
	}
	// Blocks are inserted one by one, each within its own transaction.
	for i := range source {
		require.NoError(t, s.db.Update(func(tx *bbolt.Tx) error {
			return s.idx.InsertBlock(tx, source[i])
		}))
		s.writes.Inc()
	}

	// We make the blocks visible to our test queries.
	s.blocks.Store(sourceList)
	// Give other goroutines a chance.
	runtime.Gosched()

	// Replace with compacted.
	compacted := s.createBlocks(test.Time(s.from), time.Minute*15, 4, s.tenant, s.shard, 1)
	compactedList := &metastorev1.BlockList{
		Tenant: s.tenant,
		Shard:  s.shard,
		Blocks: make([]string, len(compacted)),
	}
	for i, b := range compacted {
		compactedList.Blocks[i] = b.Id
	}
	require.NoError(t, s.db.Update(func(tx *bbolt.Tx) error {
		return s.idx.ReplaceBlocks(tx, &metastorev1.CompactedBlocks{
			SourceBlocks: sourceList,
			NewBlocks:    compacted,
		})
	}))
	s.writes.Inc()

	// After we replaced the source blocks with compacted blocks,
	// we want our test queries to check them.
	s.blocks.Store(compactedList)
	runtime.Gosched()

	// Delete all blocks.
	require.NoError(t, s.db.Update(func(tx *bbolt.Tx) error {
		return s.idx.ReplaceBlocks(tx, &metastorev1.CompactedBlocks{
			SourceBlocks: compactedList,
		})
	}))
	s.writes.Inc()

	s.blocks.Store(&metastorev1.BlockList{})
	runtime.Gosched()
}

func (s *queryTestSuite) runQuery(t *testing.T, ctx context.Context, q func(*testing.T, context.Context) int32) {
	t.Helper()

	var ret int32
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.stop:
				return

			default:
				s.queries.Inc()
				x := q(t, ctx)
				if x < 0 {
					s.doStop()
					return
				}
				if ret |= x; ret == all {
					return
				}
			}
		}
	}()
}

func (s *queryTestSuite) queryBlocks(t *testing.T, ctx context.Context) (ret int32) {
	var x []*metastorev1.BlockMeta
	var err error
	require.NoError(t, s.db.View(func(tx *bbolt.Tx) error {
		x, err = s.idx.QueryMetadata(tx, ctx, MetadataQuery{
			Expr:      `{service_name="service"}`,
			StartTime: test.Time(s.from),
			EndTime:   test.Time(s.from).Add(2 * time.Hour),
			Tenant:    []string{s.tenant},
			Labels:    []string{"service_name"},
		})
		return err
	}))

	// It's expected that we may query the data before
	// any blocks are written.
	if len(x) == 0 {
		return noBlocks
	}
	var c uint32
	for i := range x {
		c += x[i].CompactionLevel
	}

	if len(x) <= 10 && c == 0 {
		// All of level 0: note that the source blocks
		// may be seen while they are being inserted.
		return sourceBlocks
	}

	if len(x) == int(c) && c == 4 {
		// All of level 1: note that compacted blocks
		// should be added atomically.
		return compactedBlocks
	}

	t.Error("query blocks: inconsistent results")
	for i := range x {
		t.Log("\t", x[i])
	}

	return -1
}

func (s *queryTestSuite) queryLabels(t *testing.T, ctx context.Context) (ret int32) {
	var x []*typesv1.Labels
	var err error
	require.NoError(t, s.db.View(func(tx *bbolt.Tx) error {
		x, err = s.idx.QueryMetadataLabels(tx, ctx, MetadataQuery{
			Expr:      `{service_name="service"}`,
			StartTime: test.Time(s.from),
			EndTime:   test.Time(s.from).Add(2 * time.Hour),
			Tenant:    []string{s.tenant},
			Labels:    []string{"service_name"},
		})
		return err
	}))

	if len(x) == 0 {
		return noBlocks
	}

	// Inconsistent labels/strings.
	assert.EqualValues(t, 1, len(x))
	assert.EqualValues(t, 1, len(x[0].Labels))
	assert.Equal(t, x[0].Labels[0].Name, "service_name")
	assert.Equal(t, x[0].Labels[0].Value, "service")

	// We can't distinguish between source
	// and compacted blocks here.
	return sourceBlocks | compactedBlocks
}

func (s *queryTestSuite) getBlocks(t *testing.T, _ context.Context) (ret int32) {
	var x []*metastorev1.BlockMeta
	var err error
	// The writer ensures that the list is set after it finished writes.
	// If we get the list within the transaction, we may observe partial
	// source blocks [0-9]: this means the read transaction was open while
	// not all the blocks were written.
	blocks := s.blocks.Load()
	require.NoError(t, s.db.View(func(tx *bbolt.Tx) error {
		x, err = s.idx.GetBlocks(tx, blocks)
		return err
	}))

	// Same as queryBlocks except that we do not expect
	// to see partial source blocks.
	if len(x) == 0 {
		return noBlocks
	}
	var c uint32
	for i := range x {
		c += x[i].CompactionLevel
	}

	if len(x) == 10 && c == 0 {
		return sourceBlocks
	}

	if len(x) == int(c) && c == 4 {
		return compactedBlocks
	}

	t.Error("find blocks: inconsistent results")
	for i := range x {
		t.Log("\t", x[i])
	}

	return -1
}
