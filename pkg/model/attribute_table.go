package model

import (
	"unique"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

// attributeKey is a single key-value pair with interned strings.
type attributeKey struct {
	key   unique.Handle[string]
	value unique.Handle[string]
}

// AttributeTable stores unique label keys and values for efficient serialization.
type AttributeTable struct {
	table   map[attributeKey]int64
	entries []attributeKey
}

func NewAttributeTable() *AttributeTable {
	return &AttributeTable{
		table:   make(map[attributeKey]int64),
		entries: make([]attributeKey, 0),
	}
}

// LookupOrAdd returns the index for the given key-value pair.
// If the pair doesn't exist, it's added to the table.
func (t *AttributeTable) LookupOrAdd(k attributeKey) int64 {
	ref, exists := t.table[k]
	if !exists {
		ref = int64(len(t.entries))
		t.entries = append(t.entries, k)
		t.table[k] = ref
	}
	return ref
}

// Refs converts a set of labels to their attribute_refs indices.
func (t *AttributeTable) Refs(lbls Labels, refs []int64) []int64 {
	if cap(refs) < len(lbls) {
		refs = make([]int64, len(lbls))
	} else {
		refs = refs[:len(lbls)]
	}

	for i, lbl := range lbls {
		refs[i] = t.LookupOrAdd(attributeKey{
			key:   unique.Make(lbl.Name),
			value: unique.Make(lbl.Value),
		})
	}

	return refs
}

// Build converts the AttributeTable to its protobuf representation.
func (t *AttributeTable) Build(res *queryv1.AttributeTable) *queryv1.AttributeTable {
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
		res.Keys[idx] = e.key.Value()
		res.Values[idx] = e.value.Value()
	}

	return res
}
