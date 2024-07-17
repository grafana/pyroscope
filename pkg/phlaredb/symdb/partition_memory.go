package symdb

import (
	"context"
	"io"
	"sync"

	"github.com/grafana/pyroscope/pkg/iter"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type PartitionWriter struct {
	header PartitionHeader

	stacktraces *stacktracesPartition
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
	if len(p.stacktraces.chunks) == 0 {
		return dst
	}
	chunkID := stacktraceID / p.stacktraces.maxNodesPerChunk
	localSID := stacktraceID % p.stacktraces.maxNodesPerChunk
	if localSID == 0 || int(chunkID) > len(p.stacktraces.chunks) {
		return dst
	}
	return p.stacktraces.chunks[chunkID].tree.resolveUint64(dst, localSID)
}

type stacktracesPartition struct {
	maxNodesPerChunk uint32

	m         sync.RWMutex
	hashToIdx map[uint64]uint32
	chunks    []*stacktraceChunk
}

func (p *PartitionWriter) SplitStacktraceIDRanges(appender *SampleAppender) iter.Iterator[*StacktraceIDRange] {
	if len(p.stacktraces.chunks) == 0 {
		return iter.NewEmptyIterator[*StacktraceIDRange]()
	}
	var n int
	samples := appender.Samples()
	ranges := SplitStacktraces(samples.StacktraceIDs, p.stacktraces.maxNodesPerChunk)
	for _, sr := range ranges {
		c := p.stacktraces.chunks[sr.chunk]
		sr.ParentPointerTree = c.tree
		sr.Samples = samples.Range(n, n+len(sr.IDs))
		n += len(sr.IDs)
	}
	return iter.NewSliceIterator(ranges)
}

func newStacktracesPartition(maxNodesPerChunk uint32) *stacktracesPartition {
	p := &stacktracesPartition{
		maxNodesPerChunk: maxNodesPerChunk,
		hashToIdx:        make(map[uint64]uint32, defaultStacktraceTreeSize/2),
	}
	p.chunks = append(p.chunks, &stacktraceChunk{
		tree:      newStacktraceTree(defaultStacktraceTreeSize),
		partition: p,
	})
	return p
}

func (p *stacktracesPartition) size() uint64 {
	p.m.RLock()
	// TODO: map footprint isn't accounted
	v := 0
	for _, c := range p.chunks {
		v += stacktraceTreeNodeSize * cap(c.tree.nodes)
	}
	p.m.RUnlock()
	return uint64(v)
}

// stacktraceChunkForInsert returns a chunk for insertion:
// if the existing one has capacity, or a new one, if the former is full.
// Must be called with the stracktraces mutex write lock held.
func (p *stacktracesPartition) stacktraceChunkForInsert(x int) *stacktraceChunk {
	c := p.currentStacktraceChunk()
	if n := c.tree.len() + uint32(x); p.maxNodesPerChunk > 0 && n >= p.maxNodesPerChunk {
		// Calculate number of stacks in the chunk.
		s := uint32(len(p.hashToIdx))
		c.stacks = s - c.stacks
		c = &stacktraceChunk{
			partition: p,
			tree:      newStacktraceTree(defaultStacktraceTreeSize),
			stid:      c.stid + p.maxNodesPerChunk,
			stacks:    s,
		}
		p.chunks = append(p.chunks, c)
	}
	return c
}

// stacktraceChunkForRead returns a chunk for reads.
// Must be called with the stracktraces mutex read lock held.
func (p *stacktracesPartition) stacktraceChunkForRead(i int) (*stacktraceChunk, bool) {
	if i < len(p.chunks) {
		return p.chunks[i], true
	}
	return nil, false
}

func (p *stacktracesPartition) currentStacktraceChunk() *stacktraceChunk {
	// Assuming there is at least one chunk.
	return p.chunks[len(p.chunks)-1]
}

func (p *stacktracesPartition) append(dst []uint32, s []*schemav1.Stacktrace) {
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
	chunk := p.currentStacktraceChunk()

	m := int(p.maxNodesPerChunk)
	t, j := chunk.tree, chunk.stid
	for i, v := range dst[:len(s)] {
		if v != 0 {
			// Already resolved. ID 0 is reserved
			// as it is the tree root.
			continue
		}

		x := s[i].LocationIDs
		if m > 0 && len(t.nodes)+len(x) >= m {
			// If we're close to the max nodes limit and can
			// potentially exceed it, we take the next chunk,
			// even if there are some space.
			chunk = p.stacktraceChunkForInsert(len(x))
			t, j = chunk.tree, chunk.stid
		}

		// Tree insertion is idempotent,
		// we don't need to check the map.
		id = t.insert(x) + j
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

func (p *stacktracesPartition) resolve(dst StacktraceInserter, stacktraces []uint32) (err error) {
	for _, sr := range SplitStacktraces(stacktraces, p.maxNodesPerChunk) {
		if err = p.ResolveChunk(dst, sr); err != nil {
			return err
		}
	}
	return nil
}

// NOTE(kolesnikovae):
//  Caller is able to split a range of stacktrace IDs into chunks
//  with SplitStacktraces, and then resolve them concurrently:
//  StacktraceInserter could be implemented as a dense set, map,
//  slice, or an n-ary tree: the stacktraceTree should be one of
//  the options, the package provides.

func (p *stacktracesPartition) ResolveChunk(dst StacktraceInserter, sr *StacktraceIDRange) error {
	p.m.RLock()
	c, found := p.stacktraceChunkForRead(int(sr.chunk))
	if !found {
		p.m.RUnlock()
		return ErrInvalidStacktraceRange
	}
	t := stacktraceTree{nodes: c.tree.nodes}
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
	// Restore the original stacktrace ID.
	off := sr.Offset()
	for _, sid := range sr.IDs {
		s = t.resolve(s, sid)
		dst.InsertStacktrace(off+sid, s)
	}
	stacktraceLocations.put(s)
	return nil
}

type stacktraceChunk struct {
	partition *stacktracesPartition
	tree      *stacktraceTree
	stid      uint32 // Initial stack trace ID.
	stacks    uint32 //
}

func (s *stacktraceChunk) WriteTo(dst io.Writer) (int64, error) {
	return s.tree.WriteTo(dst)
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
	c := p.stacktraces.currentStacktraceChunk()
	s.MaxStacktraceID = int(c.stid + c.tree.len())
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
