package heatmap

import (
	"slices"
	"strings"
	"unique"

	prommodel "github.com/prometheus/common/model"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

type attributeKey struct {
	key   unique.Handle[string]
	value unique.Handle[string]
}

// This attribute table is used to store the attribute values for the heatmap. It might be reused for other queries at a later time.
type attributeTable struct {
	table   map[attributeKey]int64
	entries []attributeKey
}

func newAttributeTable() *attributeTable {
	return &attributeTable{
		table:   make(map[attributeKey]int64),
		entries: make([]attributeKey, 0),
	}
}

func (t *attributeTable) lookupOrAdd(k attributeKey) int64 {
	ref, exists := t.table[k]
	if !exists {
		ref = int64(len(t.entries))
		t.entries = append(t.entries, k)
		t.table[k] = ref
	}
	return ref

}

func (t *attributeTable) refs(lbls model.Labels, refs []int64) []int64 {
	if cap(refs) < len(lbls) {
		refs = make([]int64, len(lbls))
	} else {
		refs = refs[:len(lbls)]
	}
	for i, lbl := range lbls {
		refs[i] = t.lookupOrAdd(attributeKey{key: unique.Make(lbl.Name), value: unique.Make(lbl.Value)})
	}
	return refs
}

func (t *attributeTable) build(res *queryv1.AttributeTable) *queryv1.AttributeTable {
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
		res.Keys = res.Keys[:len(t.entries)]
	}

	for idx, e := range t.entries {
		res.Keys[idx] = e.key.Value()
		res.Values[idx] = e.value.Value()
	}

	return res
}

type heatmapSeries struct {
	labels        model.Labels
	pointsBuilder *pointsBuilder
}

type labelKey string

type Builder struct {
	attributeTable *attributeTable

	labelBuf []byte

	series map[labelKey]*heatmapSeries // by series

	by []string
}

func NewBuilder(by []string) *Builder {
	return &Builder{
		attributeTable: newAttributeTable(),
		series:         make(map[labelKey]*heatmapSeries),
		by:             by,
	}
}

func (h *Builder) newSeries(labels model.Labels) *heatmapSeries {
	s := &heatmapSeries{
		labels:        labels.WithLabels(h.by...),
		pointsBuilder: newPointsBuilder(),
	}
	return s
}

func (h *Builder) Add(fp prommodel.Fingerprint, labels model.Labels, ts int64, profileID string, spanID uint64, value uint64) {
	// TODO: Support annotations
	if profileID == "" && spanID == 0 {
		return
	}

	h.labelBuf = labels.BytesWithLabels(h.labelBuf, h.by...)
	seriesKey := labelKey(h.labelBuf)

	series, exists := h.series[seriesKey]
	if !exists {
		series = h.newSeries(labels)
		h.series[seriesKey] = series
	}

	series.pointsBuilder.add(fp, labels, ts, profileID, spanID, value)
}

func (h *Builder) Build(report *queryv1.HeatmapReport) *queryv1.HeatmapReport {
	if report == nil {
		report = &queryv1.HeatmapReport{}
	}
	at := newAttributeTable()

	var keys []labelKey
	for k := range h.series {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b labelKey) int {
		return strings.Compare(string(a), string(b))
	})
	for _, k := range keys {
		series := h.series[k]

		r := &queryv1.HeatmapSeries{}
		r.AttributeRefs = at.refs(series.labels, r.AttributeRefs)

		points := make([]queryv1.HeatmapPoint, series.pointsBuilder.count())
		r.Points = make([]*queryv1.HeatmapPoint, 0, series.pointsBuilder.count())
		idx := 0
		series.pointsBuilder.forEach(func(labels model.Labels, ts int64, profileID string, spanID uint64, value uint64) {
			p := &points[idx]
			p.AttributeRefs = at.refs(labels, p.AttributeRefs)
			p.Timestamp = ts
			p.ProfileId = at.lookupOrAdd(attributeKey{key: unique.Make(""), value: unique.Make(profileID)})
			p.SpanId = spanID
			p.Value = value

			r.Points = append(r.Points, p)
			idx += 1
		})

		report.HeatmapSeries = append(report.HeatmapSeries, r)
	}

	report.AttributeTable = at.build(report.AttributeTable)

	return report
}
