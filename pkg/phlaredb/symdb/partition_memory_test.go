package symdb

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/pprof"
)

func Test_StacktraceAppender_shards(t *testing.T) {
	t.Run("WithMaxStacktraceTreeNodesPerChunk", func(t *testing.T) {
		db := NewSymDB(&Config{
			Stacktraces: StacktracesConfig{
				MaxNodesPerChunk: 7,
			},
		})

		w := db.PartitionWriter(0)
		sids := make([]uint32, 4)
		w.AppendStacktraces(sids, []*schemav1.Stacktrace{
			{LocationIDs: []uint64{3, 2, 1}},
			{LocationIDs: []uint64{2, 1}},
			{LocationIDs: []uint64{4, 3, 2, 1}},
			{LocationIDs: []uint64{3, 1}},
		})
		assert.Equal(t, []uint32{3, 2, 11, 16}, sids)

		w.AppendStacktraces(sids[:3], []*schemav1.Stacktrace{
			{LocationIDs: []uint64{3, 2, 1}},
			{LocationIDs: []uint64{2, 1}},
			{LocationIDs: []uint64{4, 3, 2, 1}},
		})
		// Same input. Note that len(sids) > len(schemav1.Stacktrace)
		assert.Equal(t, []uint32{3, 2, 11}, sids[:3])

		w.AppendStacktraces(sids[:1], []*schemav1.Stacktrace{
			{LocationIDs: []uint64{5, 2, 1}},
		})
		assert.Equal(t, []uint32{18}, sids[:1])

		require.Len(t, db.partitions, 1)
		m := db.partitions[0]
		require.Len(t, m.stacktraces.chunks, 3)

		c1 := m.stacktraces.chunks[0]
		assert.Equal(t, uint32(0), c1.stid)
		assert.Equal(t, uint32(4), c1.tree.len())

		c2 := m.stacktraces.chunks[1]
		assert.Equal(t, uint32(7), c2.stid)
		assert.Equal(t, uint32(5), c2.tree.len())

		c3 := m.stacktraces.chunks[2]
		assert.Equal(t, uint32(14), c3.stid)
		assert.Equal(t, uint32(5), c3.tree.len())
	})

	t.Run("WithoutMaxStacktraceTreeNodesPerChunk", func(t *testing.T) {
		db := NewSymDB(new(Config))
		w := db.PartitionWriter(0)
		sids := make([]uint32, 5)
		w.AppendStacktraces(sids, []*schemav1.Stacktrace{
			{LocationIDs: []uint64{3, 2, 1}},
			{LocationIDs: []uint64{2, 1}},
			{LocationIDs: []uint64{4, 3, 2, 1}},
			{LocationIDs: []uint64{3, 1}},
			{LocationIDs: []uint64{5, 3, 2, 1}},
		})
		assert.Equal(t, []uint32{3, 2, 4, 5, 6}, sids)

		require.Len(t, db.partitions, 1)
		m := db.partitions[0]
		require.Len(t, m.stacktraces.chunks, 1)

		c1 := m.stacktraces.chunks[0]
		assert.Equal(t, uint32(0), c1.stid)
		assert.Equal(t, uint32(7), c1.tree.len())
	})
}

func Test_Stacktrace_append_existing(t *testing.T) {
	db := NewSymDB(new(Config))
	w := db.PartitionWriter(0)
	sids := make([]uint32, 2)
	w.AppendStacktraces(sids, []*schemav1.Stacktrace{
		{LocationIDs: []uint64{5, 4, 3, 2, 1}},
		{LocationIDs: []uint64{5, 4, 3, 2, 1}},
	})
	assert.Equal(t, []uint32{5, 5}, sids)

	w.AppendStacktraces(sids, []*schemav1.Stacktrace{
		{LocationIDs: []uint64{5, 4, 3, 2, 1}},
		{LocationIDs: []uint64{6, 5, 4, 3, 2, 1}},
	})
	assert.Equal(t, []uint32{5, 6}, sids)
}

func Test_Stacktrace_append_empty(t *testing.T) {
	db := NewSymDB(new(Config))
	w := db.PartitionWriter(0)

	sids := make([]uint32, 2)
	w.AppendStacktraces(sids, nil)
	assert.Equal(t, []uint32{0, 0}, sids)

	w.AppendStacktraces(sids, []*schemav1.Stacktrace{})
	assert.Equal(t, []uint32{0, 0}, sids)

	w.AppendStacktraces(sids, []*schemav1.Stacktrace{{}})
	assert.Equal(t, []uint32{0, 0}, sids)
}

func Test_Stacktraces_append_resolve(t *testing.T) {
	ctx := context.Background()

	t.Run("single chunk", func(t *testing.T) {
		db := NewSymDB(new(Config))
		w := db.PartitionWriter(0)

		sids := make([]uint32, 5)
		w.AppendStacktraces(sids, []*schemav1.Stacktrace{
			{LocationIDs: []uint64{3, 2, 1}},
			{LocationIDs: []uint64{2, 1}},
			{LocationIDs: []uint64{4, 3, 2, 1}},
			{LocationIDs: []uint64{3, 1}},
			{LocationIDs: []uint64{5, 2, 1}},
		})

		r, ok := db.lookupPartition(0)
		require.True(t, ok)
		dst := new(mockStacktraceInserter)
		dst.On("InsertStacktrace", uint32(2), []int32{2, 1})
		dst.On("InsertStacktrace", uint32(3), []int32{3, 2, 1})
		dst.On("InsertStacktrace", uint32(4), []int32{4, 3, 2, 1})
		dst.On("InsertStacktrace", uint32(5), []int32{3, 1})
		dst.On("InsertStacktrace", uint32(6), []int32{5, 2, 1})
		require.NoError(t, r.ResolveStacktraceLocations(ctx, dst, []uint32{2, 3, 4, 5, 6}))
	})

	t.Run("multiple chunks", func(t *testing.T) {
		db := NewSymDB(&Config{
			Stacktraces: StacktracesConfig{
				MaxNodesPerChunk: 7,
			},
		})

		w := db.PartitionWriter(0)
		stacktraces := []*schemav1.Stacktrace{ // ID, Chunk ID:
			{LocationIDs: []uint64{3, 2, 1}},        // 3  0
			{LocationIDs: []uint64{2, 1}},           // 2  0
			{LocationIDs: []uint64{4, 3, 2, 1}},     // 11 1
			{LocationIDs: []uint64{3, 1}},           // 16 2
			{LocationIDs: []uint64{5, 2, 1}},        // 18 2
			{LocationIDs: []uint64{13, 12, 11}},     // 24 3
			{LocationIDs: []uint64{12, 11}},         // 23 3
			{LocationIDs: []uint64{14, 13, 12, 11}}, // 32 4
			{LocationIDs: []uint64{13, 11}},         // 37 5
			{LocationIDs: []uint64{15, 12, 11}},     // 39 5
		}
		/*
			// TODO(kolesnikovae): Add test cases:
			// Invariants:
			//        0
			//      1
			//      1 0
			//    2
			//    2   0
			//    2 1
			//    2 1 0
			//  3
			//  3     0
			//  3   1
			//  3   1 0
			//  3 2
			//  3 2   0
			//  3 2 1
			//  3 2 1 0
		*/
		sids := make([]uint32, len(stacktraces))
		w.AppendStacktraces(sids, stacktraces)
		require.Len(t, db.partitions[0].stacktraces.chunks, 6)

		t.Run("adjacent shards at beginning", func(t *testing.T) {
			r, _ := db.lookupPartition(0)
			dst := new(mockStacktraceInserter)
			dst.On("InsertStacktrace", uint32(2), []int32{2, 1})
			dst.On("InsertStacktrace", uint32(3), []int32{3, 2, 1})
			dst.On("InsertStacktrace", uint32(11), []int32{4, 3, 2, 1})
			dst.On("InsertStacktrace", uint32(16), []int32{3, 1})
			dst.On("InsertStacktrace", uint32(18), []int32{5, 2, 1})
			require.NoError(t, r.ResolveStacktraceLocations(ctx, dst, []uint32{2, 3, 11, 16, 18}))
		})

		t.Run("adjacent shards at end", func(t *testing.T) {
			r, _ := db.lookupPartition(0)
			dst := new(mockStacktraceInserter)
			dst.On("InsertStacktrace", uint32(23), []int32{12, 11})
			dst.On("InsertStacktrace", uint32(24), []int32{13, 12, 11})
			dst.On("InsertStacktrace", uint32(32), []int32{14, 13, 12, 11})
			dst.On("InsertStacktrace", uint32(37), []int32{13, 11})
			dst.On("InsertStacktrace", uint32(39), []int32{15, 12, 11})
			require.NoError(t, r.ResolveStacktraceLocations(ctx, dst, []uint32{23, 24, 32, 37, 39}))
		})

		t.Run("non-adjacent shards", func(t *testing.T) {
			r, _ := db.lookupPartition(0)
			dst := new(mockStacktraceInserter)
			dst.On("InsertStacktrace", uint32(11), []int32{4, 3, 2, 1})
			dst.On("InsertStacktrace", uint32(32), []int32{14, 13, 12, 11})
			require.NoError(t, r.ResolveStacktraceLocations(ctx, dst, []uint32{11, 32}))
		})
	})
}

func Test_hashLocations(t *testing.T) {
	t.Run("hashLocations is thread safe", func(t *testing.T) {
		b := []uint64{123, 234, 345, 456, 567}
		h := hashLocations(b)
		const N, M = 10, 10 << 10
		var wg sync.WaitGroup
		wg.Add(N)
		for i := 0; i < N; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < M; j++ {
					if hashLocations(b) != h {
						panic("hash mismatch")
					}
				}
			}()
		}
		wg.Wait()
	})
}

func Test_Stacktraces_memory_resolve_pprof(t *testing.T) {
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)
	stacktraces := pprofSampleToStacktrace(p.Sample)
	sids := make([]uint32, len(stacktraces))

	db := NewSymDB(new(Config))
	w := db.PartitionWriter(0)
	w.AppendStacktraces(sids, stacktraces)

	r, ok := db.lookupPartition(0)
	require.True(t, ok)

	si := newStacktracesMapInserter()
	err = r.ResolveStacktraceLocations(context.Background(), si, sids)
	require.NoError(t, err)

	si.assertValid(t, sids, stacktraces)
}

func Test_Stacktraces_memory_resolve_chunked(t *testing.T) {
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)
	stacktraces := pprofSampleToStacktrace(p.Sample)
	sids := make([]uint32, len(stacktraces))

	cfg := &Config{
		Stacktraces: StacktracesConfig{
			MaxNodesPerChunk: 256,
		},
	}
	db := NewSymDB(cfg)
	w := db.PartitionWriter(0)
	w.AppendStacktraces(sids, stacktraces)

	r, ok := db.lookupPartition(0)
	require.True(t, ok)

	// ResolveStacktraceLocations modifies sids in-place,
	// if stacktraces are chunked.
	sidsCopy := make([]uint32, len(sids))
	copy(sidsCopy, sids)

	si := newStacktracesMapInserter()
	err = r.ResolveStacktraceLocations(context.Background(), si, sids)
	require.NoError(t, err)

	si.assertValid(t, sidsCopy, stacktraces)
}

// Test_Stacktraces_memory_resolve_concurrency validates if concurrent
// append and resolve do not cause race conditions.
func Test_Stacktraces_memory_resolve_concurrency(t *testing.T) {
	p, err := pprof.OpenFile("testdata/profile.pb.gz")
	require.NoError(t, err)
	stacktraces := pprofSampleToStacktrace(p.Sample)

	cfg := &Config{
		Stacktraces: StacktracesConfig{
			MaxNodesPerChunk: 256,
		},
	}

	// Allocate stacktrace IDs.
	sids := make([]uint32, len(stacktraces))
	db := NewSymDB(cfg)
	w := db.PartitionWriter(0)
	w.AppendStacktraces(sids, stacktraces)

	const (
		iterations = 10
		resolvers  = 100
		appenders  = 5
		appends    = 100
	)

	runTest := func(t *testing.T) {
		t.Helper()
		db := NewSymDB(cfg)

		var wg sync.WaitGroup
		wg.Add(appenders)
		for i := 0; i < appenders; i++ {
			go func() {
				defer wg.Done()
				w := db.PartitionWriter(0)
				for j := 0; j < appends; j++ {
					w.AppendStacktraces(make([]uint32, len(stacktraces)), stacktraces)
				}
			}()
		}

		wg.Add(resolvers)
		for i := 0; i < resolvers; i++ {
			go func() {
				defer wg.Done()

				r, ok := db.lookupPartition(0)
				if !ok {
					return
				}

				// ResolveStacktraceLocations modifies sids in-place,
				// if stacktraces are chunked.
				sidsCopy := make([]uint32, len(sids))
				copy(sidsCopy, sids)

				// It's expected that only fraction of stack traces may not
				// be appended by the time of querying, therefore validation
				// of the result is omitted (covered separately).
				si := newStacktracesMapInserter()
				_ = r.ResolveStacktraceLocations(context.Background(), si, sidsCopy)
			}()
		}

		wg.Wait()
	}

	for i := 0; i < iterations; i++ {
		runTest(t)
	}
}

type stacktracesMapInserter struct {
	m map[uint32][]int32 // Stacktrace ID => resolved locations

	unresolved int
}

func newStacktracesMapInserter() *stacktracesMapInserter {
	return &stacktracesMapInserter{m: make(map[uint32][]int32)}
}

func (m *stacktracesMapInserter) InsertStacktrace(sid uint32, locations []int32) {
	if len(locations) == 0 {
		m.unresolved++
		return
	}
	s := make([]int32, len(locations)) // InsertStacktrace must not retain input locations.
	copy(s, locations)
	m.m[sid] = s
}

func (m *stacktracesMapInserter) assertValid(t *testing.T, sids []uint32, stacktraces []*schemav1.Stacktrace) {
	assert.LessOrEqual(t, len(m.m), len(sids))
	require.Equal(t, len(sids), len(stacktraces))
	require.Zero(t, m.unresolved)
	for s, sid := range sids {
		locations := stacktraces[s].LocationIDs
		resolved := m.m[sid]
		require.Equal(t, len(locations), len(resolved))
		for i := range locations {
			require.Equal(t, int32(locations[i]), resolved[i])
		}
	}
}

func pprofSampleToStacktrace(samples []*googlev1.Sample) []*schemav1.Stacktrace {
	s := make([]*schemav1.Stacktrace, len(samples))
	for i := range samples {
		s[i] = &schemav1.Stacktrace{LocationIDs: samples[i].LocationId}
	}
	return s
}
