package dimension

import (
	"bytes"
	"sort"
	"sync"
)

type Key []byte

type Dimension struct {
	m sync.RWMutex
	// keys are sorted
	Keys []Key
}

func New() *Dimension {
	return &Dimension{
		Keys: []Key{},
	}
}

func (d *Dimension) Insert(key Key) {
	d.m.Lock()
	defer d.m.Unlock()

	i := sort.Search(len(d.Keys), func(i int) bool {
		return bytes.Compare(d.Keys[i], key) >= 0
	})

	if i < len(d.Keys) && bytes.Equal(d.Keys[i], key) {
		return
	}

	if i > len(d.Keys)-1 || !bytes.Equal(d.Keys[i], key) {
		d.Keys = append(d.Keys, key)
		copy(d.Keys[i+1:], d.Keys[i:])
		d.Keys[i] = key
	}
}

func (d *Dimension) Delete(key Key) {
	d.m.Lock()
	defer d.m.Unlock()

	i := sort.Search(len(d.Keys), func(i int) bool {
		return bytes.Compare(d.Keys[i], key) >= 0
	})

	if i < len(d.Keys) && bytes.Equal(d.Keys[i], key) {
		d.Keys = append(d.Keys[:i], d.Keys[i+1:]...)
		return
	}
}

type advanceResult int

const (
	match advanceResult = iota
	noMatch
	end
)

type sortableDim struct {
	keys []Key
	i    int
	l    int
}

func (sd *sortableDim) current() Key {
	return sd.keys[sd.i]
}

func (sd *sortableDim) advance(cmp Key) advanceResult {
	for {
		v := bytes.Compare(sd.current(), cmp)
		switch v {
		case 0:
			return match
		case 1:
			return noMatch
		}
		sd.i++
		if sd.i == sd.l {
			return end
		}
	}
}

type sortableDims []*sortableDim

func (s sortableDims) Len() int {
	return len(s)
}

func (s sortableDims) Less(i, j int) bool {
	return bytes.Compare(s[i].current(), s[j].current()) >= 0
}

func (s sortableDims) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// finds keys that are present in all dimensions
func Intersection(input ...*Dimension) []Key {
	if len(input) == 0 {
		return []Key{}
	} else if len(input) == 1 {
		return input[0].Keys
	}

	// TODO(kolesnikov): shouldn't a bitmap be used?
	r := make(map[string]int)
	for _, v := range input {
		for _, k := range v.Keys {
			r[string(k)]++
		}
	}

	var res []Key
	for k, v := range r {
		if v == len(input) {
			res = append(res, []byte(k))
		}
	}

	return res
}

// TODO: we need to take advantage of the fact that these are sorted arrays
// Current implementation might be taking too much memory
func Union(input ...*Dimension) []Key {
	if len(input) == 0 {
		return []Key{}
	} else if len(input) == 1 {
		return input[0].Keys
	}

	result := []Key{}

	isExists := map[string]bool{}

	for _, v := range input {
		for _, k := range v.Keys {
			if !isExists[string(k)] {
				result = append(result, k)
			}

			isExists[string(k)] = true
		}
	}

	return result
}

// TODO: rework
func AndNot(a, b *Dimension) []Key {
	a.m.RLock()
	defer a.m.RUnlock()
	if len(a.Keys) == 0 {
		return nil
	}

	b.m.RLock()
	defer b.m.RUnlock()
	if len(b.Keys) == 0 {
		r := make([]Key, len(a.Keys))
		copy(r, a.Keys)
		return r
	}

	r := make([]Key, 0, len(a.Keys))
	m := make(map[string]struct{}, len(b.Keys))

	for _, k := range b.Keys {
		m[string(k)] = struct{}{}
	}

	for _, k := range a.Keys {
		if _, ok := m[string(k)]; !ok {
			r = append(r, k)
		}
	}

	return r
}
