package symdb

import (
	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/slices"
)

type pprofProtoSymbols struct {
	profile googlev1.Profile
	symbols *Symbols
	samples *schemav1.Samples
	lut     []uint32
	cur     int
}

func (r *pprofProtoSymbols) init(symbols *Symbols, samples schemav1.Samples) {
	r.symbols = symbols
	r.samples = &samples
	r.profile.Sample = make([]*googlev1.Sample, samples.Len())
}

func (r *pprofProtoSymbols) InsertStacktrace(_ uint32, locations []int32) {
	s := &googlev1.Sample{
		LocationId: make([]uint64, len(locations)),
		Value:      []int64{int64(r.samples.Values[r.cur])},
	}
	for i, v := range locations {
		s.LocationId[i] = uint64(v)
	}
	r.profile.Sample[r.cur] = s
	r.cur++
}

func (r *pprofProtoSymbols) buildPprof() *googlev1.Profile {
	createSampleTypeStub(&r.profile)
	if r.symbols != nil {
		copyLocations(&r.profile, r.symbols, r.lut)
		copyFunctions(&r.profile, r.symbols, r.lut)
		copyMappings(&r.profile, r.symbols, r.lut)
		copyStrings(&r.profile, r.symbols, r.lut)
	}
	return &r.profile
}

func createSampleTypeStub(profile *googlev1.Profile) {
	profile.PeriodType = new(googlev1.ValueType)
	profile.SampleType = []*googlev1.ValueType{new(googlev1.ValueType)}
}

func copyLocations(profile *googlev1.Profile, symbols *Symbols, lut []uint32) {
	profile.Location = make([]*googlev1.Location, len(symbols.Locations))
	// Copy locations referenced by nodes.
	for _, n := range profile.Sample {
		for _, loc := range n.LocationId {
			if loc == truncationMark {
				// To be replaced with a stub location.
				continue
			}
			if profile.Location[loc] != nil {
				// Already copied: it's expected that
				// the same location is referenced by
				// multiple nodes.
				continue
			}
			src := symbols.Locations[loc]
			// The location identifier is its index
			// in symbols.Locations, therefore it
			// matches the node location reference.
			location := &googlev1.Location{
				Id:        loc,
				MappingId: uint64(src.MappingId),
				Address:   src.Address,
				Line:      make([]*googlev1.Line, len(src.Line)),
				IsFolded:  src.IsFolded,
			}
			for i, line := range src.Line {
				location.Line[i] = &googlev1.Line{
					FunctionId: uint64(line.FunctionId),
					Line:       int64(line.Line),
				}
			}
			profile.Location[loc] = location
		}
	}
	// Now profile.Location contains copies of locations.
	// The slice also has nil items, therefore we need to
	// filter them out.
	n := len(profile.Location)
	lut = slices.GrowLen(lut, n)
	var j int
	for i := 0; i < len(profile.Location); i++ {
		loc := profile.Location[i]
		if loc == nil {
			continue
		}
		oldId := loc.Id
		loc.Id = uint64(j) + 1
		lut[oldId] = uint32(loc.Id)
		profile.Location[j] = loc
		j++
	}
	profile.Location = profile.Location[:j]
	// Next we need to restore references, as the
	// Sample.LocationId identifiers/indices are
	// pointing to the old places.
	for _, s := range profile.Sample {
		for i, loc := range s.LocationId {
			if loc != truncationMark {
				s.LocationId[i] = uint64(lut[loc])
			}
		}
	}
}

func copyFunctions(profile *googlev1.Profile, symbols *Symbols, lut []uint32) {
	profile.Function = make([]*googlev1.Function, len(symbols.Functions))
	for _, loc := range profile.Location {
		for _, line := range loc.Line {
			if profile.Function[line.FunctionId] == nil {
				src := symbols.Functions[line.FunctionId]
				profile.Function[line.FunctionId] = &googlev1.Function{
					Id:         line.FunctionId,
					Name:       int64(src.Name),
					SystemName: int64(src.SystemName),
					Filename:   int64(src.Filename),
					StartLine:  int64(src.StartLine),
				}
			}
		}
	}
	n := len(profile.Function)
	lut = slices.GrowLen(lut, n)
	var j int
	for i := 0; i < len(profile.Function); i++ {
		fn := profile.Function[i]
		if fn == nil {
			continue
		}
		oldId := fn.Id
		fn.Id = uint64(j) + 1
		lut[oldId] = uint32(fn.Id)
		profile.Function[j] = fn
		j++
	}
	profile.Function = profile.Function[:j]
	for _, loc := range profile.Location {
		for _, line := range loc.Line {
			line.FunctionId = uint64(lut[line.FunctionId])
		}
	}
}

func copyMappings(profile *googlev1.Profile, symbols *Symbols, lut []uint32) {
	profile.Mapping = make([]*googlev1.Mapping, len(symbols.Mappings))
	for _, loc := range profile.Location {
		if profile.Mapping[loc.MappingId] == nil {
			src := symbols.Mappings[loc.MappingId]
			profile.Mapping[loc.MappingId] = &googlev1.Mapping{
				Id:              loc.MappingId,
				MemoryStart:     src.MemoryStart,
				MemoryLimit:     src.MemoryLimit,
				FileOffset:      src.FileOffset,
				Filename:        int64(src.Filename),
				BuildId:         int64(src.BuildId),
				HasFunctions:    src.HasFunctions,
				HasFilenames:    src.HasFilenames,
				HasLineNumbers:  src.HasLineNumbers,
				HasInlineFrames: src.HasInlineFrames,
			}
		}
	}
	n := len(profile.Mapping)
	lut = slices.GrowLen(lut, n)
	var j int
	for i := 0; i < len(profile.Mapping); i++ {
		m := profile.Mapping[i]
		if m == nil {
			continue
		}
		oldId := m.Id
		m.Id = uint64(j) + 1
		lut[oldId] = uint32(m.Id)
		profile.Mapping[j] = m
		j++
	}
	profile.Mapping = profile.Mapping[:j]
	for _, loc := range profile.Location {
		loc.MappingId = uint64(lut[loc.MappingId])
	}
}

func copyStrings(profile *googlev1.Profile, symbols *Symbols, lut []uint32) {
	profile.StringTable = make([]string, len(symbols.Strings))
	for _, m := range profile.Mapping {
		profile.StringTable[m.Filename] = symbols.Strings[m.Filename]
		profile.StringTable[m.BuildId] = symbols.Strings[m.BuildId]
	}
	for _, f := range profile.Function {
		profile.StringTable[f.Name] = symbols.Strings[f.Name]
		profile.StringTable[f.Filename] = symbols.Strings[f.Filename]
		profile.StringTable[f.SystemName] = symbols.Strings[f.SystemName]
	}
	n := len(profile.StringTable)
	lut = slices.GrowLen(lut, n)
	var j int
	for i := 0; i < len(profile.StringTable); i++ {
		s := profile.StringTable[i]
		if s == "" && i > 0 {
			continue
		}
		lut[i] = uint32(j)
		profile.StringTable[j] = s
		j++
	}
	profile.StringTable = profile.StringTable[:j]
	for _, m := range profile.Mapping {
		m.Filename = int64(lut[m.Filename])
		m.BuildId = int64(lut[m.BuildId])
	}
	for _, f := range profile.Function {
		f.Name = int64(lut[f.Name])
		f.Filename = int64(lut[f.Filename])
		f.SystemName = int64(lut[f.SystemName])
	}
}
