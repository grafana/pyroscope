package symdb

import (
	"reflect"
	"unsafe"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/slices"
)

const (
	truncationMark    = 1 << 30
	truncatedNodeName = "other"
)

type pprofProtoTruncatedSymbols struct {
	symbols *Symbols
	samples *schemav1.Samples
	profile googlev1.Profile
	lut     []uint32
	cur     int

	maxNodes  int64
	truncated int

	functionTree *model.StacktraceTree
	stacktraces  []truncatedStacktraceSample
	// Two buffers are needed as we handle both function and location
	// stacks simultaneously.
	functionsBuf []int32
	locationsBuf []uint64

	selection *SelectedStackTraces
	fnNames   func(locations []int32) ([]int32, bool)

	// After truncation many samples will have the same stack trace.
	// The map is used to deduplicate them. The key is sample.LocationId
	// slice turned into a string: the underlying memory must not change.
	sampleMap map[string]*googlev1.Sample
	// As an optimisation, we merge all the stack trace samples that were
	// fully truncated to a single sample.
	fullyTruncated int64
}

type truncatedStacktraceSample struct {
	stacktraceID    uint32
	functionNodeIdx int32
	value           int64
}

func (r *pprofProtoTruncatedSymbols) init(symbols *Symbols, samples schemav1.Samples) {
	r.symbols = symbols
	r.samples = &samples
	// We optimistically assume that each stacktrace has only
	// 2 unique nodes. For pathological cases it may exceed 10.
	r.functionTree = model.NewStacktraceTree(samples.Len() * 2)
	r.stacktraces = make([]truncatedStacktraceSample, 0, samples.Len())
	r.sampleMap = make(map[string]*googlev1.Sample, samples.Len())
	if r.selection != nil && len(r.selection.callSite) > 0 {
		r.fnNames = r.locFunctionsFiltered
	} else {
		r.fnNames = r.locFunctions
	}
}

func (r *pprofProtoTruncatedSymbols) InsertStacktrace(stacktraceID uint32, locations []int32) {
	value := int64(r.samples.Values[r.cur])
	r.cur++
	functions, ok := r.fnNames(locations)
	if ok {
		functionNodeIdx := r.functionTree.Insert(functions, value)
		r.stacktraces = append(r.stacktraces, truncatedStacktraceSample{
			stacktraceID:    stacktraceID,
			functionNodeIdx: functionNodeIdx,
			value:           value,
		})
	}
}

func (r *pprofProtoTruncatedSymbols) locFunctions(locations []int32) ([]int32, bool) {
	r.functionsBuf = r.functionsBuf[:0]
	for i := 0; i < len(locations); i++ {
		lines := r.symbols.Locations[locations[i]].Line
		for j := 0; j < len(lines); j++ {
			r.functionsBuf = append(r.functionsBuf, int32(lines[j].FunctionId))
		}
	}
	return r.functionsBuf, true
}

func (r *pprofProtoTruncatedSymbols) locFunctionsFiltered(locations []int32) ([]int32, bool) {
	r.functionsBuf = r.functionsBuf[:0]
	var pos int
	pathLen := len(r.selection.callSite)
	// Even if len(locations) < pathLen, we still
	// need to inspect locations line by line.
	for i := len(locations) - 1; i >= 0; i-- {
		lines := r.symbols.Locations[locations[i]].Line
		for j := len(lines) - 1; j >= 0; j-- {
			f := lines[j].FunctionId
			if pos < pathLen {
				if r.selection.callSite[pos] != f {
					return nil, false
				}
				pos++
			}
			r.functionsBuf = append(r.functionsBuf, int32(f))
		}
	}
	if pos < pathLen {
		return nil, false
	}
	slices.Reverse(r.functionsBuf)
	return r.functionsBuf, true
}

func (r *pprofProtoTruncatedSymbols) buildPprof() *googlev1.Profile {
	r.markNodesForTruncation()
	for _, n := range r.stacktraces {
		r.addSample(n)
	}
	r.createSamples()
	createSampleTypeStub(&r.profile)
	copyLocations(&r.profile, r.symbols, r.lut)
	copyFunctions(&r.profile, r.symbols, r.lut)
	copyMappings(&r.profile, r.symbols, r.lut)
	copyStrings(&r.profile, r.symbols, r.lut)
	if r.truncated > 0 || r.fullyTruncated > 0 {
		createLocationStub(&r.profile)
	}
	return &r.profile
}

func (r *pprofProtoTruncatedSymbols) markNodesForTruncation() {
	minValue := r.functionTree.MinValue(r.maxNodes)
	if minValue == 0 {
		return
	}
	for i := range r.functionTree.Nodes {
		if r.functionTree.Nodes[i].Total < minValue {
			r.functionTree.Nodes[i].Location |= truncationMark
			r.truncated++
		}
	}
}

func (r *pprofProtoTruncatedSymbols) addSample(n truncatedStacktraceSample) {
	// Find the original stack trace and remove truncated
	// locations based on the truncated functions.
	var off int
	r.functionsBuf, off = r.buildFunctionsStack(r.functionsBuf, n.functionNodeIdx)
	if off < 0 {
		// The stack has no functions without the truncation mark.
		r.fullyTruncated += n.value
		return
	}
	r.locationsBuf = r.symbols.Stacktraces.LookupLocations(r.locationsBuf, n.stacktraceID)
	if off > 0 {
		// Some functions were truncated.
		r.locationsBuf = truncateLocations(r.locationsBuf, r.functionsBuf, off, r.symbols)
		// Otherwise, if the offset is zero, the stack can be taken as is.
	}
	// Truncation may result in vast duplication of stack traces.
	// Even if a particular stack trace is not truncated, we still
	// remember it, as there might be another truncated stack trace
	// that fully matches it.
	// Note that this is safe to take locationsBuf memory for the
	// map key lookup as it is not retained.
	if s, dup := r.sampleMap[uint64sliceString(r.locationsBuf)]; dup {
		s.Value[0] += n.value
		return
	}
	// If this is a new stack trace, copy locations, create
	// the sample, and add the stack trace to the map.
	locationsCopy := make([]uint64, len(r.locationsBuf))
	copy(locationsCopy, r.locationsBuf)
	s := &googlev1.Sample{LocationId: locationsCopy, Value: []int64{n.value}}
	r.profile.Sample = append(r.profile.Sample, s)
	r.sampleMap[uint64sliceString(locationsCopy)] = s
}

func (r *pprofProtoTruncatedSymbols) buildFunctionsStack(funcs []int32, idx int32) ([]int32, int) {
	offset := -1
	funcs = funcs[:0]
	for i := idx; i > 0; i = r.functionTree.Nodes[i].Parent {
		n := r.functionTree.Nodes[i]
		if offset < 0 && n.Location&truncationMark == 0 {
			// Remember the first node to keep.
			offset = len(funcs)
		}
		funcs = append(funcs, n.Location&^truncationMark)
	}
	return funcs, offset
}

func (r *pprofProtoTruncatedSymbols) createSamples() {
	samples := len(r.sampleMap)
	r.profile.Sample = make([]*googlev1.Sample, samples, samples+1)
	var i int
	for _, s := range r.sampleMap {
		r.profile.Sample[i] = s
		i++
	}
	if r.fullyTruncated > 0 {
		r.createStubSample()
	}
}

func truncateLocations(locations []uint64, functions []int32, offset int, symbols *Symbols) []uint64 {
	if offset < 1 {
		return locations
	}
	f := len(functions)
	l := len(locations)
	for ; l > 0 && f >= offset; l-- {
		location := symbols.Locations[locations[l-1]]
		for j := len(location.Line) - 1; j >= 0; j-- {
			f--
		}
	}
	if l > 0 {
		locations[0] = truncationMark
		return append(locations[:1], locations[l:]...)
	}
	return locations[l:]
}

func uint64sliceString(u []uint64) string {
	var s string
	if len(u) != 0 {
		hdr := (*reflect.SliceHeader)(unsafe.Pointer(&s))
		hdr.Data = uintptr(unsafe.Pointer(&u[0]))
		hdr.Len = len(u) * 8
	}
	return s
}

func (r *pprofProtoTruncatedSymbols) createStubSample() {
	r.profile.Sample = append(r.profile.Sample, &googlev1.Sample{
		LocationId: []uint64{truncationMark},
		Value:      []int64{r.fullyTruncated},
	})
}

func createLocationStub(profile *googlev1.Profile) {
	var stubNodeNameIdx int64
	for i, s := range profile.StringTable {
		if s == truncatedNodeName {
			stubNodeNameIdx = int64(i)
			break
		}
	}
	if stubNodeNameIdx == 0 {
		stubNodeNameIdx = int64(len(profile.StringTable))
		profile.StringTable = append(profile.StringTable, truncatedNodeName)
	}
	stubFn := &googlev1.Function{
		Id:         uint64(len(profile.Function) + 1),
		Name:       stubNodeNameIdx,
		SystemName: stubNodeNameIdx,
	}
	profile.Function = append(profile.Function, stubFn)
	stubLoc := &googlev1.Location{
		Id:        uint64(len(profile.Location) + 1),
		Line:      []*googlev1.Line{{FunctionId: stubFn.Id}},
		MappingId: 1,
	}
	profile.Location = append(profile.Location, stubLoc)
	for _, s := range profile.Sample {
		for i, loc := range s.LocationId {
			if loc == truncationMark {
				s.LocationId[i] = stubLoc.Id
			}
		}
	}
}
