package tree

// sample type -> labels hash -> entry
type LabelsCache map[int64]map[uint64]*LabelsCacheEntry

type LabelsCacheEntry struct {
	Labels
	*Tree
}

func NewCacheEntry(l Labels) *LabelsCacheEntry {
	return &LabelsCacheEntry{Tree: New(), Labels: CopyLabels(l)}
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
		l[i] = CopyLabel(v)
	}
	return l
}

// CutLabel creates a copy of labels without label i.
func CutLabel(labels Labels, i int) Labels {
	c := make(Labels, 0, len(labels)-1)
	for j, label := range labels {
		if i != j {
			c = append(c, CopyLabel(label))
		}
	}
	return c
}

func CopyLabel(label *Label) *Label {
	return &Label{
		Key:     label.Key,
		Str:     label.Str,
		Num:     label.Num,
		NumUnit: label.NumUnit,
	}
}
