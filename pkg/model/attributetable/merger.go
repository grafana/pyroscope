package attributetable

import (
	"fmt"
	"sync"
	"unique"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

// MergedSeries represents a merged series with generic point type.
type MergedSeries[T any] struct {
	AttributeRefs []int64
	Points        []T
}

// Merger provides generic merging functionality for series with attribute tables.
// T is the point type, K is the series key type.
type Merger[T any, K comparable] struct {
	mu     sync.Mutex
	table  *Table
	series map[K]*MergedSeries[T]
}

// NewMerger creates a new generic merger.
func NewMerger[T any, K comparable]() *Merger[T, K] {
	return &Merger[T, K]{
		table:  New(),
		series: make(map[K]*MergedSeries[T]),
	}
}

// RemapAttributeTable adds entries from the input attribute table to the merger's
// attribute table and returns a mapping from old refs to new refs.
func (m *Merger[T, K]) RemapAttributeTable(table *queryv1.AttributeTable) map[int64]int64 {
	// Keys and Values must have the same length - this is a data corruption bug
	if len(table.Keys) != len(table.Values) {
		panic(fmt.Sprintf("attribute table corruption: Keys length (%d) != Values length (%d)", len(table.Keys), len(table.Values)))
	}

	// only build the refMap if this is not the first report
	var refMap map[int64]int64
	if len(table.Keys) > 0 {
		refMap = make(map[int64]int64, len(table.Keys))
	}
	for i := range table.Keys {
		oldRef := int64(i)
		key := unique.Make(table.Keys[i])
		value := unique.Make(table.Values[i])
		newRef := m.table.LookupOrAdd(Key{Key: key, Value: value})
		if refMap != nil {
			refMap[oldRef] = newRef
		}
	}
	return refMap
}

// RemapRefs remaps a slice of attribute refs using the provided mapping.
// Returns the same slice with remapped values, or the original slice if refMap is nil.
func (m *Merger[T, K]) RemapRefs(refs []int64, refMap map[int64]int64) []int64 {
	// if we are the first report, we don't have a refMap yet
	if refMap == nil {
		return refs
	}

	for i, ref := range refs {
		if newRef, ok := refMap[ref]; ok {
			refs[i] = newRef
		} else {
			panic(fmt.Sprintf("attribute ref %d not found in attribute table", ref))
		}
	}
	return refs
}

// GetOrCreateSeries returns an existing series or creates a new one with the given key and attribute refs.
func (m *Merger[T, K]) GetOrCreateSeries(key K, attributeRefs []int64) *MergedSeries[T] {
	existing, ok := m.series[key]
	if !ok {
		m.series[key] = &MergedSeries[T]{
			AttributeRefs: attributeRefs,
			Points:        make([]T, 0),
		}
		return m.series[key]
	}
	return existing
}

// Table returns the underlying attribute table.
func (m *Merger[T, K]) Table() *Table {
	return m.table
}

// Series returns the map of merged series.
func (m *Merger[T, K]) Series() map[K]*MergedSeries[T] {
	return m.series
}

// Lock acquires the merger's mutex.
func (m *Merger[T, K]) Lock() {
	m.mu.Lock()
}

// Unlock releases the merger's mutex.
func (m *Merger[T, K]) Unlock() {
	m.mu.Unlock()
}

// IsEmpty returns true if no series have been merged.
func (m *Merger[T, K]) IsEmpty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.series) == 0
}

// BuildAttributeTable builds the protobuf AttributeTable from the merger's table.
func (m *Merger[T, K]) BuildAttributeTable(res *queryv1.AttributeTable) *queryv1.AttributeTable {
	return m.table.Build(res)
}
