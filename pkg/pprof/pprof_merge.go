package pprof

import (
	"hash/maphash"
	"sort"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	"github.com/grafana/pyroscope/pkg/slices"
)

type ProfileMerge struct {
	profile *profilev1.Profile
	tmp     []uint32

	stringTable   RewriteTable[string, string, string]
	functionTable RewriteTable[FunctionKey, *profilev1.Function, *profilev1.Function]
	mappingTable  RewriteTable[MappingKey, *profilev1.Mapping, *profilev1.Mapping]
	locationTable RewriteTable[LocationKey, *profilev1.Location, *profilev1.Location]
	sampleTable   RewriteTable[SampleKey, *profilev1.Sample, *profilev1.Sample]
}

func NewProfileMerge() *ProfileMerge { return new(ProfileMerge) }

func (m *ProfileMerge) init(x *profilev1.Profile) {
	factor := 2
	m.stringTable = NewRewriteTable(
		factor*len(x.StringTable),
		func(s string) string { return s },
		func(s string) string { return s },
	)

	m.functionTable = NewRewriteTable[FunctionKey, *profilev1.Function, *profilev1.Function](
		factor*len(x.Function), GetFunctionKey, cloneVT[*profilev1.Function])

	m.mappingTable = NewRewriteTable[MappingKey, *profilev1.Mapping, *profilev1.Mapping](
		factor*len(x.Mapping), GetMappingKey, cloneVT[*profilev1.Mapping])

	m.locationTable = NewRewriteTable[LocationKey, *profilev1.Location, *profilev1.Location](
		factor*len(x.Location), GetLocationKey, cloneVT[*profilev1.Location])

	m.sampleTable = NewRewriteTable[SampleKey, *profilev1.Sample, *profilev1.Sample](
		factor*len(x.Sample), GetSampleKey, cloneVT[*profilev1.Sample])

	m.profile = new(profilev1.Profile)
	// TODO: Set headers
}

func cloneVT[T interface{ CloneVT() T }](t T) T { return t.CloneVT() }

func (m *ProfileMerge) Merge(p *profilev1.Profile) {
	if m.profile == nil {
		m.init(p)
	} else {
		// TODO:
		//  - validate compatibility.
		//  - combine headers.
	}

	slices.GrowLen(m.tmp, len(p.StringTable))
	m.stringTable.Index(m.tmp, p.StringTable)
	RewriteStrings(p, m.tmp)

	slices.GrowLen(m.tmp, len(p.Function))
	m.functionTable.Index(m.tmp, p.Function)
	RewriteFuncs(p, m.tmp)

	slices.GrowLen(m.tmp, len(p.Mapping))
	m.mappingTable.Index(m.tmp, p.Mapping)
	RewriteMappings(p, m.tmp)

	slices.GrowLen(m.tmp, len(p.Location))
	m.locationTable.Index(m.tmp, p.Location)
	RewriteLocations(p, m.tmp)

	slices.GrowLen(m.tmp, len(p.Sample))
	m.sampleTable.Index(m.tmp, p.Sample)

	for i, idx := range m.tmp {
		values := m.sampleTable.s[idx].Value
		for j, v := range p.Sample[i].Value {
			values[j] += v
		}
	}
}

func RewriteStrings(p *profilev1.Profile, n []uint32) {
	for _, t := range p.SampleType {
		t.Unit = int64(n[t.Unit])
		t.Type = int64(n[t.Type])
	}
	p.PeriodType.Type = int64(n[p.PeriodType.Type])
	p.PeriodType.Unit = int64(n[p.PeriodType.Unit])
	for _, s := range p.Sample {
		for _, l := range s.Label {
			l.Key = int64(n[l.Key])
			l.Str = int64(n[l.Str])
		}
	}
	for _, f := range p.Function {
		f.Name = int64(n[f.Name])
		f.Filename = int64(n[f.Filename])
		f.SystemName = int64(n[f.SystemName])
	}
	for _, m := range p.Mapping {
		m.Filename = int64(n[m.Filename])
		m.BuildId = int64(n[m.BuildId])
	}
	for i, x := range p.Comment {
		p.Comment[i] = int64(n[x])
	}
	p.DropFrames = int64(n[p.DropFrames])
	p.KeepFrames = int64(n[p.DropFrames])
}

func RewriteFuncs(p *profilev1.Profile, n []uint32) {
	for _, loc := range p.Location {
		for _, l := range loc.Line {
			l.FunctionId = uint64(n[l.FunctionId])
		}
	}
}

func RewriteMappings(p *profilev1.Profile, n []uint32) {
	for _, loc := range p.Location {
		loc.MappingId = uint64(n[loc.MappingId])
	}
}

func RewriteLocations(p *profilev1.Profile, n []uint32) {
	for _, s := range p.Sample {
		for i, loc := range s.LocationId {
			s.LocationId[i] = uint64(n[loc])
		}
	}
}

type FunctionKey struct {
	startLine  uint32
	name       uint32
	systemName uint32
	fileName   uint32
}

func GetFunctionKey(fn *profilev1.Function) FunctionKey {
	return FunctionKey{
		startLine:  uint32(fn.StartLine),
		name:       uint32(fn.Name),
		systemName: uint32(fn.SystemName),
		fileName:   uint32(fn.Filename),
	}
}

type MappingKey struct {
	size          uint64
	offset        uint64
	buildIDOrFile int64
}

func GetMappingKey(m *profilev1.Mapping) MappingKey {
	// NOTE(kolesnikovae): Copied from pprof.
	// Normalize addresses to handle address space randomization.
	// Round up to next 4K boundary to avoid minor discrepancies.
	const mapsizeRounding = 0x1000
	size := m.MemoryLimit - m.MemoryLimit
	size = size + mapsizeRounding - 1
	size = size - (size % mapsizeRounding)
	k := MappingKey{
		size:   size,
		offset: m.FileOffset,
	}
	switch {
	case m.BuildId != 0:
		k.buildIDOrFile = m.BuildId
	case m.Filename != 0:
		k.buildIDOrFile = m.Filename
	default:
		// A mapping containing neither build ID nor file name is a fake mapping. A
		// key with empty buildIDOrFile is used for fake mappings so that they are
		// treated as the same mapping during merging.
	}
	return k
}

type LocationKey struct {
	addr      uint64
	lines     uint64
	mappingID uint64
}

func GetLocationKey(loc *profilev1.Location) LocationKey {
	return LocationKey{
		addr:      loc.Address,
		mappingID: loc.MappingId,
		lines:     hashLines(loc.Line),
	}
}

type SampleKey struct {
	locations uint64
	labels    uint64
}

func GetSampleKey(s *profilev1.Sample) SampleKey {
	return SampleKey{
		locations: hashLocations(s.LocationId),
		labels:    hashLabels(s.Label),
	}
}

var mapHashSeed = maphash.MakeSeed()

// NOTE(kolesnikovae):
//  Probably we should use strings instead of hashes.

func hashLocations(s []uint64) uint64 {
	return maphash.Bytes(mapHashSeed, uint64Bytes(s))
}

func hashLines(s []*profilev1.Line) uint64 {
	x := make([]uint64, len(s))
	for i, l := range s {
		x[i] = l.FunctionId | uint64(l.Line)<<32
	}
	return maphash.Bytes(mapHashSeed, uint64Bytes(x))
}

func hashLabels(s []*profilev1.Label) uint64 {
	if len(s) == 0 {
		return 0
	}
	sort.Sort(LabelsByKeyValue(s))
	x := make([]uint64, len(s))
	for i, l := range s {
		// Num and Unit ignored.
		x[i] = uint64(l.Key | l.Str<<32)
	}
	return maphash.Bytes(mapHashSeed, uint64Bytes(x))
}

// RewriteTable maintains unique values V and their indices.
// V is never modified nor retained, K and M are kept in memory.
type RewriteTable[K comparable, V, M any] struct {
	k func(V) K
	v func(V) M
	t map[K]uint32
	s []M
}

func NewRewriteTable[K comparable, V, M any](
	size int,
	k func(V) K,
	v func(V) M,
) RewriteTable[K, V, M] {
	return RewriteTable[K, V, M]{
		k: k,
		v: v,
		t: make(map[K]uint32, size),
		s: make([]M, size),
	}
}

func (t *RewriteTable[K, V, M]) Index(dst []uint32, values []V) {
	for i, value := range values {
		k := t.k(value)
		n, found := t.t[k]
		if !found {
			n = uint32(len(t.s))
			t.s = append(t.s, t.v(value))
			t.t[k] = n
		}
		dst[i] = n
	}
}

func (t *RewriteTable[K, V, M]) Values() []M { return t.s }
