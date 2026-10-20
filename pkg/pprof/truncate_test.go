package pprof

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
)

// builder assembles small pprof profiles with dense identifiers, the same
// shape that ProfileMerge produces.
type builder struct {
	strings     []string
	strIdx      map[string]int64
	sampleTypes []*profilev1.ValueType
	mappings    []*profilev1.Mapping
	functions   []*profilev1.Function
	locations   []*profilev1.Location
	samples     []*profilev1.Sample
}

func newBuilder() *builder {
	b := &builder{
		strings: []string{""},
		strIdx:  map[string]int64{"": 0},
	}
	b.mappings = []*profilev1.Mapping{{
		Id:           1,
		Filename:     b.str("app.bin"),
		BuildId:      b.str("build-1"),
		HasFunctions: true,
	}}
	b.sampleTypes = append(b.sampleTypes,
		&profilev1.ValueType{Type: b.str("samples"), Unit: b.str("count")})
	return b
}

// multiValue makes samples carry two value slots.
func (b *builder) multiValue() *builder {
	b.sampleTypes = append(b.sampleTypes,
		&profilev1.ValueType{Type: b.str("cpu"), Unit: b.str("nanoseconds")})
	return b
}

func (b *builder) str(s string) int64 {
	if i, ok := b.strIdx[s]; ok {
		return i
	}
	i := int64(len(b.strings))
	b.strings = append(b.strings, s)
	b.strIdx[s] = i
	return i
}

func (b *builder) function(name string) *profilev1.Function {
	f := &profilev1.Function{
		Id:         uint64(len(b.functions) + 1),
		Name:       b.str(name),
		SystemName: b.str(name),
	}
	b.functions = append(b.functions, f)
	return f
}

// location creates a location referencing the given functions in order;
// each function gets its own line, modeling non-inlined frames.
func (b *builder) location(fns ...*profilev1.Function) *profilev1.Location {
	l := &profilev1.Location{
		Id:        uint64(len(b.locations) + 1),
		Address:   uint64(0x4000 + len(b.locations)),
		MappingId: 1,
	}
	for _, f := range fns {
		l.Line = append(l.Line, &profilev1.Line{FunctionId: f.Id})
	}
	b.locations = append(b.locations, l)
	return l
}

func (b *builder) sample(value int64, labels map[string]string, locs ...*profilev1.Location) *profilev1.Sample {
	s := &profilev1.Sample{Value: []int64{value}}
	if len(b.sampleTypes) > 1 {
		s.Value = []int64{value, value * 2}
	}
	for _, l := range locs {
		s.LocationId = append(s.LocationId, l.Id)
	}
	for k, v := range labels {
		s.Label = append(s.Label, &profilev1.Label{
			Key: b.str(k),
			Str: b.str(v),
		})
	}
	b.samples = append(b.samples, s)
	return s
}

// stack appends a sample whose locations are created for the given
// functions, in leaf-first order.
func (b *builder) stack(value int64, fns ...*profilev1.Function) *profilev1.Sample {
	locs := make([]*profilev1.Location, len(fns))
	for i, f := range fns {
		locs[i] = b.location(f)
	}
	return b.sample(value, nil, locs...)
}

func (b *builder) profile() *profilev1.Profile {
	return &profilev1.Profile{
		SampleType:  b.sampleTypes,
		PeriodType:  &profilev1.ValueType{Type: b.str("cpu"), Unit: b.str("nanoseconds")},
		Sample:      b.samples,
		Location:    b.locations,
		Function:    b.functions,
		Mapping:     b.mappings,
		StringTable: b.strings,
	}
}

// total returns the sum of the first sample value slot.
func total(p *profilev1.Profile) int64 {
	var t int64
	for _, s := range p.Sample {
		if len(s.Value) > 0 {
			t += s.Value[0]
		}
	}
	return t
}

// functionNames returns the set of function names referenced by samples.
func functionNames(p *profilev1.Profile) map[string]bool {
	m := make(map[string]bool)
	for _, s := range p.Sample {
		for _, id := range s.LocationId {
			if int(id) < 1 || int(id) > len(p.Location) {
				continue
			}
			for _, ln := range p.Location[id-1].Line {
				if int(ln.FunctionId) < 1 || int(ln.FunctionId) > len(p.Function) {
					continue
				}
				m[p.StringTable[p.Function[ln.FunctionId-1].Name]] = true
			}
		}
	}
	return m
}

// stubSampleValue returns the sum of the first value slot of the samples
// that reference the synthetic "other" frame.
func stubSampleValue(p *profilev1.Profile) (int64, bool) {
	stubLoc := int64(-1)
	for i, l := range p.Location {
		if i > 0 && len(l.Line) == 1 {
			f := p.Function[l.Line[0].FunctionId-1]
			if f.Name < int64(len(p.StringTable)) && p.StringTable[f.Name] == "other" {
				stubLoc = int64(l.Id)
				break
			}
		}
	}
	if stubLoc < 0 {
		return 0, false
	}
	var v int64
	var found bool
	for _, s := range p.Sample {
		for _, id := range s.LocationId {
			if int64(id) == stubLoc {
				v += s.Value[0]
				found = true
				break
			}
		}
	}
	return v, found
}

// hasDenseIdentifiers verifies that every object of the profile is
// referenced and that the identifiers are dense, starting from 1.
func hasDenseIdentifiers(p *profilev1.Profile) bool {
	locUsed := make([]bool, len(p.Location))
	for _, s := range p.Sample {
		for _, id := range s.LocationId {
			if int(id) < 1 || int(id) > len(p.Location) {
				return false
			}
			locUsed[id-1] = true
		}
	}
	fnUsed := make([]bool, len(p.Function))
	for i, used := range locUsed {
		if !used {
			return false
		}
		for _, ln := range p.Location[i].Line {
			if int(ln.FunctionId) < 1 || int(ln.FunctionId) > len(p.Function) {
				return false
			}
			fnUsed[ln.FunctionId-1] = true
		}
	}
	for _, used := range fnUsed {
		if !used {
			return false
		}
	}
	for i, f := range p.Function {
		if int(f.Id) != i+1 {
			return false
		}
	}
	for i, l := range p.Location {
		if int(l.Id) != i+1 {
			return false
		}
	}
	return len(p.StringTable) == 0 || p.StringTable[0] == ""
}

// TestTruncateMergedPartitions pins the primary scenario of #5841: several
// partitions, each individually within the node budget, merge into a
// profile that exceeds it. The merged profile must be brought back under
// budget: the leaf-side frames of the cut branches are replaced by the
// synthetic "other" frame, values are conserved, and objects that become
// unreferenced are removed.
func TestTruncateMergedPartitions(t *testing.T) {
	const (
		partitions   = 3
		perPartition = 10
		maxNodes     = int64(12)
	)
	// Each partition carries 10 disjoint deep branches over a shared
	// trunk, with branch values 1..30 across the partitions, so that
	// the 12th largest node total is 28 and branches valued 1..27 are
	// cut while 28..30 survive.
	build := func() []*profilev1.Profile {
		var partials []*profilev1.Profile
		for p := 0; p < partitions; p++ {
			b := newBuilder()
			mid, root := b.function("mid"), b.function("root")
			for j := 0; j < perPartition; j++ {
				i := p*perPartition + j
				d := b.function(fmt.Sprintf("d%d", i))
				c := b.function(fmt.Sprintf("c%d", i))
				e := b.function(fmt.Sprintf("e%d", i))
				a := b.function(fmt.Sprintf("a%d", i))
				b.stack(int64(i+1), d, c, e, a, mid, root)
			}
			partials = append(partials, b.profile())
		}
		return partials
	}
	partials := build()
	m := new(ProfileMerge)
	for _, p := range partials {
		require.NoError(t, m.Merge(p, false))
	}
	merged := m.Profile()

	// The merged profile is far beyond the budget: 30 branches with
	// 4 frames each plus the trunk.
	require.Equal(t, int64(465), total(merged))
	require.Len(t, merged.Function, 122)
	require.Len(t, merged.Sample, 30)

	got := Truncate(merged, maxNodes)

	// No sample value is lost.
	assert.Equal(t, int64(465), total(got))
	// 27 cut branches + 3 branches kept in full.
	require.Len(t, got.Sample, 30)
	// The synthetic frame carries the sum of the cut leaf-side values.
	v, ok := stubSampleValue(got)
	require.True(t, ok)
	assert.Equal(t, int64(378), v) // 55 + 155 + 168.
	// The cut branches are shorter: the three deepest frames of every
	// cut branch are gone, one boundary frame is retained.
	depths := make(map[int]int)
	for _, s := range got.Sample {
		depths[len(s.LocationId)]++
	}
	assert.Equal(t, map[int]int{4: 27, 6: 3}, depths)
	// The d, c, and e frames of the cut branches are unreferenced now;
	// compaction must have removed them, along with their locations and
	// strings. 42 functions and 52 locations remain.
	// 42 functions remain; 60 locations: the 27 retained boundary
	// locations, the 12 locations of the branches kept in full, the
	// shared trunk locations of the ten stack slots, and the stub.
	require.Len(t, got.Function, 42)
	require.Len(t, got.Location, 60)
	assert.Contains(t, functionNames(got), "mid")
	assert.Contains(t, functionNames(got), "root")
	assert.Contains(t, functionNames(got), "a0") // boundary frame kept
	assert.NotContains(t, functionNames(got), "d0")
	assert.NotContains(t, functionNames(got), "c0")
	assert.NotContains(t, functionNames(got), "e0")
	assert.Contains(t, functionNames(got), "d29") // kept in full
	assert.True(t, hasDenseIdentifiers(got))
}

// TestTruncateFullyCutStacks verifies that stacks that fall below the
// threshold entirely are aggregated into a single synthetic sample.
func TestTruncateFullyCutStacks(t *testing.T) {
	b := newBuilder()
	w := b.function("w")
	b.stack(100, w)
	for i := 1; i <= 10; i++ {
		x := b.function(fmt.Sprintf("x%d", i))
		b.stack(int64(i), x)
	}
	p := b.profile()
	require.Equal(t, 11, len(p.Sample))

	got := Truncate(p, 1)

	require.Len(t, got.Sample, 2)
	// The value is conserved: the heavy stack kept in full plus the
	// aggregated remainder.
	assert.Equal(t, int64(155), total(got))
	v, ok := stubSampleValue(got)
	require.True(t, ok)
	assert.Equal(t, int64(55), v)
	assert.Len(t, got.Function, 2)
	assert.True(t, hasDenseIdentifiers(got))
}

// TestTruncateNoop verifies that a non-positive budget or a profile that
// fits the budget is returned unchanged.
func TestTruncateNoop(t *testing.T) {
	build := func() *profilev1.Profile {
		b := newBuilder()
		root := b.function("root")
		mid := b.function("mid")
		for i := 0; i < 4; i++ {
			leaf := b.function(fmt.Sprintf("leaf%d", i))
			b.stack(int64(i+1), leaf, mid, root)
		}
		return b.profile()
	}

	p := build()
	same := Truncate(p, 0)
	require.Same(t, p, same)
	require.Len(t, same.Sample, 4)

	p = build()
	same = Truncate(p, -1)
	require.Same(t, p, same)

	p = build()
	same = Truncate(p, 100)
	require.Same(t, p, same)
	require.Len(t, same.Sample, 4)

	// A nil or empty profile must not panic.
	assert.Nil(t, Truncate(nil, 1))
	empty := &profilev1.Profile{}
	require.Same(t, empty, Truncate(empty, 1))
}

// TestTruncateLabelsAndValues verifies that samples that share the stack
// and labels are aggregated, that samples with distinct labels are kept
// apart, that every value slot is carried over, and that labels survive.
func TestTruncateLabelsAndValues(t *testing.T) {
	b := newBuilder().multiValue()
	root := b.function("root")
	for i := 0; i < 6; i++ {
		leaf := b.function(fmt.Sprintf("leaf%d", i))
		loc := b.location(leaf, root)
		b.sample(1, map[string]string{"tenant": fmt.Sprintf("t%d", i)}, loc)
	}
	// Two samples on the same stack and labels aggregate; the two
	// value slots are summed independently.
	rootLeaf := b.location(b.function("shared"), root)
	b.sample(10, map[string]string{"tenant": "agg"}, rootLeaf)
	b.sample(20, map[string]string{"tenant": "agg"}, rootLeaf)

	p := b.profile()
	require.Len(t, p.Sample, 8)

	got := Truncate(p, 3)

	// All samples fit; nothing is cut, but the two identical samples
	// have been aggregated into one.
	require.Len(t, got.Sample, 7)
	assert.Equal(t, int64(36), total(got))
	byLabel := make(map[string][]int64)
	for _, s := range got.Sample {
		var key string
		for _, l := range s.Label {
			key = p.StringTable[l.Str]
		}
		byLabel[key] = s.Value
	}
	require.Contains(t, byLabel, "agg")
	assert.Equal(t, []int64{30, 60}, byLabel["agg"])
	assert.Contains(t, byLabel, "t0")
	assert.True(t, hasDenseIdentifiers(got))
}

// TestTruncatePreservesAddressesAndMappings verifies that the retained
// locations keep their address and mapping information, so that the
// result remains valid for symbolization, and that the addresses of the
// frames dropped by a cut are gone.
func TestTruncatePreservesAddressesAndMappings(t *testing.T) {
	b := newBuilder()
	root := b.function("root")
	// Two deep branches over a shared trunk and one heavy stack.
	mid := b.function("mid")
	deepA := []*profilev1.Function{b.function("dA"), b.function("cA"), b.function("bA")}
	deepB := []*profilev1.Function{b.function("dB"), b.function("cB"), b.function("bB")}
	b.stack(700, append(append([]*profilev1.Function{}, deepA...), mid, root)...)
	b.stack(500, append(append([]*profilev1.Function{}, deepB...), mid, root)...)
	keep := b.function("keep")
	b.stack(1000, keep, root)

	p := b.profile()
	// Record the address of every location before the enforcement
	// runs: it modifies the profile in place.
	addressOf := make(map[string]uint64)
	for _, f := range p.Function {
		for _, l := range p.Location {
			for _, ln := range l.Line {
				if ln.FunctionId == f.Id {
					addressOf[p.StringTable[f.Name]] = l.Address
				}
			}
		}
	}
	address := func(name string) (uint64, bool) {
		a, ok := addressOf[name]
		return a, ok
	}
	// maxNodes=6: the 6th largest node total is 700, so the frames of
	// the lighter branch are cut, while the heavier branch and the
	// heavy stack survive in full.
	got := Truncate(p, 6)

	present := make(map[uint64]bool)
	for _, l := range got.Location {
		present[l.Address] = true
	}
	for _, name := range []string{"mid", "root", "keep", "dA", "cA", "bA", "bB"} {
		a, ok := address(name)
		require.True(t, ok, "function %s", name)
		assert.Contains(t, present, a, "address of %s must be retained", name)
	}
	for _, name := range []string{"dB", "cB"} {
		a, ok := address(name)
		require.True(t, ok, "function %s", name)
		assert.NotContains(t, present, a, "address of %s must be dropped", name)
	}
	// The mapping survives with its strings intact.
	require.Len(t, got.Mapping, 1)
	assert.NotEmpty(t, got.StringTable[got.Mapping[0].Filename])
	assert.True(t, hasDenseIdentifiers(got))
}
