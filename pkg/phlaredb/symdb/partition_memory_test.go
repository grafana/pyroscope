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

	db := NewSymDB(new(Config))
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

	// Allocate stacktrace IDs.
	sids := make([]uint32, len(stacktraces))
	db := NewSymDB(new(Config))
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
		db := NewSymDB(new(Config))

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
