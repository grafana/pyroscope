package pprof

import (
	"reflect"
	"unsafe"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type ProfileReader struct {
	sampleTypesFilter func(string) bool
	cache             labelsCache
}

func NewProfileReader() *ProfileReader { return new(ProfileReader) }

func (r *ProfileReader) SampleTypeFilter(fn func(string) bool) *ProfileReader {
	r.sampleTypesFilter = fn
	return r
}

func AllSampleTypes(string) bool { return true }

func (r *ProfileReader) Reset() {
	r.cache = make(labelsCache)
}

func (r *ProfileReader) Load(sampleType int64, labels tree.Labels) (*tree.Tree, bool) {
	e, ok := r.cache.get(sampleType, labels.Hash())
	if !ok {
		return nil, false
	}
	return e.Tree, true
}

func (r *ProfileReader) Read(x *tree.Profile, fn func(vt *tree.ValueType, l tree.Labels, t *tree.Tree) (keep bool, err error)) error {
	// Reuse cache if it is empty.
	c := r.cache
	if len(r.cache) != 0 || r.cache == nil {
		c = make(labelsCache)
	}
	r.readTrees(x, c, tree.NewFinder(x))
	for sampleType, entries := range c {
		if t, ok := x.ResolveSampleType(sampleType); ok {
			for h, e := range entries {
				keep, err := fn(t, e.Labels, e.Tree)
				if err != nil {
					return err
				}
				if !keep {
					c.remove(sampleType, h)
				}
			}
		}
	}
	r.cache = c
	return nil
}

// readTrees generates trees from the profile populating c.
func (r *ProfileReader) readTrees(x *tree.Profile, c labelsCache, f tree.Finder) {
	// SampleType value indexes.
	indexes := make([]int, 0, len(x.SampleType))
	// Corresponding type IDs used as the main cache keys.
	types := make([]int64, 0, len(x.SampleType))
	for i, s := range x.SampleType {
		if r.sampleTypesFilter != nil && r.sampleTypesFilter(x.StringTable[s.Type]) {
			indexes = append(indexes, i)
			types = append(types, s.Type)
		}
	}
	if len(indexes) == 0 {
		return
	}
	stack := make([][]byte, 0, 16)
	for _, s := range x.Sample {
		for i := len(s.LocationId) - 1; i >= 0; i-- {
			// Resolve stack.
			loc, ok := f.FindLocation(s.LocationId[i])
			if !ok {
				continue
			}
			// Multiple line indicates this location has inlined functions,
			// where the last entry represents the caller into which the
			// preceding entries were inlined.
			//
			// E.g., if memcpy() is inlined into printf:
			//    line[0].function_name == "memcpy"
			//    line[1].function_name == "printf"
			//
			// Therefore iteration goes in reverse order.
			for j := len(loc.Line) - 1; j >= 0; j-- {
				fn, ok := f.FindFunction(loc.Line[j].FunctionId)
				if !ok || x.StringTable[fn.Name] == "" {
					continue
				}
				stack = append(stack, unsafeStrToSlice(x.StringTable[fn.Name]))
			}
		}
		// Insert tree nodes.
		for i, vi := range indexes {
			if v := uint64(s.Value[vi]); v != 0 {
				c.getOrCreate(types[i], s.Label).InsertStack(stack, v)
			}
		}
		stack = stack[:0]
	}
}

func unsafeStrToSlice(s string) []byte {
	return (*[0x7fff0000]byte)(unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data))[:len(s):len(s)]
}

// sample type -> labels hash -> entry
type labelsCache map[int64]map[uint64]*labelsCacheEntry

type labelsCacheEntry struct {
	tree.Labels
	*tree.Tree
}

func newCacheEntry(l tree.Labels) *labelsCacheEntry {
	return &labelsCacheEntry{Tree: tree.New(), Labels: l}
}

func (c labelsCache) getOrCreate(sampleType int64, l tree.Labels) *labelsCacheEntry {
	p, ok := c[sampleType]
	if !ok {
		e := newCacheEntry(l)
		c[sampleType] = map[uint64]*labelsCacheEntry{l.Hash(): e}
		return e
	}
	h := l.Hash()
	e, found := p[h]
	if !found {
		e = newCacheEntry(l)
		p[h] = e
	}
	return e
}

func (c labelsCache) get(sampleType int64, h uint64) (*labelsCacheEntry, bool) {
	p, ok := c[sampleType]
	if !ok {
		return nil, false
	}
	x, ok := p[h]
	return x, ok
}

func (c labelsCache) put(sampleType int64, e *labelsCacheEntry) {
	p, ok := c[sampleType]
	if !ok {
		p = make(map[uint64]*labelsCacheEntry)
		c[sampleType] = p
	}
	p[e.Hash()] = e
}

func (c labelsCache) remove(sampleType int64, h uint64) {
	p, ok := c[sampleType]
	if !ok {
		return
	}
	delete(p, h)
	if len(p) == 0 {
		delete(c, sampleType)
	}
}
