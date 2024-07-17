package symdb

import (
	"context"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/slices"
)

type pprofBuilder interface {
	StacktraceInserter
	init(*Symbols, schemav1.Samples)
	buildPprof() *googlev1.Profile
}

func buildPprof(
	ctx context.Context,
	symbols *Symbols,
	samples schemav1.Samples,
	maxNodes int64,
	selection *SelectedStackTraces,
) (*googlev1.Profile, error) {
	// By default, we use a builder that's optimized for the most
	// basic case: we take all the source stack traces unchanged.
	var b pprofBuilder = new(pprofFull)
	switch {
	// Go PGO selector; the stack traces are trimmed in the way
	// that only the first KeepLocations are retained. Optionally,
	// samples are aggregated by the callee, ignoring the leaf
	// location line number.
	case selection.gopgo != nil:
		b = &pprofGoPGO{pgo: selection.gopgo}
	// If a stack trace selector is specified, check if such a
	// profile can exist. Otherwise, build an empty profile.
	case !selection.HasValidCallSite():
		return b.buildPprof(), nil
	// Truncation is applicable when there is an explicit
	// limit on the number of the nodes in the profile, or
	// if stack traces should be filtered by the call site.
	case maxNodes > 0 || len(selection.callSite) > 0:
		b = &pprofTree{maxNodes: maxNodes, selection: selection}
	}
	b.init(symbols, samples)
	if err := symbols.Stacktraces.ResolveStacktraceLocations(ctx, b, samples.StacktraceIDs); err != nil {
		return nil, err
	}
	return b.buildPprof(), nil
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
	// symbols.Strings may not contain empty strings as it is
	// required by the pprof format. Therefore, we create one
	// at index 0 to ensure correctness.
	z := -1
	for i := 0; i < len(symbols.Strings); i++ {
		s := symbols.Strings[i]
		if s == "" {
			z = i
			break
		}
	}
	// o is the offset to apply to the string table:
	// it's 0 if the empty string is present, 1 otherwise.
	var o int64
	if z < 0 {
		// There is no empty string. We need to allocate one.
		// Otherwise, if "" is at any place other than 0, we
		// only need to swap the strings after we gather them.
		o = 1
	}
	profile.StringTable = make([]string, len(symbols.Strings)+int(o))
	// Gather strings referenced by the profile: profile.StringTable
	// is a sparse array with empty slots, that will be removed later.
	for _, m := range profile.Mapping {
		profile.StringTable[m.Filename+o] = symbols.Strings[m.Filename]
		profile.StringTable[m.BuildId+o] = symbols.Strings[m.BuildId]
	}
	for _, f := range profile.Function {
		profile.StringTable[f.Name+o] = symbols.Strings[f.Name]
		profile.StringTable[f.Filename+o] = symbols.Strings[f.Filename]
		profile.StringTable[f.SystemName+o] = symbols.Strings[f.SystemName]
	}
	// Swap zero string, if needed.
	if z > 0 {
		profile.StringTable[z], profile.StringTable[0] = profile.StringTable[0], profile.StringTable[z]
	}
	n := len(profile.StringTable)
	lut = slices.GrowLen(lut, n)
	j := 1 // Skip "" as its index is deterministic.
	for i := 1; i < len(profile.StringTable); i++ {
		s := profile.StringTable[i]
		if s == "" {
			continue
		}
		x := i
		if i == z {
			// Move item at the "" index to 0.
			x = 0
		}
		lut[x] = uint32(j)
		profile.StringTable[j] = s
		j++
	}
	// Rewrite string references in the profile.
	profile.StringTable = profile.StringTable[:j]
	for _, m := range profile.Mapping {
		m.Filename = int64(lut[m.Filename+o])
		m.BuildId = int64(lut[m.BuildId+o])
	}
	for _, f := range profile.Function {
		f.Name = int64(lut[f.Name+o])
		f.Filename = int64(lut[f.Filename+o])
		f.SystemName = int64(lut[f.SystemName+o])
	}
}
