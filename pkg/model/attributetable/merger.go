package attributetable

import (
	"fmt"
	"sync"
	"unique"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

// Merger provides generic merging functionality for series with attribute tables.
type Merger struct {
	mu    sync.Mutex
	table *Table
}

// NewMerger creates a new generic merger.
func NewMerger() *Merger {
	return &Merger{
		table: New(),
	}
}

type Remapper struct {
	m map[int64]int64
}

// Refs remaps a slice of attribute refs.
func (r *Remapper) Refs(refs []int64) []int64 {
	// if we are the first report, we don't have a m yet
	if r.m == nil {
		return refs
	}

	for i, ref := range refs {
		if newRef, ok := r.m[ref]; ok {
			refs[i] = newRef
		} else {
			panic(fmt.Sprintf("attribute ref %d not found in attribute table", ref))
		}
	}
	return refs
}

// Ref remaps a single attribute ref.
func (r *Remapper) Ref(ref int64) int64 {
	// if we are the first report, we don't have a m yet
	if r.m == nil {
		return ref
	}

	newRef, ok := r.m[ref]
	if !ok {
		panic(fmt.Sprintf("attribute ref %d not found in attribute table", ref))
	}
	return newRef
}

// Merge adds entries from the input attribute table to the merger's
// attribute table and returns a remapper, remapper only safe to use during callback
func (m *Merger) Merge(table *queryv1.AttributeTable, f func(*Remapper)) {
	m.mu.Lock()
	defer m.mu.Unlock()

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

	f(&Remapper{m: refMap})
}

// BuildAttributeTable builds the protobuf AttributeTable from the merger's table.
func (m *Merger) BuildAttributeTable(res *queryv1.AttributeTable) *queryv1.AttributeTable {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.table.Build(res)
}
