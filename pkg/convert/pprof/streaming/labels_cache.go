package streaming

import "C"
import (
	"encoding/binary"
	"github.com/cespare/xxhash/v2"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"golang.org/x/exp/slices"
)

type Labels []uint64

func (l Labels) Len() int           { return len(l) }
func (l Labels) Less(i, j int) bool { return l[i] < l[j] }
func (l Labels) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

func (l Labels) Hash() uint64 {
	h := xxhash.Digest{}
	h.Reset()
	var t [8]byte
	//sort.Sort(l) // slice to interface conversion does an allocation for a slice header copy
	slices.Sort(l)
	for _, x := range l {
		binary.LittleEndian.PutUint64(t[0:8], uint64(x))
		_, _ = h.Write(t[:])
	}
	return h.Sum64()
}

// sample type index -> labels hash -> entry

type LabelsCache struct {
	sampleTypes []map[uint64]*LabelsCacheEntry
}

func (c *LabelsCache) Reset() {
	if c.sampleTypes == nil {
		c.sampleTypes = make([]map[uint64]*LabelsCacheEntry, 4, 4)
	} else {
		for i := range c.sampleTypes {
			c.sampleTypes[i] = nil
		}
	}
}

type LabelsCacheEntry struct {
	Labels
	*tree.Tree
}

func NewCacheEntry(l Labels) *LabelsCacheEntry {
	return &LabelsCacheEntry{Tree: tree.New(), Labels: CopyLabels(l)}
}

func (c *LabelsCache) GetOrCreateTree(sampleTypeIndex int, l Labels) *LabelsCacheEntry {
	if sampleTypeIndex >= len(c.sampleTypes) {
		newSampleTypes := make([]map[uint64]*LabelsCacheEntry, sampleTypeIndex+1, sampleTypeIndex+1)
		copy(newSampleTypes, c.sampleTypes)
		c.sampleTypes = newSampleTypes
	}
	p := c.sampleTypes[sampleTypeIndex]
	if p == nil {
		e := NewCacheEntry(l)
		c.sampleTypes[sampleTypeIndex] = map[uint64]*LabelsCacheEntry{l.Hash(): e}
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

func (c *LabelsCache) Get(sampleTypeIndex int, h uint64) (*LabelsCacheEntry, bool) {
	if sampleTypeIndex >= len(c.sampleTypes) {
		return nil, false
	}
	p := c.sampleTypes[sampleTypeIndex]
	if p == nil {
		return nil, false
	}
	x, ok := p[h]
	return x, ok
}

func (c *LabelsCache) Remove(sampleTypeIndex int, h uint64) {
	if sampleTypeIndex >= len(c.sampleTypes) {
		return
	}
	p := c.sampleTypes[sampleTypeIndex]
	if p == nil {
		return
	}
	delete(p, h)
	if len(p) == 0 {
		c.sampleTypes[sampleTypeIndex] = nil
	}
}

func CopyLabels(labels Labels) Labels {
	l := make(Labels, len(labels))
	copy(l, labels)
	return l
}

// CutLabel creates a copy of labels without label i.
func CutLabel(labels Labels, i int) Labels {
	c := make(Labels, len(labels)-1)
	copy(c[:i], labels[:i])
	copy(c[i:], labels[i+1:])
	return c
}
