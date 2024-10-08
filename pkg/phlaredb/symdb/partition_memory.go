package symdb

import (
	"context"
	"io"
	"sync"

	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type PartitionWriter struct {
	header PartitionHeader

	stacktraces *stacktraces
	strings     deduplicatingSlice[string, string, *stringsHelper]
	mappings    deduplicatingSlice[schemav1.InMemoryMapping, mappingsKey, *mappingsHelper]
	functions   deduplicatingSlice[schemav1.InMemoryFunction, functionsKey, *functionsHelper]
	locations   deduplicatingSlice[schemav1.InMemoryLocation, locationsKey, *locationsHelper]
}

func (p *PartitionWriter) AppendStacktraces(dst []uint32, s []*schemav1.Stacktrace) {
	p.stacktraces.append(dst, s)
}

func (p *PartitionWriter) ResolveStacktraceLocations(_ context.Context, dst StacktraceInserter, stacktraces []uint32) error {
	if len(stacktraces) == 0 {
		return nil
	}
	return p.stacktraces.resolve(dst, stacktraces)
}

func (p *PartitionWriter) LookupLocations(dst []uint64, stacktraceID uint32) []uint64 {
	dst = dst[:0]
	if stacktraceID == 0 {
		return dst
	}
	return p.stacktraces.tree.resolveUint64(dst, stacktraceID)
}

func newStacktraces() *stacktraces {
	p := &stacktraces{
		hashToIdx: make(map[uint64]uint32),
		tree:      newStacktraceTree(defaultStacktraceTreeSize),
	}
	return p
}

type stacktraces struct {
	m         sync.RWMutex
	hashToIdx map[uint64]uint32
	tree      *stacktraceTree
	stacks    uint32
}

func (p *stacktraces) size() uint64 {
	p.m.RLock()
	// TODO: map footprint isn't accounted
	v := stacktraceTreeNodeSize * cap(p.tree.nodes)
	p.m.RUnlock()
	return uint64(v)
}

func (p *stacktraces) append(dst []uint32, s []*schemav1.Stacktrace) {
	if len(s) == 0 {
		return
	}

	var (
		id     uint32
		found  bool
		misses int
	)

	p.m.RLock()
	for i, x := range s {
		if dst[i], found = p.hashToIdx[hashLocations(x.LocationIDs)]; !found {
			misses++
		}
	}

	p.m.RUnlock()
	if misses == 0 {
		return
	}

	// NOTE(kolesnikovae):
	//
	// Maybe we don't need this map at all: tree insertion might be
	// done in a thread safe fashion, and optimised to the extent
	// that its performance is comparable with:
	//   map_read + r_(un)lock + map_overhead +
	//   miss_rate * (map_write + w_(un)lock)
	//
	// Instead of inserting stacks one by one, it is better to
	// build a tree, and merge it to the existing one.

	p.m.Lock()
	defer p.m.Unlock()
	for i, v := range dst[:len(s)] {
		if v != 0 {
			// Already resolved. ID 0 is reserved
			// as it is the tree root.
			continue
		}
		x := s[i].LocationIDs
		// Tree insertion is idempotent,
		// we don't need to check the map.
		id = p.tree.insert(x)
		h := hashLocations(x)
		p.hashToIdx[h] = id
		dst[i] = id
	}
}

const defaultStacktraceDepth = 64

var stacktraceLocations = stacktraceLocationsPool{
	Pool: sync.Pool{New: func() any { return make([]int32, 0, defaultStacktraceDepth) }},
}

type stacktraceLocationsPool struct{ sync.Pool }

func (p *stacktraceLocationsPool) get() []int32 {
	return stacktraceLocations.Get().([]int32)
}

func (p *stacktraceLocationsPool) put(x []int32) {
	stacktraceLocations.Put(x)
}

func (p *stacktraces) resolve(dst StacktraceInserter, stacktraces []uint32) (err error) {
	p.m.RLock()
	t := stacktraceTree{nodes: p.tree.nodes}
	// tree.resolve is thread safe: only the parent node index (p)
	// and the reference to location (r) node fields are accessed,
	// which are never modified after insertion.
	//
	// Nevertheless, the node slice header should be copied to avoid
	// races when the slice grows: in the worst case, the underlying
	// capacity will be retained and thus not be eligible for GC during
	// the call.
	p.m.RUnlock()
	s := stacktraceLocations.get()
	for _, sid := range stacktraces {
		s = t.resolve(s, sid)
		dst.InsertStacktrace(sid, s)
	}
	stacktraceLocations.put(s)
	return nil
}

func (p *stacktraces) WriteTo(dst io.Writer) (int64, error) {
	return p.tree.WriteTo(dst)
}

func (p *PartitionWriter) AppendLocations(dst []uint32, locations []schemav1.InMemoryLocation) {
	p.locations.append(dst, locations)
}

func (p *PartitionWriter) AppendMappings(dst []uint32, mappings []schemav1.InMemoryMapping) {
	p.mappings.append(dst, mappings)
}

func (p *PartitionWriter) AppendFunctions(dst []uint32, functions []schemav1.InMemoryFunction) {
	p.functions.append(dst, functions)
}

func (p *PartitionWriter) AppendStrings(dst []uint32, strings []string) {
	p.strings.append(dst, strings)
}

func (p *PartitionWriter) Symbols() *Symbols {
	return &Symbols{
		Stacktraces: p,
		Locations:   p.locations.sliceHeaderCopy(),
		Mappings:    p.mappings.sliceHeaderCopy(),
		Functions:   p.functions.sliceHeaderCopy(),
		Strings:     p.strings.sliceHeaderCopy(),
	}
}

func (p *PartitionWriter) WriteStats(s *PartitionStats) {
	p.stacktraces.m.RLock()
	s.MaxStacktraceID = int(p.stacktraces.tree.len())
	s.StacktracesTotal = len(p.stacktraces.hashToIdx)
	p.stacktraces.m.RUnlock()

	p.mappings.lock.RLock()
	s.MappingsTotal = len(p.mappings.slice)
	p.mappings.lock.RUnlock()

	p.functions.lock.RLock()
	s.FunctionsTotal = len(p.functions.slice)
	p.functions.lock.RUnlock()

	p.locations.lock.RLock()
	s.LocationsTotal += len(p.locations.slice)
	p.locations.lock.RUnlock()

	p.strings.lock.RLock()
	s.StringsTotal += len(p.strings.slice)
	p.strings.lock.RUnlock()
}

func (p *PartitionWriter) Release() {
	// Noop. Satisfies PartitionReader interface.
}
