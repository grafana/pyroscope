package pprof

import (
	"fmt"
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

// Merge adds p to the profile merge, cloning new objects.
// Profile p is modified in place but not retained by the function.
func (m *ProfileMerge) Merge(p *profilev1.Profile) error {
	return m.merge(p, true)
}

// MergeNoClone adds p to the profile merge, borrowing objects.
// Profile p is modified in place and retained by the function.
func (m *ProfileMerge) MergeNoClone(p *profilev1.Profile) error {
	return m.merge(p, false)
}

func (m *ProfileMerge) merge(p *profilev1.Profile, clone bool) error {
	if p == nil || len(p.StringTable) < 2 {
		return nil
	}
	ConvertIDsToIndices(p)
	var initial bool
	if m.profile == nil {
		m.init(p, clone)
		initial = true
	}

	// We rewrite strings first in order to compare
	// sample types and period type.
	m.tmp = slices.GrowLen(m.tmp, len(p.StringTable))
	m.stringTable.Index(m.tmp, p.StringTable)
	RewriteStrings(p, m.tmp)
	if initial {
		// Right after initialisation we need to make
		// sure that the string identifiers are normalized
		// among profiles.
		RewriteStrings(m.profile, m.tmp)
	}

	if err := combineHeaders(m.profile, p); err != nil {
		return err
	}

	m.tmp = slices.GrowLen(m.tmp, len(p.Function))
	m.functionTable.Index(m.tmp, p.Function)
	RewriteFunctions(p, m.tmp)

	m.tmp = slices.GrowLen(m.tmp, len(p.Mapping))
	m.mappingTable.Index(m.tmp, p.Mapping)
	RewriteMappings(p, m.tmp)

	m.tmp = slices.GrowLen(m.tmp, len(p.Location))
	m.locationTable.Index(m.tmp, p.Location)
	RewriteLocations(p, m.tmp)

	m.tmp = slices.GrowLen(m.tmp, len(p.Sample))
	m.sampleTable.Index(m.tmp, p.Sample)

	for i, idx := range m.tmp {
		dst := m.sampleTable.s[idx].Value
		src := p.Sample[i].Value
		for j, v := range src {
			dst[j] += v
		}
	}

	return nil
}

func (m *ProfileMerge) Profile() *profilev1.Profile {
	if m.profile == nil {
		return &profilev1.Profile{
			SampleType:  []*profilev1.ValueType{new(profilev1.ValueType)},
			PeriodType:  new(profilev1.ValueType),
			StringTable: []string{""},
		}
	}
	m.profile.Sample = m.sampleTable.Values()
	m.profile.Location = m.locationTable.Values()
	m.profile.Function = m.functionTable.Values()
	m.profile.Mapping = m.mappingTable.Values()
	m.profile.StringTable = m.stringTable.Values()
	for i := range m.profile.Location {
		m.profile.Location[i].Id = uint64(i + 1)
	}
	for i := range m.profile.Function {
		m.profile.Function[i].Id = uint64(i + 1)
	}
	for i := range m.profile.Mapping {
		m.profile.Mapping[i].Id = uint64(i + 1)
	}
	return m.profile
}

func (m *ProfileMerge) init(x *profilev1.Profile, clone bool) {
	factor := 2
	m.stringTable = NewRewriteTable(
		factor*len(x.StringTable),
		func(s string) string { return s },
		func(s string) string { return s },
	)

	if clone {
		m.functionTable = NewRewriteTable[FunctionKey, *profilev1.Function, *profilev1.Function](
			factor*len(x.Function), GetFunctionKey, cloneVT[*profilev1.Function])

		m.mappingTable = NewRewriteTable[MappingKey, *profilev1.Mapping, *profilev1.Mapping](
			factor*len(x.Mapping), GetMappingKey, cloneVT[*profilev1.Mapping])

		m.locationTable = NewRewriteTable[LocationKey, *profilev1.Location, *profilev1.Location](
			factor*len(x.Location), GetLocationKey, cloneVT[*profilev1.Location])

		m.sampleTable = NewRewriteTable[SampleKey, *profilev1.Sample, *profilev1.Sample](
			factor*len(x.Sample), GetSampleKey, func(sample *profilev1.Sample) *profilev1.Sample {
				c := sample.CloneVT()
				slices.Clear(c.Value)
				return c
			})
	} else {
		m.functionTable = NewRewriteTable[FunctionKey, *profilev1.Function, *profilev1.Function](
			factor*len(x.Function), GetFunctionKey, noClone[*profilev1.Function])

		m.mappingTable = NewRewriteTable[MappingKey, *profilev1.Mapping, *profilev1.Mapping](
			factor*len(x.Mapping), GetMappingKey, noClone[*profilev1.Mapping])

		m.locationTable = NewRewriteTable[LocationKey, *profilev1.Location, *profilev1.Location](
			factor*len(x.Location), GetLocationKey, noClone[*profilev1.Location])

		m.sampleTable = NewRewriteTable[SampleKey, *profilev1.Sample, *profilev1.Sample](
			factor*len(x.Sample), GetSampleKey, noClone[*profilev1.Sample])
	}

	m.profile = &profilev1.Profile{
		SampleType: make([]*profilev1.ValueType, len(x.SampleType)),
		DropFrames: x.DropFrames,
		KeepFrames: x.KeepFrames,
		TimeNanos:  x.TimeNanos,
		// Profile durations are summed up, therefore
		// we skip the field at initialization.
		// DurationNanos:  x.DurationNanos,
		PeriodType:        x.PeriodType.CloneVT(),
		Period:            x.Period,
		DefaultSampleType: x.DefaultSampleType,
	}
	for i, st := range x.SampleType {
		m.profile.SampleType[i] = st.CloneVT()
	}
}

func noClone[T any](t T) T { return t }

func cloneVT[T interface{ CloneVT() T }](t T) T { return t.CloneVT() }

// combineHeaders checks that all profiles can be merged and returns
// their combined profile.
// NOTE(kolesnikovae): Copied from pprof.
func combineHeaders(a, b *profilev1.Profile) error {
	if err := compatible(a, b); err != nil {
		return err
	}
	// Smallest timestamp.
	if a.TimeNanos == 0 || b.TimeNanos < a.TimeNanos {
		a.TimeNanos = b.TimeNanos
	}
	// Summed up duration.
	a.DurationNanos += b.DurationNanos
	// Largest period.
	if a.Period == 0 || a.Period < b.Period {
		a.Period = b.Period
	}
	if a.DefaultSampleType == 0 {
		a.DefaultSampleType = b.DefaultSampleType
	}
	return nil
}

// compatible determines if two profiles can be compared/merged.
// returns nil if the profiles are compatible; otherwise an error with
// details on the incompatibility.
func compatible(a, b *profilev1.Profile) error {
	if !equalValueType(a.PeriodType, b.PeriodType) {
		return fmt.Errorf("incompatible period types %v and %v", a.PeriodType, b.PeriodType)
	}
	if len(b.SampleType) != len(a.SampleType) {
		return fmt.Errorf("incompatible sample types %v and %v", a.SampleType, b.SampleType)
	}
	for i := range a.SampleType {
		if !equalValueType(a.SampleType[i], b.SampleType[i]) {
			return fmt.Errorf("incompatible sample types %v and %v", a.SampleType, b.SampleType)
		}
	}
	return nil
}

// equalValueType returns true if the two value types are semantically
// equal. It ignores the internal fields used during encode/decode.
func equalValueType(st1, st2 *profilev1.ValueType) bool {
	return st1.Type == st2.Type && st1.Unit == st2.Unit
}

func RewriteStrings(p *profilev1.Profile, n []uint32) {
	for _, t := range p.SampleType {
		t.Unit = int64(n[t.Unit])
		t.Type = int64(n[t.Type])
	}
	for _, s := range p.Sample {
		for _, l := range s.Label {
			l.Key = int64(n[l.Key])
			l.Str = int64(n[l.Str])
		}
	}
	for _, m := range p.Mapping {
		m.Filename = int64(n[m.Filename])
		m.BuildId = int64(n[m.BuildId])
	}
	for _, f := range p.Function {
		f.Name = int64(n[f.Name])
		f.Filename = int64(n[f.Filename])
		f.SystemName = int64(n[f.SystemName])
	}
	p.DropFrames = int64(n[p.DropFrames])
	p.KeepFrames = int64(n[p.KeepFrames])
	p.PeriodType.Type = int64(n[p.PeriodType.Type])
	p.PeriodType.Unit = int64(n[p.PeriodType.Unit])
	for i, x := range p.Comment {
		p.Comment[i] = int64(n[x])
	}
	p.DefaultSampleType = int64(n[p.DefaultSampleType])
}

func RewriteFunctions(p *profilev1.Profile, n []uint32) {
	for _, loc := range p.Location {
		for _, line := range loc.Line {
			line.FunctionId = uint64(n[line.FunctionId-1]) + 1
		}
	}
}

func RewriteMappings(p *profilev1.Profile, n []uint32) {
	for _, loc := range p.Location {
		loc.MappingId = uint64(n[loc.MappingId-1]) + 1
	}
}

func RewriteLocations(p *profilev1.Profile, n []uint32) {
	for _, s := range p.Sample {
		for i, loc := range s.LocationId {
			s.LocationId[i] = uint64(n[loc-1]) + 1
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
	size := m.MemoryLimit - m.MemoryStart
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
//  Probably we should use strings instead of hashes
//  to eliminate collisions.

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
		s: make([]M, 0, size),
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

func (t *RewriteTable[K, V, M]) Append(values []V) {
	for _, value := range values {
		k := t.k(value)
		n := uint32(len(t.s))
		t.s = append(t.s, t.v(value))
		t.t[k] = n
	}
}

func (t *RewriteTable[K, V, M]) Values() []M { return t.s }

func ConvertIDsToIndices(p *profilev1.Profile) {
	denseMappings := hasDenseMappings(p)
	denseLocations := hasDenseLocations(p)
	denseFunctions := hasDenseFunctions(p)
	if denseMappings && denseLocations && denseFunctions {
		// In most cases IDs are dense (do match the element index),
		// therefore the function does not change anything.
		return
	}
	// NOTE(kolesnikovae):
	// In some cases IDs is a non-monotonically increasing sequence,
	// therefore the same map can be reused to avoid re-allocations.
	t := make(map[uint64]uint64, len(p.Location))
	if !denseMappings {
		for i, x := range p.Mapping {
			idx := uint64(i + 1)
			x.Id, t[x.Id] = idx, idx
		}
		RewriteMappingsWithMap(p, t)
	}
	if !denseLocations {
		for i, x := range p.Location {
			idx := uint64(i + 1)
			x.Id, t[x.Id] = idx, idx
		}
		RewriteLocationsWithMap(p, t)
	}
	if !denseFunctions {
		for i, x := range p.Function {
			idx := uint64(i + 1)
			x.Id, t[x.Id] = idx, idx
		}
		RewriteFunctionsWithMap(p, t)
	}
}

func hasDenseFunctions(p *profilev1.Profile) bool {
	for i, f := range p.Function {
		if f.Id != uint64(i+1) {
			return false
		}
	}
	return true
}

func hasDenseLocations(p *profilev1.Profile) bool {
	for i, loc := range p.Location {
		if loc.Id != uint64(i+1) {
			return false
		}
	}
	return true
}

func hasDenseMappings(p *profilev1.Profile) bool {
	for i, m := range p.Mapping {
		if m.Id != uint64(i+1) {
			return false
		}
	}
	return true
}

func RewriteFunctionsWithMap(p *profilev1.Profile, n map[uint64]uint64) {
	for _, loc := range p.Location {
		for _, line := range loc.Line {
			line.FunctionId = n[line.FunctionId]
		}
	}
}

func RewriteMappingsWithMap(p *profilev1.Profile, n map[uint64]uint64) {
	for _, loc := range p.Location {
		loc.MappingId = n[loc.MappingId]
	}
}

func RewriteLocationsWithMap(p *profilev1.Profile, n map[uint64]uint64) {
	for _, s := range p.Sample {
		for i, loc := range s.LocationId {
			s.LocationId[i] = n[loc]
		}
	}
}
