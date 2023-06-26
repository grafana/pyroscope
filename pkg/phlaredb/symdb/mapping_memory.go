package symdb

import (
	"context"
	"hash/maphash"
	"io"
	"reflect"
	"sync"
	"unsafe"

	"github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
)

var (
	_ MappingReader = (*inMemoryMapping)(nil)
	_ MappingWriter = (*inMemoryMapping)(nil)

	_ StacktraceAppender = (*stacktraceAppender)(nil)
	_ StacktraceResolver = (*stacktraceResolverMemory)(nil)
)

type inMemoryMapping struct {
	name uint64

	maxNodesPerChunk uint32
	// maxStackDepth uint32

	// Stack traces originating from the mapping (binary):
	// their bottom frames (roots) refer to this mapping.
	stacktraceMutex    sync.RWMutex
	stacktraceHashToID map[uint64]uint32
	stacktraceChunks   []*stacktraceChunk
	// Headers of already written stack trace chunks.
	stacktraceChunkHeaders []StacktraceChunkHeader
}

func (b *inMemoryMapping) StacktraceAppender() StacktraceAppender {
	b.stacktraceMutex.RLock()
	// Assuming there is at least one chunk.
	c := b.stacktraceChunks[len(b.stacktraceChunks)-1]
	b.stacktraceMutex.RUnlock()
	return &stacktraceAppender{
		mapping: b,
		chunk:   c,
	}
}

func (b *inMemoryMapping) StacktraceResolver() StacktraceResolver {
	return &stacktraceResolverMemory{
		mapping: b,
	}
}

// stacktraceChunkForInsert returns a chunk for insertion:
// if the existing one has capacity, or a new one, if the former is full.
// Must be called with the stracktraces mutex write lock held.
func (b *inMemoryMapping) stacktraceChunkForInsert(x int) *stacktraceChunk {
	c := b.stacktraceChunks[len(b.stacktraceChunks)-1]
	if n := c.tree.len() + uint32(x); b.maxNodesPerChunk > 0 && n >= b.maxNodesPerChunk {
		c = &stacktraceChunk{
			mapping: b,
			tree:    newStacktraceTree(defaultStacktraceTreeSize),
			stid:    c.stid + b.maxNodesPerChunk,
		}
		b.stacktraceChunks = append(b.stacktraceChunks, c)
	}
	return c
}

// stacktraceChunkForRead returns a chunk for reads.
// Must be called with the stracktraces mutex read lock held.
func (b *inMemoryMapping) stacktraceChunkForRead(i int) (*stacktraceChunk, bool) {
	if i < len(b.stacktraceChunks) {
		return b.stacktraceChunks[i], true
	}
	return nil, false
}

type stacktraceChunk struct {
	mapping *inMemoryMapping
	stid    uint32 // Initial stack trace ID.
	tree    *stacktraceTree
}

func (s *stacktraceChunk) WriteTo(dst io.Writer) (int64, error) {
	return s.tree.WriteTo(dst)
}

type stacktraceAppender struct {
	mapping     *inMemoryMapping
	chunk       *stacktraceChunk
	releaseOnce sync.Once
}

func (a *stacktraceAppender) AppendStacktrace(dst []uint32, s []*v1.Stacktrace) {
	if len(s) == 0 {
		return
	}

	var (
		id     uint32
		found  bool
		misses int
	)

	a.mapping.stacktraceMutex.RLock()
	for i, x := range s {
		if dst[i], found = a.mapping.stacktraceHashToID[hashLocations(x.LocationIDs)]; !found {
			misses++
		}
	}
	a.mapping.stacktraceMutex.RUnlock()
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

	a.mapping.stacktraceMutex.Lock()
	defer a.mapping.stacktraceMutex.Unlock()

	m := int(a.mapping.maxNodesPerChunk)
	t, j := a.chunk.tree, a.chunk.stid
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
			a.chunk = a.mapping.stacktraceChunkForInsert(len(x))
			t, j = a.chunk.tree, a.chunk.stid
		}

		// Tree insertion is idempotent,
		// we don't need to check the map.
		id = t.insert(x) + j
		h := hashLocations(x)
		a.mapping.stacktraceHashToID[h] = id
		dst[i] = id
	}
}

func (a *stacktraceAppender) Release() {}

func (r *stacktraceResolverMemory) Release() {}

var seed = maphash.MakeSeed()

func hashLocations(s []uint64) uint64 {
	if len(s) == 0 {
		return 0
	}
	var b []byte
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	hdr.Len = len(s) * 8
	hdr.Cap = hdr.Len
	hdr.Data = uintptr(unsafe.Pointer(&s[0]))
	return maphash.Bytes(seed, b)
}

type stacktraceResolverMemory struct {
	mapping *inMemoryMapping
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

func (r *stacktraceResolverMemory) ResolveStacktraces(_ context.Context, dst StacktraceInserter, stacktraces []uint32) (err error) {
	// TODO(kolesnikovae): Add option to do resolve concurrently.
	//   Depends on StacktraceInserter implementation.
	for _, sr := range SplitStacktraces(stacktraces, r.mapping.maxNodesPerChunk) {
		if err = r.ResolveStacktracesChunk(dst, sr); err != nil {
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

func (r *stacktraceResolverMemory) ResolveStacktracesChunk(dst StacktraceInserter, sr StacktracesRange) error {
	r.mapping.stacktraceMutex.RLock()
	c, found := r.mapping.stacktraceChunkForRead(int(sr.chunk))
	if !found {
		r.mapping.stacktraceMutex.RUnlock()
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
	r.mapping.stacktraceMutex.RUnlock()
	s := stacktraceLocations.get()
	// Restore the original stacktrace ID.
	off := sr.offset()
	for _, sid := range sr.ids {
		s = t.resolve(s, sid)
		dst.InsertStacktrace(off+sid, s)
	}
	stacktraceLocations.put(s)
	return nil
}

type StacktracesRange struct {
	ids   []uint32
	chunk uint32 // Chunk index.
	m     uint32 // Max nodes per chunk.
}

func (r StacktracesRange) offset() uint32 { return r.m * r.chunk }

// SplitStacktraces splits the range of stack trace IDs by limit n into
// sub-ranges matching to the corresponding chunks and shifts the values
// accordingly. Note that the input s is modified in place.
//
// stack trace ID 0 is reserved and is not expected at the input.
// stack trace ID % max_nodes == 0 is not expected as well.
func SplitStacktraces(s []uint32, n uint32) []StacktracesRange {
	if s[len(s)-1] < n || n == 0 {
		// Fast path, just one chunk: the highest stack trace ID
		// is less than the chunk size, or the size is not limited.
		// It's expected that in most cases we'll end up here.
		return []StacktracesRange{{m: n, ids: s}}
	}

	var (
		loi int
		lov = (s[0] / n) * n // Lowest possible value for the current chunk.
		hiv = lov + n        // Highest possible value for the current chunk.
		p   uint32           // Previous value (to derive chunk index).
		// 16 chunks should be more than enough in most cases.
		cs = make([]StacktracesRange, 0, 16)
	)

	for i, v := range s {
		if v < hiv {
			// The stack belongs to the current chunk.
			s[i] -= lov
			p = v
			continue
		}
		lov = (v / n) * n
		hiv = lov + n
		s[i] -= lov
		cs = append(cs, StacktracesRange{
			chunk: p / n,
			ids:   s[loi:i],
			m:     n,
		})
		loi = i
		p = v
	}

	if t := s[loi:]; len(t) > 0 {
		cs = append(cs, StacktracesRange{
			chunk: p / n,
			ids:   t,
			m:     n,
		})
	}

	return cs
}
