package streaming

import (
	"encoding/binary"
	"github.com/cespare/xxhash/v2"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"sort"
)

type Labels []label

func (l Labels) Len() int           { return len(l) }
func (l Labels) Less(i, j int) bool { return l[i].k < l[j].k }
func (l Labels) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

func (l Labels) Hash() uint64 {
	h := xxhash.New()
	t := make([]byte, 16)
	sort.Sort(l)
	for _, x := range l {
		if x.v == 0 {
			continue
		}
		binary.LittleEndian.PutUint64(t[0:8], uint64(x.k))
		binary.LittleEndian.PutUint64(t[8:16], uint64(x.v))
		_, _ = h.Write(t)
	}
	return h.Sum64()
}

// sample type -> labels hash -> entry

type LabelsCache map[int64]map[uint64]*LabelsCacheEntry

type LabelsCacheEntry struct {
	Labels
	*tree.Tree
}

func NewCacheEntry(l Labels) *LabelsCacheEntry {
	return &LabelsCacheEntry{Tree: tree.New(), Labels: CopyLabels(l)}
}

func (c LabelsCache) GetOrCreateTree(sampleType int64, l Labels) *LabelsCacheEntry {
	p, ok := c[sampleType]
	if !ok {
		e := NewCacheEntry(l)
		c[sampleType] = map[uint64]*LabelsCacheEntry{l.Hash(): e}
		return e
	}
	h := l.Hash()
	e, found := p[h]
	if !found {
		e = NewCacheEntry(l)
		p[h] = e
	}
	return e
}

func (c LabelsCache) GetOrCreateTreeByHash(sampleType int64, l Labels, h uint64) *LabelsCacheEntry {
	p, ok := c[sampleType]
	if !ok {
		e := NewCacheEntry(l)
		c[sampleType] = map[uint64]*LabelsCacheEntry{h: e}
		return e
	}
	e, found := p[h]
	if !found {
		e = NewCacheEntry(l)
		p[h] = e
	}
	return e
}

func (c LabelsCache) Get(sampleType int64, h uint64) (*LabelsCacheEntry, bool) {
	p, ok := c[sampleType]
	if !ok {
		return nil, false
	}
	x, ok := p[h]
	return x, ok
}

func (c LabelsCache) Put(sampleType int64, e *LabelsCacheEntry) {
	p, ok := c[sampleType]
	if !ok {
		p = make(map[uint64]*LabelsCacheEntry)
		c[sampleType] = p
	}
	p[e.Hash()] = e
}

func (c LabelsCache) Remove(sampleType int64, h uint64) {
	p, ok := c[sampleType]
	if !ok {
		return
	}
	delete(p, h)
	if len(p) == 0 {
		delete(c, sampleType)
	}
}

func CopyLabels(labels Labels) Labels {
	l := make(Labels, len(labels))
	for i, v := range labels {
		l[i] = copyLabel(v)
	}
	return l
}

// CutLabel creates a copy of labels without label i.
func CutLabel(labels Labels, i int) Labels {
	c := make(Labels, 0, len(labels)-1)
	for j, label := range labels {
		if i != j {
			c = append(c, copyLabel(label))
		}
	}
	return c
}

func copyLabel(label label) label {
	return label
}
