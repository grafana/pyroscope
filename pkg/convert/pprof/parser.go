package pprof

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"time"
	"unsafe"

	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type Parser struct {
	putter      storage.Putter
	spyName     string
	labels      map[string]string
	sampleTypes map[string]*tree.SampleTypeConfig
	cache       labelsCache
}

type ParserConfig struct {
	Putter      storage.Putter
	SpyName     string
	Labels      map[string]string
	SampleTypes map[string]*tree.SampleTypeConfig
}

func NewParser(config ParserConfig) *Parser {
	return &Parser{
		putter:      config.Putter,
		spyName:     config.SpyName,
		labels:      config.Labels,
		sampleTypes: config.SampleTypes,
		cache:       make(labelsCache),
	}
}

func (p *Parser) Reset() { p.cache = make(labelsCache) }

func (p *Parser) ParsePprof(ctx context.Context, startTime, endTime time.Time, b io.Reader) error {
	return DecodePool(b, func(profile *tree.Profile) error {
		return p.Convert(ctx, startTime, endTime, profile)
	})
}

func (p *Parser) Convert(ctx context.Context, startTime, endTime time.Time, profile *tree.Profile) error {
	return p.iterate(profile, func(vt *tree.ValueType, l tree.Labels, t *tree.Tree) (keep bool, err error) {
		if vt.Type >= int64(len(profile.StringTable)) {
			return false, fmt.Errorf("sample value type is invalid")
		}
		sampleType := profile.StringTable[vt.Type]
		sampleTypeConfig, ok := p.sampleTypes[sampleType]
		if !ok {
			return false, fmt.Errorf("sample value type is unknown")
		}
		pi := storage.PutInput{
			StartTime: startTime,
			EndTime:   endTime,
			SpyName:   p.spyName,
			Val:       t,
		}
		// Cumulative profiles require two consecutive samples,
		// therefore we have to cache this trie.
		if sampleTypeConfig.Cumulative {
			prev, found := p.load(vt.Type, l)
			if !found {
				// Keep the current entry in cache.
				return true, nil
			}
			// Take diff with the previous tree.
			// The result is written to prev, t is not changed.
			pi.Val = prev.Diff(t)
		}
		pi.AggregationType = sampleTypeConfig.Aggregation
		if sampleTypeConfig.Sampled {
			pi.SampleRate = sampleRate(profile)
		}
		if sampleTypeConfig.DisplayName != "" {
			sampleType = sampleTypeConfig.DisplayName
		}
		if sampleTypeConfig.Units != "" {
			pi.Units = sampleTypeConfig.Units
		} else {
			// TODO(petethepig): this conversion is questionable
			pi.Units = metadata.Units(profile.StringTable[vt.Unit])
		}
		pi.Key = p.buildName(sampleType, profile.ResolveLabels(l))
		err = p.putter.Put(ctx, &pi)
		return sampleTypeConfig.Cumulative, err
	})
}

func sampleRate(p *tree.Profile) uint32 {
	if p.Period <= 0 || p.PeriodType == nil {
		return 0
	}
	sampleUnit := time.Nanosecond
	switch p.StringTable[p.PeriodType.Unit] {
	case "microseconds":
		sampleUnit = time.Microsecond
	case "milliseconds":
		sampleUnit = time.Millisecond
	case "seconds":
		sampleUnit = time.Second
	}
	return uint32(time.Second / (sampleUnit * time.Duration(p.Period)))
}

func (p *Parser) buildName(sampleTypeName string, labels map[string]string) *segment.Key {
	for k, v := range p.labels {
		labels[k] = v
	}
	labels["__name__"] += "." + sampleTypeName
	return segment.NewKey(labels)
}

func (p *Parser) load(sampleType int64, labels tree.Labels) (*tree.Tree, bool) {
	e, ok := p.cache.get(sampleType, labels.Hash())
	if !ok {
		return nil, false
	}
	return e.Tree, true
}

func (p *Parser) iterate(x *tree.Profile, fn func(vt *tree.ValueType, l tree.Labels, t *tree.Tree) (keep bool, err error)) error {
	c := make(labelsCache)
	p.readTrees(x, c, tree.NewFinder(x))
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
	p.cache = c
	return nil
}

// readTrees generates trees from the profile populating c.
func (p *Parser) readTrees(x *tree.Profile, c labelsCache, f tree.Finder) {
	// SampleType value indexes.
	indexes := make([]int, 0, len(x.SampleType))
	// Corresponding type IDs used as the main cache keys.
	types := make([]int64, 0, len(x.SampleType))
	for i, s := range x.SampleType {
		if len(p.sampleTypes) > 0 {
			if _, ok := p.sampleTypes[x.StringTable[s.Type]]; !ok {
				continue
			}
		}
		indexes = append(indexes, i)
		types = append(types, s.Type)
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
				if j := labelIndex(x, s.Label, segment.ProfileIDLabelName); j >= 0 {
					c.getOrCreate(types[i], cutLabel(s.Label, j)).InsertStack(stack, v)
				}
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
	return &labelsCacheEntry{Tree: tree.New(), Labels: copyLabels(l)}
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

func labelIndex(p *tree.Profile, labels tree.Labels, key string) int {
	for i, label := range labels {
		if n, ok := p.ResolveLabelName(label); ok && n == key {
			return i
		}
	}
	return -1
}

func copyLabels(labels tree.Labels) tree.Labels {
	l := make(tree.Labels, len(labels))
	for i, v := range labels {
		l[i] = copyLabel(v)
	}
	return l
}

// cutLabel creates a copy of labels without label i.
func cutLabel(labels tree.Labels, i int) tree.Labels {
	c := make(tree.Labels, 0, len(labels)-1)
	for j, label := range labels {
		if i != j {
			c = append(c, copyLabel(label))
		}
	}
	return c
}

func copyLabel(label *tree.Label) *tree.Label {
	return &tree.Label{
		Key:     label.Key,
		Str:     label.Str,
		Num:     label.Num,
		NumUnit: label.NumUnit,
	}
}
