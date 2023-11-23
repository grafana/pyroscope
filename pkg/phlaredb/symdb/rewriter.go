package symdb

import (
	"context"
	"math"
	"sort"

	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/slices"
)

type Rewriter struct {
	symdb      *SymDB
	source     SymbolsReader
	partitions map[uint64]*partitionRewriter
}

func NewRewriter(w *SymDB, r SymbolsReader) *Rewriter {
	return &Rewriter{
		source: r,
		symdb:  w,
	}
}

func (r *Rewriter) Rewrite(partition uint64, stacktraces []uint32) error {
	p, err := r.init(partition)
	if err != nil {
		return err
	}
	if err = p.populateUnresolved(stacktraces); err != nil {
		return err
	}
	if p.hasUnresolved() {
		return p.appendRewrite(stacktraces)
	}
	return nil
}

func (r *Rewriter) init(partition uint64) (p *partitionRewriter, err error) {
	if r.partitions == nil {
		r.partitions = make(map[uint64]*partitionRewriter)
	}
	if p, err = r.getOrCreatePartition(partition); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *Rewriter) getOrCreatePartition(partition uint64) (_ *partitionRewriter, err error) {
	p, ok := r.partitions[partition]
	if ok {
		p.reset()
		return p, nil
	}

	n := &partitionRewriter{name: partition}
	n.dst = r.symdb.PartitionWriter(partition)
	// Note that the partition is not released: we want to keep
	// it during the whole lifetime of the rewriter.
	pr, err := r.source.Partition(context.TODO(), partition)
	if err != nil {
		return nil, err
	}
	// We clone locations, functions, and mappings,
	// because these object will be modified.
	n.src = cloneSymbolsPartially(pr.Symbols())
	var stats PartitionStats
	pr.WriteStats(&stats)

	n.stacktraces = newLookupTable[[]int32](stats.MaxStacktraceID)
	n.locations = newLookupTable[*schemav1.InMemoryLocation](stats.LocationsTotal)
	n.mappings = newLookupTable[*schemav1.InMemoryMapping](stats.MappingsTotal)
	n.functions = newLookupTable[*schemav1.InMemoryFunction](stats.FunctionsTotal)
	n.strings = newLookupTable[string](stats.StringsTotal)

	r.partitions[partition] = n
	return n, nil
}

type partitionRewriter struct {
	name uint64

	src *Symbols
	dst *PartitionWriter

	stacktraces *lookupTable[[]int32]
	locations   *lookupTable[*schemav1.InMemoryLocation]
	mappings    *lookupTable[*schemav1.InMemoryMapping]
	functions   *lookupTable[*schemav1.InMemoryFunction]
	strings     *lookupTable[string]

	// FIXME(kolesnikovae): schemav1.Stacktrace should be just a uint32 slice:
	//   type Stacktrace []uint32
	current []*schemav1.Stacktrace
}

func (p *partitionRewriter) reset() {
	p.stacktraces.reset()
	p.locations.reset()
	p.mappings.reset()
	p.functions.reset()
	p.strings.reset()
	p.current = p.current[:0]
}

func (p *partitionRewriter) hasUnresolved() bool {
	return len(p.stacktraces.unresolved)+
		len(p.locations.unresolved)+
		len(p.mappings.unresolved)+
		len(p.functions.unresolved)+
		len(p.strings.unresolved) > 0
}

func (p *partitionRewriter) populateUnresolved(stacktraceIDs []uint32) error {
	// Filter out all stack traces that have been already
	// resolved and populate locations lookup table.
	if err := p.resolveStacktraces(stacktraceIDs); err != nil {
		return err
	}
	if len(p.locations.unresolved) == 0 {
		return nil
	}

	// Resolve functions and mappings for new locations.
	unresolvedLocs := p.locations.iter()
	for unresolvedLocs.Next() {
		location := p.src.Locations[unresolvedLocs.At()]
		location.MappingId = p.mappings.tryLookup(location.MappingId)
		for j, line := range location.Line {
			location.Line[j].FunctionId = p.functions.tryLookup(line.FunctionId)
		}
		unresolvedLocs.setValue(location)
	}

	// Resolve strings.
	unresolvedMappings := p.mappings.iter()
	for unresolvedMappings.Next() {
		mapping := p.src.Mappings[unresolvedMappings.At()]
		mapping.BuildId = p.strings.tryLookup(mapping.BuildId)
		mapping.Filename = p.strings.tryLookup(mapping.Filename)
		unresolvedMappings.setValue(mapping)
	}

	unresolvedFunctions := p.functions.iter()
	for unresolvedFunctions.Next() {
		function := p.src.Functions[unresolvedFunctions.At()]
		function.Name = p.strings.tryLookup(function.Name)
		function.Filename = p.strings.tryLookup(function.Filename)
		function.SystemName = p.strings.tryLookup(function.SystemName)
		unresolvedFunctions.setValue(function)
	}

	unresolvedStrings := p.strings.iter()
	for unresolvedStrings.Next() {
		unresolvedStrings.setValue(p.src.Strings[unresolvedStrings.At()])
	}

	return nil
}

func (p *partitionRewriter) appendRewrite(stacktraces []uint32) error {
	p.dst.AppendStrings(p.strings.buf, p.strings.values)
	p.strings.updateResolved()

	for _, v := range p.functions.values {
		v.Name = p.strings.lookupResolved(v.Name)
		v.Filename = p.strings.lookupResolved(v.Filename)
		v.SystemName = p.strings.lookupResolved(v.SystemName)
	}
	p.dst.AppendFunctions(p.functions.buf, p.functions.values)
	p.functions.updateResolved()

	for _, v := range p.mappings.values {
		v.BuildId = p.strings.lookupResolved(v.BuildId)
		v.Filename = p.strings.lookupResolved(v.Filename)
	}
	p.dst.AppendMappings(p.mappings.buf, p.mappings.values)
	p.mappings.updateResolved()

	for _, v := range p.locations.values {
		v.MappingId = p.mappings.lookupResolved(v.MappingId)
		for j, line := range v.Line {
			v.Line[j].FunctionId = p.functions.lookupResolved(line.FunctionId)
		}
	}
	p.dst.AppendLocations(p.locations.buf, p.locations.values)
	p.locations.updateResolved()

	for _, v := range p.stacktraces.values {
		for j, location := range v {
			v[j] = int32(p.locations.lookupResolved(uint32(location)))
		}
	}
	p.dst.AppendStacktraces(p.stacktraces.buf, p.stacktracesFromResolvedValues())
	p.stacktraces.updateResolved()

	for i, v := range stacktraces {
		stacktraces[i] = p.stacktraces.lookupResolved(v)
	}

	return nil
}

func (p *partitionRewriter) resolveStacktraces(stacktraceIDs []uint32) error {
	for i, v := range stacktraceIDs {
		stacktraceIDs[i] = p.stacktraces.tryLookup(v)
	}
	if len(p.stacktraces.unresolved) == 0 {
		return nil
	}
	p.stacktraces.initSorted()
	return p.src.Stacktraces.ResolveStacktraceLocations(
		context.Background(), p, p.stacktraces.buf)
}

func (p *partitionRewriter) stacktracesFromResolvedValues() []*schemav1.Stacktrace {
	p.current = slices.GrowLen(p.current, len(p.stacktraces.values))
	for i, v := range p.stacktraces.values {
		s := p.current[i]
		if s == nil {
			s = &schemav1.Stacktrace{LocationIDs: make([]uint64, len(v))}
			p.current[i] = s
		}
		s.LocationIDs = slices.GrowLen(s.LocationIDs, len(v))
		for j, m := range v {
			s.LocationIDs[j] = uint64(m)
		}
	}
	return p.current
}

func (p *partitionRewriter) InsertStacktrace(stacktrace uint32, locations []int32) {
	// Resolve locations for new stack traces.
	for j, loc := range locations {
		locations[j] = int32(p.locations.tryLookup(uint32(loc)))
	}
	// stacktrace points to resolved which should
	// be a marked pointer to unresolved value.
	idx := p.stacktraces.resolved[stacktrace] & markerMask
	v := &p.stacktraces.values[idx]
	n := slices.GrowLen(*v, len(locations))
	copy(n, locations)
	// Preserve allocated capacity.
	p.stacktraces.values[idx] = n
}

func cloneSymbolsPartially(x *Symbols) *Symbols {
	n := Symbols{
		Stacktraces: x.Stacktraces,
		Locations:   make([]*schemav1.InMemoryLocation, len(x.Locations)),
		Mappings:    make([]*schemav1.InMemoryMapping, len(x.Mappings)),
		Functions:   make([]*schemav1.InMemoryFunction, len(x.Functions)),
		Strings:     x.Strings,
	}
	for i, l := range x.Locations {
		n.Locations[i] = l.Clone()
	}
	for i, m := range x.Mappings {
		n.Mappings[i] = m.Clone()
	}
	for i, f := range x.Functions {
		n.Functions[i] = f.Clone()
	}
	return &n
}

const (
	marker     = 1 << 31
	markerMask = math.MaxUint32 >> 1
)

type lookupTable[T any] struct {
	// Index is source ID, and the value is the destination ID.
	// If destination ID is not known, the element is index to 'unresolved' (marked).
	resolved   []uint32
	unresolved []uint32 // Points to resolved. Index matches values.
	values     []T      // Values are populated for unresolved items.
	buf        []uint32 // Sorted unresolved values.
}

func newLookupTable[T any](size int) *lookupTable[T] {
	var t lookupTable[T]
	t.grow(size)
	return &t
}

func (t *lookupTable[T]) grow(size int) {
	if cap(t.resolved) < size {
		t.resolved = make([]uint32, size)
		return
	}
	t.resolved = t.resolved[:size]
	for i := range t.resolved {
		t.resolved[i] = 0
	}
}

func (t *lookupTable[T]) reset() {
	t.unresolved = t.unresolved[:0]
	t.values = t.values[:0]
	t.buf = t.buf[:0]
}

// tryLookup looks up the value at x in resolved.
// If x is has not been resolved yet, the x is memorized
// for future resolve, and returned values is the marked
// index to unresolved.
func (t *lookupTable[T]) tryLookup(x uint32) uint32 {
	// todo(ctovena): this is a hack to make sure we don't have any out of bounds errors
	// see https://github.com/grafana/pyroscope/issues/2488
	if x >= uint32(len(t.resolved)) {
		t.grow(int(x + 1))
	}
	if v := t.resolved[x]; v != 0 {
		if v&marker > 0 {
			return v // Already marked for resolve.
		}
		return v - 1 // Already resolved.
	}
	u := t.newUnresolved(x) | marker
	t.resolved[x] = u
	return u
}

func (t *lookupTable[T]) newUnresolved(rid uint32) uint32 {
	t.unresolved = append(t.unresolved, rid)
	x := len(t.values)
	if x < cap(t.values) {
		// Try to reuse previously allocated value.
		t.values = t.values[:x+1]
	} else {
		var v T
		t.values = append(t.values, v)
	}
	return uint32(x)
}

func (t *lookupTable[T]) storeResolved(i int, rid uint32) {
	// The index is incremented to avoid 0 because it is
	// used as sentinel and indicates absence (resolved is
	// a sparse slice initialized with the maximal expected
	// size). Correspondingly, lookupResolved should
	// decrement the index on read.
	t.resolved[t.unresolved[i]] = rid + 1
}

func (t *lookupTable[T]) lookupResolved(x uint32) uint32 {
	if x&marker > 0 {
		return t.resolved[t.unresolved[x&markerMask]] - 1
	}
	return x // Already resolved.
}

// updateResolved loads indices from buf to resolved.
// It is expected that the order matches values.
func (t *lookupTable[T]) updateResolved() {
	for i, rid := range t.unresolved {
		t.resolved[rid] = t.buf[i] + 1
	}
}

func (t *lookupTable[T]) initSorted() {
	// Gather and sort references to unresolved values.
	t.buf = slices.GrowLen(t.buf, len(t.unresolved))
	copy(t.buf, t.unresolved)
	sort.Slice(t.buf, func(i, j int) bool {
		return t.buf[i] < t.buf[j]
	})
}

func (t *lookupTable[T]) iter() *lookupTableIterator[T] {
	t.initSorted()
	return &lookupTableIterator[T]{table: t}
}

type lookupTableIterator[T any] struct {
	table *lookupTable[T]
	cur   uint32
}

func (t *lookupTableIterator[T]) Next() bool {
	return t.cur < uint32(len(t.table.buf))
}

func (t *lookupTableIterator[T]) At() uint32 {
	x := t.table.buf[t.cur]
	t.cur++
	return x
}

func (t *lookupTableIterator[T]) setValue(v T) {
	u := t.table.resolved[t.table.buf[t.cur-1]]
	t.table.values[u&markerMask] = v
}

func (t *lookupTableIterator[T]) Close() error { return nil }

func (t *lookupTableIterator[T]) Err() error { return nil }
