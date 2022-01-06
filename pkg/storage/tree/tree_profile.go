package tree

import (
	"encoding/binary"
	"reflect"
	"sort"
	"unsafe"

	"github.com/cespare/xxhash/v2"
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

func (r *ProfileReader) Reset() {
	r.cache = make(labelsCache)
}

func (r *ProfileReader) Load(sampleType int64, labels Labels) (*Tree, bool) {
	e, ok := r.cache.get(sampleType, labels.Hash())
	if !ok {
		return nil, false
	}
	return e.Tree, true
}

func (r *ProfileReader) Read(x *Profile, fn func(vt *ValueType, l Labels, t *Tree) (keep bool, err error)) error {
	c := make(labelsCache)
	r.readTrees(x, c, NewFinder(x), 1024)
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

func (x *Profile) ResolveLabels(l Labels) map[string]string {
	m := make(map[string]string, len(l))
	for _, label := range l {
		if label.Str != 0 {
			m[x.StringTable[label.Key]] = x.StringTable[label.Str]
		}
	}
	return m
}

func (x *Profile) ResolveSampleType(v int64) (*ValueType, bool) {
	for _, vt := range x.SampleType {
		if vt.Type == v {
			return vt, true
		}
	}
	return nil, false
}

type Labels []*Label

func (l Labels) Hash() uint64 {
	h := xxhash.New()
	t := make([]byte, 16)
	for _, x := range l {
		if x.Str == 0 {
			continue
		}
		binary.LittleEndian.PutUint64(t[0:8], uint64(x.Key))
		binary.LittleEndian.PutUint64(t[8:16], uint64(x.Str))
		_, _ = h.Write(t)
	}
	return h.Sum64()
}

// readTrees generates trees from the profile populating c.
func (r *ProfileReader) readTrees(x *Profile, c labelsCache, f Finder, l int) {
	stack := make([][]byte, 0, 16)
	for vi, st := range x.SampleType {
		if r.sampleTypesFilter != nil && !r.sampleTypesFilter(x.StringTable[st.Type]) {
			continue
		}
		sort.Sort(sort.Reverse(samplesByValue{vi, x.Sample}))
		for _, s := range x.Sample[:min(len(x.Sample), l)] {
			v := uint64(s.Value[vi])
			if v == 0 {
				continue
			}
			for i := len(s.LocationId) - 1; i >= 0; i-- {
				if loc, ok := f.FindLocation(s.LocationId[i]); ok {
					for j := len(loc.Line) - 1; j >= 0; j-- {
						if fn, ok := f.FindFunction(loc.Line[j].FunctionId); ok {
							stack = append(stack, unsafeStrToSlice(x.StringTable[fn.Name]))
						}
					}
				}
			}
			c.getOrCreate(st.Type, s.Label).InsertStack(stack, v)
			stack = stack[:0]
		}
	}
}

type samplesByValue struct {
	i int
	s []*Sample
}

func (x samplesByValue) Len() int           { return len(x.s) }
func (x samplesByValue) Less(i, j int) bool { return x.s[i].Value[x.i] < x.s[j].Value[x.i] }
func (x samplesByValue) Swap(i, j int)      { x.s[i], x.s[j] = x.s[j], x.s[i] }

func unsafeStrToSlice(s string) []byte {
	return (*[0x7fff0000]byte)(unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data))[:len(s):len(s)]
}

// sample type -> labels hash -> entry
type labelsCache map[int64]map[uint64]*labelsCacheEntry

type labelsCacheEntry struct {
	Labels
	*Tree
}

func newCacheEntry(l Labels) *labelsCacheEntry {
	return &labelsCacheEntry{Tree: New(), Labels: l}
}

func (c labelsCache) getOrCreate(sampleType int64, l Labels) *labelsCacheEntry {
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
