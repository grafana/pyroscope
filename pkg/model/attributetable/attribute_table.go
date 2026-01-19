package attributetable

import (
	"unique"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

type attributeKey struct {
	key   unique.Handle[string]
	value unique.Handle[string]
}

// Table stores unique label keys and values for efficient serialization.
type Table struct {
	table   map[attributeKey]int64
	entries []attributeKey
}

// NewTable creates a new AttributeTable for string interning.
func NewTable() *Table {
	return &Table{
		table:   make(map[attributeKey]int64),
		entries: make([]attributeKey, 0),
	}
}

// LookupOrAdd returns the index for the given key-value pair.
func (t *Table) LookupOrAdd(k attributeKey) int64 {
	ref, exists := t.table[k]
	if !exists {
		ref = int64(len(t.entries))
		t.entries = append(t.entries, k)
		t.table[k] = ref
	}
	return ref
}

// AddKeyValue interns a key-value string pair and returns its ref.
func (t *Table) AddKeyValue(key, value string) int64 {
	return t.LookupOrAdd(attributeKey{
		key:   unique.Make(key),
		value: unique.Make(value),
	})
}

// Refs converts a set of labels to their attribute_refs indices.
func (t *Table) Refs(lbls []*typesv1.LabelPair, refs []int64) []int64 {
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

// Build converts the Table to its protobuf representation.
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
		res.Keys[idx] = e.key.Value()
		res.Values[idx] = e.value.Value()
	}

	return res
}

// Entries returns the internal entries for accessing stored key-value pairs.
func (t *Table) Entries() []attributeKey {
	return t.entries
}

// GetKeyValue returns the key and value strings for a given attribute ref.
func (t *Table) GetKeyValue(ref int64) (key, value string, ok bool) {
	if ref < 0 || ref >= int64(len(t.entries)) {
		return "", "", false
	}
	entry := t.entries[ref]
	return entry.key.Value(), entry.value.Value(), true
}
