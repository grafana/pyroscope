package attributetable

import (
	"encoding/binary"
	"encoding/hex"
	"unique"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
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

// ResolveLabelPairs converts attribute references to label pairs.
func ResolveLabelPairs(refs []int64, table *queryv1.AttributeTable) []*typesv1.LabelPair {
	if table == nil || len(refs) == 0 {
		return nil
	}
	labels := make([]*typesv1.LabelPair, 0, len(refs))
	for _, ref := range refs {
		if ref < 0 || ref >= int64(len(table.Keys)) {
			continue
		}
		labels = append(labels, &typesv1.LabelPair{
			Name:  table.Keys[ref],
			Value: table.Values[ref],
		})
	}
	return labels
}

// ResolveAnnotations converts attribute references to profile annotations.
func ResolveAnnotations(refs []int64, table *queryv1.AttributeTable) []*typesv1.ProfileAnnotation {
	if table == nil || len(refs) == 0 {
		return nil
	}
	annotations := make([]*typesv1.ProfileAnnotation, 0, len(refs))
	for _, ref := range refs {
		if ref < 0 || ref >= int64(len(table.Keys)) {
			continue
		}
		annotations = append(annotations, &typesv1.ProfileAnnotation{
			Key:   table.Keys[ref],
			Value: table.Values[ref],
		})
	}
	return annotations
}

// SpanIDToHex converts a uint64 span ID to a hex string
func SpanIDToHex(spanID uint64) string {
	if spanID == 0 {
		return ""
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, spanID)
	return hex.EncodeToString(b)
}

// resolveExemplar is the shared implementation for converting to a public Exemplar.
// ProfileId and SpanId must already be resolved to strings by the caller.
func resolveExemplar(timestamp, value int64, profileID, spanID string, attributeRefs []int64, table *queryv1.AttributeTable) *typesv1.Exemplar {
	labels := ResolveLabelPairs(attributeRefs, table)
	if profileID == "" && spanID == "" && len(labels) == 0 {
		return nil
	}
	return &typesv1.Exemplar{
		Timestamp: timestamp,
		ProfileId: profileID,
		SpanId:    spanID,
		Value:     value,
		Labels:    labels,
	}
}

// ResolveExemplars converts compact exemplars to public exemplars by resolving attribute refs.
func ResolveExemplars(exemplars []*queryv1.Exemplar, table *queryv1.AttributeTable) []*typesv1.Exemplar {
	if len(exemplars) == 0 {
		return nil
	}
	result := make([]*typesv1.Exemplar, 0, len(exemplars))
	for _, ex := range exemplars {
		if e := resolveExemplar(ex.Timestamp, ex.Value, ex.ProfileId, ex.SpanId, ex.AttributeRefs, table); e != nil {
			result = append(result, e)
		}
	}
	return result
}

// ResolveHeatmapExemplar converts a HeatmapPoint to a public Exemplar by resolving
// attribute refs, profile ID (table lookup), and span ID (hex encoding).
func ResolveHeatmapExemplar(point *queryv1.HeatmapPoint, table *queryv1.AttributeTable) *typesv1.Exemplar {
	if point == nil || table == nil {
		return nil
	}

	profileID := ""
	if point.ProfileId >= 0 && point.ProfileId < int64(len(table.Values)) {
		profileID = table.Values[point.ProfileId]
	}

	return resolveExemplar(point.Timestamp, point.Value, profileID, SpanIDToHex(point.SpanId), point.AttributeRefs, table)
}
