package query

import (
	"bytes"
	"strings"

	pq "github.com/segmentio/parquet-go"
	"go.uber.org/atomic"
	"golang.org/x/exp/constraints"
)

// Predicate is a pushdown predicate that can be applied at
// the chunk, page, and value levels.
type Predicate interface {
	KeepColumnChunk(cc pq.ColumnChunk) bool
	KeepPage(page pq.Page) bool
	KeepValue(pq.Value) bool
}

// StringInPredicate checks for any of the given strings.
type StringInPredicate struct {
	ss [][]byte
}

var _ Predicate = (*StringInPredicate)(nil)

func NewStringInPredicate(ss []string) Predicate {
	p := &StringInPredicate{
		ss: make([][]byte, len(ss)),
	}
	for i := range ss {
		p.ss[i] = []byte(ss[i])
	}
	return p
}

func (p *StringInPredicate) KeepColumnChunk(cc pq.ColumnChunk) bool {
	if ci := cc.ColumnIndex(); ci != nil {

		for _, subs := range p.ss {
			for i := 0; i < ci.NumPages(); i++ {
				ok := bytes.Compare(ci.MinValue(i).ByteArray(), subs) <= 0 && bytes.Compare(ci.MaxValue(i).ByteArray(), subs) >= 0
				if ok {
					// At least one page in this chunk matches
					return true
				}
			}
		}
		return false
	}

	return true
}

func (p *StringInPredicate) KeepValue(v pq.Value) bool {
	ba := v.ByteArray()
	for _, ss := range p.ss {
		if bytes.Equal(ba, ss) {
			return true
		}
	}
	return false
}

func (p *StringInPredicate) KeepPage(page pq.Page) bool {
	// todo: check bounds

	// If a dictionary column then ensure at least one matching
	// value exists in the dictionary
	dict := page.Dictionary()
	if dict != nil && dict.Len() > 0 {
		len := dict.Len()

		for i := 0; i < len; i++ {
			dictionaryEntry := dict.Index(int32(i)).ByteArray()
			for _, subs := range p.ss {
				if bytes.Equal(dictionaryEntry, subs) {
					// At least 1 string present in this page
					return true
				}
			}
		}

		return false
	}

	return true
}

type SubstringPredicate struct {
	substring string
	matches   map[string]bool
}

var _ Predicate = (*SubstringPredicate)(nil)

func NewSubstringPredicate(substring string) *SubstringPredicate {
	return &SubstringPredicate{
		substring: substring,
		matches:   map[string]bool{},
	}
}

func (p *SubstringPredicate) KeepColumnChunk(_ pq.ColumnChunk) bool {
	// Reset match cache on each row group change
	p.matches = make(map[string]bool, len(p.matches))

	// Is there any filtering possible here?
	// Column chunk contains a bloom filter and min/max bounds,
	// but those can't be inspected for a substring match.
	return true
}

func (p *SubstringPredicate) KeepValue(v pq.Value) bool {
	vs := v.String()
	if m, ok := p.matches[vs]; ok {
		return m
	}

	m := strings.Contains(vs, p.substring)
	p.matches[vs] = m
	return m
}

func (p *SubstringPredicate) KeepPage(page pq.Page) bool {
	// If a dictionary column then ensure at least one matching
	// value exists in the dictionary
	dict := page.Dictionary()
	if dict != nil && dict.Len() > 0 {
		len := dict.Len()
		for i := 0; i < len; i++ {
			if p.KeepValue(dict.Index(int32(i))) {
				return true
			}
		}

		return false
	}

	return true
}

// IntBetweenPredicate checks for int between the bounds [min,max] inclusive
type IntBetweenPredicate struct {
	min, max int64
}

var _ Predicate = (*IntBetweenPredicate)(nil)

func NewIntBetweenPredicate(min, max int64) *IntBetweenPredicate {
	return &IntBetweenPredicate{min, max}
}

func (p *IntBetweenPredicate) KeepColumnChunk(c pq.ColumnChunk) bool {
	if ci := c.ColumnIndex(); ci != nil {
		for i := 0; i < ci.NumPages(); i++ {
			min := ci.MinValue(i).Int64()
			max := ci.MaxValue(i).Int64()
			if p.max >= min && p.min <= max {
				return true
			}
		}
		return false
	}

	return true
}

func (p *IntBetweenPredicate) KeepValue(v pq.Value) bool {
	vv := v.Int64()
	return p.min <= vv && vv <= p.max
}

func (p *IntBetweenPredicate) KeepPage(page pq.Page) bool {
	if min, max, ok := page.Bounds(); ok {
		return p.max >= min.Int64() && p.min <= max.Int64()
	}
	return true
}

type EqualInt64Predicate int64

func NewEqualInt64Predicate(value int64) EqualInt64Predicate {
	return EqualInt64Predicate(value)
}

func (p EqualInt64Predicate) KeepColumnChunk(c pq.ColumnChunk) bool {
	if ci := c.ColumnIndex(); ci != nil {
		for i := 0; i < ci.NumPages(); i++ {
			min := ci.MinValue(i).Int64()
			max := ci.MaxValue(i).Int64()
			if int64(p) >= min && int64(p) <= max {
				return true
			}
		}
		return false
	}

	return true
}

func (p EqualInt64Predicate) KeepValue(v pq.Value) bool {
	vv := v.Int64()
	return int64(p) <= vv && vv <= int64(p)
}

func (p EqualInt64Predicate) KeepPage(page pq.Page) bool {
	if min, max, ok := page.Bounds(); ok {
		return int64(p) >= min.Int64() && int64(p) <= max.Int64()
	}
	return true
}

type InstrumentedPredicate struct {
	pred                  Predicate // Optional, if missing then just keeps metrics with no filtering
	InspectedColumnChunks atomic.Int64
	InspectedPages        atomic.Int64
	InspectedValues       atomic.Int64
	KeptColumnChunks      atomic.Int64
	KeptPages             atomic.Int64
	KeptValues            atomic.Int64
}

var _ Predicate = (*InstrumentedPredicate)(nil)

func (p *InstrumentedPredicate) KeepColumnChunk(c pq.ColumnChunk) bool {
	p.InspectedColumnChunks.Inc()

	if p.pred == nil || p.pred.KeepColumnChunk(c) {
		p.KeptColumnChunks.Inc()
		return true
	}

	return false
}

func (p *InstrumentedPredicate) KeepPage(page pq.Page) bool {
	p.InspectedPages.Inc()

	if p.pred == nil || p.pred.KeepPage(page) {
		p.KeptPages.Inc()
		return true
	}

	return false
}

func (p *InstrumentedPredicate) KeepValue(v pq.Value) bool {
	p.InspectedValues.Inc()

	if p.pred == nil || p.pred.KeepValue(v) {
		p.KeptValues.Inc()
		return true
	}

	return false
}

type mapPredicate[K constraints.Integer, V any] struct {
	inbetweenPred Predicate
	m             map[K]V
}

func NewMapPredicate[K constraints.Integer, V any](m map[K]V) Predicate {

	var min, max int64

	first := true
	for k := range m {
		if first || max < int64(k) {
			max = int64(k)
		}
		if first || min > int64(k) {
			min = int64(k)
		}
		first = false
	}

	return &mapPredicate[K, V]{
		inbetweenPred: NewIntBetweenPredicate(min, max),
		m:             m,
	}
}

func (m *mapPredicate[K, V]) KeepColumnChunk(c pq.ColumnChunk) bool {
	return m.inbetweenPred.KeepColumnChunk(c)
}

func (m *mapPredicate[K, V]) KeepPage(page pq.Page) bool {
	return m.inbetweenPred.KeepPage(page)
}

func (m *mapPredicate[K, V]) KeepValue(v pq.Value) bool {
	_, exists := m.m[K(v.Int64())]
	return exists
}
