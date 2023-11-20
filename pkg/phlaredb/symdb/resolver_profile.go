package symdb

import (
	"sync"

	"github.com/google/pprof/profile"

	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type profileSymbols struct {
	profile *profile.Profile
	symbols *Symbols
	samples *schemav1.Samples
	cur     int

	locations []*profile.Location
	mappings  []*profile.Mapping
	functions []*profile.Function
}

var profileSymbolsPool = sync.Pool{
	New: func() any { return new(profileSymbols) },
}

func profileSymbolsFromPool() *profileSymbols {
	return profileSymbolsPool.Get().(*profileSymbols)
}

func (r *profileSymbols) reset() {
	r.profile = nil
	r.symbols = nil
	r.samples = nil
	r.cur = 0
	clear(r.locations)
	clear(r.mappings)
	clear(r.functions)
	profileSymbolsPool.Put(r)
}

func (r *profileSymbols) init(symbols *Symbols, samples schemav1.Samples) {
	r.symbols = symbols
	r.samples = &samples
	r.profile = &profile.Profile{
		Sample:     make([]*profile.Sample, len(samples.StacktraceIDs)),
		PeriodType: new(profile.ValueType),
	}
	r.locations = grow(r.locations, len(r.symbols.Locations))
	r.mappings = grow(r.mappings, len(r.symbols.Mappings))
	r.functions = grow(r.functions, len(r.symbols.Functions))
}

func grow[T any](s []T, n int) []T {
	if cap(s) < n {
		s = make([]T, n)
	}
	s = s[:n]
	return s
}

func clear[T any](s []T) {
	var zero T
	for i := range s {
		s[i] = zero
	}
}

func (r *profileSymbols) InsertStacktrace(_ uint32, locations []int32) {
	sample := &profile.Sample{
		Location: make([]*profile.Location, len(locations)),
		Value:    []int64{int64(r.samples.Values[r.cur])},
	}
	for j, loc := range locations {
		sample.Location[j] = r.location(loc)
	}
	r.profile.Sample[r.cur] = sample
	r.cur++
}

func (r *profileSymbols) location(i int32) *profile.Location {
	if x := r.locations[i]; x != nil {
		return x
	}
	loc := r.inMemoryLocationToPprof(r.symbols.Locations[i])
	r.profile.Location = append(r.profile.Location, loc)
	r.locations[i] = loc
	return loc
}

func (r *profileSymbols) mapping(i uint32) *profile.Mapping {
	if x := r.mappings[i]; x != nil {
		return x
	}
	m := r.inMemoryMappingToPprof(r.symbols.Mappings[i])
	r.profile.Mapping = append(r.profile.Mapping, m)
	r.mappings[i] = m
	return m
}

func (r *profileSymbols) function(i uint32) *profile.Function {
	if x := r.functions[i]; x != nil {
		return x
	}
	f := r.inMemoryFunctionToPprof(r.symbols.Functions[i])
	r.profile.Function = append(r.profile.Function, f)
	r.functions[i] = f
	return f
}

func (r *profileSymbols) inMemoryLocationToPprof(m *schemav1.InMemoryLocation) *profile.Location {
	x := &profile.Location{
		ID:       m.Id,
		Mapping:  r.mapping(m.MappingId),
		Address:  m.Address,
		IsFolded: m.IsFolded,
	}
	x.Line = make([]profile.Line, len(m.Line))
	for i, line := range m.Line {
		x.Line[i] = profile.Line{
			Function: r.function(line.FunctionId),
			Line:     int64(line.Line),
		}
	}
	return x
}

func (r *profileSymbols) inMemoryMappingToPprof(m *schemav1.InMemoryMapping) *profile.Mapping {
	return &profile.Mapping{
		ID:              m.Id,
		Start:           m.MemoryStart,
		Limit:           m.MemoryLimit,
		Offset:          m.FileOffset,
		File:            r.symbols.Strings[m.Filename],
		BuildID:         r.symbols.Strings[m.BuildId],
		HasFunctions:    m.HasFunctions,
		HasFilenames:    m.HasFilenames,
		HasLineNumbers:  m.HasLineNumbers,
		HasInlineFrames: m.HasInlineFrames,
	}
}

func (r *profileSymbols) inMemoryFunctionToPprof(m *schemav1.InMemoryFunction) *profile.Function {
	return &profile.Function{
		ID:         m.Id,
		Name:       r.symbols.Strings[m.Name],
		SystemName: r.symbols.Strings[m.SystemName],
		Filename:   r.symbols.Strings[m.Filename],
		StartLine:  int64(m.StartLine),
	}
}

func (r *profileSymbols) incrementIDs() {
	for i, l := range r.profile.Location {
		l.ID = uint64(i) + 1
	}
	for i, f := range r.profile.Function {
		f.ID = uint64(i) + 1
	}
	for i, m := range r.profile.Mapping {
		m.ID = uint64(i) + 1
	}
}
