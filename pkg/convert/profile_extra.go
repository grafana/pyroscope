package convert

// These functions are kept separately as profile.pb.go is a generated file

import (
	"sort"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/valyala/bytebufferpool"
)

type cacheKey []int64

type cacheEntry struct {
	key cacheKey
	val *spy.Labels
}
type cache struct {
	data []*cacheEntry
}

func newCache() *cache {
	return &cache{
		data: []*cacheEntry{},
	}
}

func getCacheKey(l []*Label) cacheKey {
	r := []int64{}
	for _, x := range l {
		if x.Str != 0 {
			r = append(r, x.Key, x.Str)
		}
	}
	return r
}

func eq(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func (c *cache) pprofLabelsToSpyLabels(x *Profile, pprofLabels []*Label) *spy.Labels {
	k := getCacheKey(pprofLabels)
	for _, e := range c.data {
		if eq(e.key, k) {
			return e.val
		}
	}

	l := spy.NewLabels()
	for _, pl := range pprofLabels {
		if pl.Str != 0 {
			l.Set(x.StringTable[pl.Key], x.StringTable[pl.Str])
		}
	}
	newVal := &cacheEntry{
		key: k,
		val: l,
	}
	c.data = append(c.data, newVal)
	return l
}

func (x *Profile) Get(sampleType string, cb func(labels *spy.Labels, name []byte, val int)) error {
	valueIndex := 0
	if sampleType != "" {
		for i, v := range x.SampleType {
			if x.StringTable[v.Type] == sampleType {
				valueIndex = i
				break
			}
		}
	}

	labelsCache := newCache()

	b := bytebufferpool.Get()
	defer bytebufferpool.Put(b)

	for _, s := range x.Sample {
		for i := len(s.LocationId) - 1; i >= 0; i-- {
			name, ok := x.findFunctionName(s.LocationId[i])
			if !ok {
				continue
			}
			if b.Len() > 0 {
				_ = b.WriteByte(';')
			}
			_, _ = b.WriteString(name)
		}

		labels := labelsCache.pprofLabelsToSpyLabels(x, s.Label)
		cb(labels, b.Bytes(), int(s.Value[valueIndex]))

		b.Reset()
	}

	return nil
}

func (x *Profile) findFunctionName(locID uint64) (string, bool) {
	if loc, ok := x.findLocation(locID); ok {
		if fn, ok := x.findFunction(loc.Line[0].FunctionId); ok {
			return x.StringTable[fn.Name], true
		}
	}
	return "", false
}

func (x *Profile) findLocation(lid uint64) (*Location, bool) {
	idx := sort.Search(len(x.Location), func(i int) bool {
		return x.Location[i].Id >= lid
	})
	if idx < len(x.Location) {
		if l := x.Location[idx]; l.Id == lid {
			return l, true
		}
	}
	return nil, false
}

func (x *Profile) findFunction(fid uint64) (*Function, bool) {
	idx := sort.Search(len(x.Function), func(i int) bool {
		return x.Function[i].Id >= fid
	})
	if idx < len(x.Function) {
		if f := x.Function[idx]; f.Id == fid {
			return f, true
		}
	}
	return nil, false
}
