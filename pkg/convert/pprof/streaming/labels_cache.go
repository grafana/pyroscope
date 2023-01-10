package streaming

import (
	"github.com/cespare/xxhash/v2"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"golang.org/x/exp/slices"
	"reflect"
	"unsafe"
)

type Labels []uint64

func (l Labels) Len() int           { return len(l) }
func (l Labels) Less(i, j int) bool { return l[i] < l[j] }
func (l Labels) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

var zeroHash = xxhash.Sum64(nil)

func (l Labels) Hash() uint64 {
	if len(l) == 0 {
		return zeroHash
	}
	slices.Sort(l) //sort.Sort(l) // slice to interface conversion does an allocation for a slice header copy
	var raw []byte
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&raw))
	sh.Data = uintptr(unsafe.Pointer(&l[0]))
	sh.Len = len(l) * 8
	sh.Cap = len(l) * 8
	return xxhash.Sum64(raw)
}

type LabelsCache struct {
	// sample type -> labels hash -> index to labelRefs and trees
	indices []map[uint64]int
	// A label reference points to the subset in labels:
	// Hight 32 bits is the start offset, lower 32 bits is the subset size.
	labelRefs []uint64
	labels    []uint64 // Packed label Key and Value indices
	trees     []*tree.Tree
}

func (c *LabelsCache) Reset() {
	if c.indices == nil {
		c.indices = make([]map[uint64]int, 4, 4)
	} else {
		for i := range c.indices {
			c.indices[i] = nil
		}
		for i := range c.trees {
			c.trees[i] = nil
		}
		c.labelRefs = c.labelRefs[:0]
		c.labels = c.labels[:0]
		c.trees = c.trees[:0]
	}
}

func (c *LabelsCache) GetOrCreateTree(sampleTypeIndex int, l Labels) *tree.Tree {
	if sampleTypeIndex >= len(c.indices) {
		newSampleTypes := make([]map[uint64]int, sampleTypeIndex+1, sampleTypeIndex+1)
		copy(newSampleTypes, c.indices)
		c.indices = newSampleTypes
	}
	p := c.indices[sampleTypeIndex]
	if p == nil {
		e, t := c.newCacheEntry(l)
		c.indices[sampleTypeIndex] = map[uint64]int{l.Hash(): e}
		return t
	}
	h := l.Hash()
	e, found := p[h]
	if found {
		return c.trees[e]
	}
	e, t := c.newCacheEntry(l)
	p[h] = e
	return t
}

func (c *LabelsCache) Get(sampleTypeIndex int, h uint64) (*tree.Tree, bool) {
	if sampleTypeIndex >= len(c.indices) {
		return nil, false
	}
	p := c.indices[sampleTypeIndex]
	if p == nil {
		return nil, false
	}
	x, ok := p[h]
	return c.trees[x], ok
}

func (c *LabelsCache) Remove(sampleTypeIndex int, h uint64) {
	if sampleTypeIndex >= len(c.indices) {
		return
	}
	p := c.indices[sampleTypeIndex]
	if p == nil {
		return
	}
	delete(p, h)
	if len(p) == 0 {
		c.indices[sampleTypeIndex] = nil
	}
}

func (c *LabelsCache) newCacheEntry(l Labels) (int, *tree.Tree) {
	from := len(c.labels)
	for _, u := range l {
		c.labels = append(c.labels, u)
	}
	to := len(c.labels)
	res := len(c.labelRefs)
	c.labelRefs = append(c.labelRefs, uint64(from<<32|to))
	t := tree.New()
	c.trees = append(c.trees, t)
	return res, t
}

func (c *LabelsCache) iterate(f func(sampleTypeIndex int, l Labels, lh uint64, t *tree.Tree) error) error {
	for sampleTypeIndex, p := range c.indices {
		if p == nil {
			continue
		}
		for h, x := range p {
			labelRef := c.labelRefs[x]
			l := c.labels[labelRef>>32 : labelRef&0xffffffff]
			err := f(sampleTypeIndex, l, h, c.trees[x])
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// CutLabel creates a copy of labels without label i.
func CutLabel(labels Labels, i int) Labels {
	c := make(Labels, len(labels)-1)
	copy(c[:i], labels[:i])
	copy(c[i:], labels[i+1:])
	return c
}
