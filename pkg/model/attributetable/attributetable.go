package attributetable

import (
	"unique"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

type Key struct {
	Key   unique.Handle[string]
	Value unique.Handle[string]
}

// Table is used to store attribute key-value pairs for efficient lookup and deduplication.
// It maps attribute keys to integer references, allowing compact representation of labels.
type Table struct {
	table   map[Key]int64
	entries []Key
}

func New() *Table {
	return &Table{
		table: make(map[Key]int64),
	}
}

func (t *Table) LookupOrAdd(k Key) int64 {
	ref, exists := t.table[k]
	if !exists {
		ref = int64(len(t.entries))
		t.entries = append(t.entries, k)
		t.table[k] = ref
	}
	return ref
}

func (t *Table) Refs(lbls model.Labels, refs []int64) []int64 {
	if cap(refs) < len(lbls) {
		refs = make([]int64, len(lbls))
	} else {
		refs = refs[:len(lbls)]
	}
	for i, lbl := range lbls {
		refs[i] = t.LookupOrAdd(Key{Key: unique.Make(lbl.Name), Value: unique.Make(lbl.Value)})
	}
	return refs
}

func (t *Table) AnnotationRefs(keys, values []string, refs []int64) []int64 {
	if cap(refs) < len(keys) {
		refs = make([]int64, len(keys))
	} else {
		refs = refs[:len(keys)]
	}
	for i := range keys {
		refs[i] = t.LookupOrAdd(Key{Key: unique.Make(keys[i]), Value: unique.Make(values[i])})
	}
	return refs
}

func (t *Table) Build(res *queryv1.AttributeTable) *queryv1.AttributeTable {
	if res == nil {
		res = &queryv1.AttributeTable{}
	}

	if cap(res.Keys) < len(t.entries) {
		res.Keys = make([]string, len(t.entries))
	} else {
		res.Keys = res.Keys[:len(t.entries)]
	}
	if cap(res.Values) < len(t.entries) {
		res.Values = make([]string, len(t.entries))
	} else {
		res.Values = res.Values[:len(t.entries)]
	}

	for idx, e := range t.entries {
		res.Keys[idx] = e.Key.Value()
		res.Values[idx] = e.Value.Value()
	}

	return res
}
