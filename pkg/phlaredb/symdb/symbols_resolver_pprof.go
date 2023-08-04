package symdb

import (
	"context"
	"sync"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type StacktraceResolver interface {
	// ResolveStacktraces resolves locations for each stack trace
	// and inserts it to the StacktraceInserter provided.
	//
	// The stacktraces must be ordered in the ascending order.
	// If a stacktrace can't be resolved, dst receives an empty
	// array of locations.
	//
	// Stacktraces slice might be modified during the call.
	ResolveStacktraces(ctx context.Context, dst StacktraceInserter, stacktraces []uint32) error
}

// StacktraceInserter accepts resolved locations for a given stack trace.
// The leaf is at locations[0].
//
// Locations slice must not be retained by implementation.
// It is guaranteed, that for a given stacktrace ID
// InsertStacktrace is called not more than once.
type StacktraceInserter interface {
	InsertStacktrace(stacktraceID uint32, locations []int32)
}

type StacktraceInserterFn func(stacktraceID uint32, locations []int32)

func (fn StacktraceInserterFn) InsertStacktrace(stacktraceID uint32, locations []int32) {
	fn(stacktraceID, locations)
}

type Resolver struct {
	Stacktraces StacktraceResolver
	Locations   []*v1.InMemoryLocation
	Mappings    []*v1.InMemoryMapping
	Functions   []*v1.InMemoryFunction
	Strings     []string
}

func (r *Resolver) ResolvePprof(ctx context.Context, samples v1.Samples) (*googlev1.Profile, error) {
	t := pprofResolveFromPool()
	defer t.reset()
	t.init(r, samples)
	if err := r.Stacktraces.ResolveStacktraces(ctx, t, samples.StacktraceIDs); err != nil {
		return nil, err
	}
	return t.profile, nil
}

type pprofResolve struct {
	profile  *googlev1.Profile
	resolver *Resolver
	samples  *v1.Samples
	cur      int

	locations []int32
	mappings  []int32
	functions []int32
	strings   []int32
}

var pprofResolvePool = sync.Pool{
	New: func() any { return new(pprofResolve) },
}

func pprofResolveFromPool() *pprofResolve {
	return pprofResolvePool.Get().(*pprofResolve)
}

func (r *pprofResolve) reset() {
	r.profile = nil
	r.resolver = nil
	r.samples = nil
	r.cur = 0
	pprofResolvePool.Put(r)
}

func (r *pprofResolve) init(resolver *Resolver, samples v1.Samples) {
	r.resolver = resolver
	r.samples = &samples
	r.profile = &googlev1.Profile{
		Sample: make([]*googlev1.Sample, len(samples.StacktraceIDs)),
	}
	r.locations = r.initSlice(r.locations, len(r.locations))
	r.mappings = r.initSlice(r.mappings, len(r.mappings))
	r.functions = r.initSlice(r.functions, len(r.functions))
	r.strings = r.initSlice(r.strings, len(r.strings))
}

func (*pprofResolve) initSlice(s []int32, n int) []int32 {
	if cap(s) < n {
		s = make([]int32, n)
	}
	s = s[:n]
	// Zero is valid value and can't be used, therefore a sentinel
	// value is used to indicate that the slot is empty.
	for i := range s {
		s[i] = -1
	}
	return s
}

func (r *pprofResolve) InsertStacktrace(_ uint32, locations []int32) {
	var sample googlev1.Sample
	sample.Value = []int64{int64(r.samples.Values[r.cur])}
	sample.LocationId = make([]uint64, len(locations))
	for j, loc := range locations {
		sample.LocationId[j] = r.location(loc)
	}
	r.profile.Sample[r.cur] = &sample
	r.cur++
}

func (r *pprofResolve) location(i int32) uint64 {
	if x := r.locations[i]; x > 0 {
		return uint64(x)
	}
	v := int32(len(r.profile.Location))
	loc := inMemoryLocationToPprof(r.resolver.Locations[i])
	loc.MappingId = r.mapping(loc.MappingId)
	for _, line := range loc.Line {
		line.FunctionId = r.function(line.FunctionId)
	}
	r.profile.Location = append(r.profile.Location, loc)
	r.locations[i] = v
	return uint64(v)
}

func (r *pprofResolve) mapping(i uint64) uint64 {
	if x := r.mappings[i]; x > 0 {
		return uint64(x)
	}
	v := int32(len(r.profile.Mapping))
	m := inMemoryMappingToPprof(r.resolver.Mappings[i])
	m.BuildId = r.string(m.BuildId)
	m.Filename = r.string(m.Filename)
	r.profile.Mapping = append(r.profile.Mapping, m)
	r.mappings[i] = v
	return uint64(v)
}

func (r *pprofResolve) function(i uint64) uint64 {
	if x := r.functions[i]; x > 0 {
		return uint64(x)
	}
	v := int32(len(r.profile.Function))
	f := inMemoryFunctionToPprof(r.resolver.Functions[i])
	f.Name = r.string(f.Name)
	f.Filename = r.string(f.Filename)
	f.SystemName = r.string(f.SystemName)
	r.profile.Function = append(r.profile.Function, f)
	r.functions[i] = v
	return uint64(v)
}

func (r *pprofResolve) string(i int64) int64 {
	if x := r.strings[i]; x > 0 {
		return int64(x)
	}
	v := int32(len(r.profile.StringTable))
	r.profile.StringTable = append(r.profile.StringTable, r.resolver.Strings[i])
	r.strings[i] = v
	return int64(v)
}

func inMemoryLocationToPprof(m *v1.InMemoryLocation) *googlev1.Location {
	x := &googlev1.Location{
		Id:        m.Id,
		MappingId: uint64(m.MappingId),
		Address:   m.Address,
		IsFolded:  m.IsFolded,
	}
	x.Line = make([]*googlev1.Line, len(m.Line))
	for i, line := range m.Line {
		x.Line[i] = &googlev1.Line{
			FunctionId: uint64(line.FunctionId),
			Line:       int64(line.Line),
		}
	}
	return x
}

func inMemoryMappingToPprof(m *v1.InMemoryMapping) *googlev1.Mapping {
	return &googlev1.Mapping{
		Id:              m.Id,
		MemoryStart:     m.MemoryStart,
		MemoryLimit:     m.MemoryLimit,
		FileOffset:      m.FileOffset,
		Filename:        int64(m.Filename),
		BuildId:         int64(m.BuildId),
		HasFunctions:    m.HasFunctions,
		HasFilenames:    m.HasFilenames,
		HasLineNumbers:  m.HasLineNumbers,
		HasInlineFrames: m.HasInlineFrames,
	}
}

func inMemoryFunctionToPprof(m *v1.InMemoryFunction) *googlev1.Function {
	return &googlev1.Function{
		Id:         m.Id,
		Name:       int64(m.Name),
		SystemName: int64(m.SystemName),
		Filename:   int64(m.Filename),
		StartLine:  int64(m.StartLine),
	}
}
