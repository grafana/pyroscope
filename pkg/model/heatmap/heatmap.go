package heatmap

import (
	"slices"
	"strings"
	"unique"

	prommodel "github.com/prometheus/common/model"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/model/attributetable"
)

type heatmapSeries struct {
	labels        model.Labels
	pointsBuilder *pointsBuilder
}

type labelKey string

type Builder struct {
	labelBuf []byte

	series map[labelKey]*heatmapSeries // by series

	by []string
}

func NewBuilder(by []string) *Builder {
	return &Builder{
		series: make(map[labelKey]*heatmapSeries),
		by:     by,
	}
}

func (h *Builder) newSeries(labels model.Labels) *heatmapSeries {
	s := &heatmapSeries{
		labels:        labels.WithLabels(h.by...),
		pointsBuilder: newPointsBuilder(),
	}
	return s
}

func (h *Builder) Add(fp prommodel.Fingerprint, labels model.Labels, ts int64, profileID string, spanID uint64, value int64) {
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

	pointLabels := labels.WithoutLabels(h.by...)
	series.pointsBuilder.add(fp, pointLabels, ts, profileID, spanID, value)
}

func (h *Builder) Build(report *queryv1.HeatmapReport) *queryv1.HeatmapReport {
	if report == nil {
		report = &queryv1.HeatmapReport{}
	}
	at := attributetable.New()

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
		r.AttributeRefs = at.Refs(series.labels, r.AttributeRefs)

		points := make([]queryv1.HeatmapPoint, series.pointsBuilder.count())
		r.Points = make([]*queryv1.HeatmapPoint, 0, series.pointsBuilder.count())
		idx := 0
		series.pointsBuilder.forEach(func(labels model.Labels, ts int64, profileID string, spanID uint64, value int64) {
			p := &points[idx]
			p.AttributeRefs = at.Refs(labels, p.AttributeRefs)
			p.Timestamp = ts
			p.ProfileId = at.LookupOrAdd(attributetable.Key{Key: unique.Make(""), Value: unique.Make(profileID)})
			p.SpanId = spanID
			p.Value = value

			r.Points = append(r.Points, p)
			idx += 1
		})

		report.HeatmapSeries = append(report.HeatmapSeries, r)
	}

	report.AttributeTable = at.Build(report.AttributeTable)

	return report
}
