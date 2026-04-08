package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

type queryExemplarsParams struct {
	*queryParams
	ProfileType     string
	Output          string
	TopN            uint64
	MaxLabelColumns int
}

func addQueryExemplarsParams(queryCmd commander) *queryExemplarsParams {
	params := new(queryExemplarsParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("output", "Output format, one of: table, json.").Default("table").StringVar(&params.Output)
	queryCmd.Flag("top-n", "Maximum number of exemplars to show.").Default("100").Uint64Var(&params.TopN)
	queryCmd.Flag("max-label-columns", "Maximum number of label columns to show in table output. Set to 0 to hide labels.").Default("3").IntVar(&params.MaxLabelColumns)
	return params
}

// exemplarEntry is a flattened representation of a single exemplar extracted
// from a SelectSeries response point.
type exemplarEntry struct {
	ProfileID string
	Timestamp time.Time
	Value     int64
	SpanID    string
	Labels    map[string]string
}

func queryExemplars(ctx context.Context, params *queryExemplarsParams) error {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	level.Info(logger).Log(
		"msg", "querying exemplars",
		"url", params.URL,
		"from", from,
		"to", to,
		"query", params.Query,
		"type", params.ProfileType,
		"top_n", params.TopN,
	)

	// Calculate step: divide time range into approximately topN buckets so we
	// get roughly one exemplar per bucket (DefaultMaxExemplarsPerPoint = 1).
	rangeSeconds := to.Sub(from).Seconds()
	stepSeconds := rangeSeconds / float64(params.TopN)
	if stepSeconds < 1 {
		stepSeconds = 1
	}

	qc := params.queryClient()
	resp, err := qc.SelectSeries(ctx, connect.NewRequest(&querierv1.SelectSeriesRequest{
		ProfileTypeID: params.ProfileType,
		LabelSelector: params.Query,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		Step:          stepSeconds,
		ExemplarType:  typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL,
	}))
	if err != nil {
		return errors.Wrap(err, "failed to query exemplars")
	}

	logDiagnostics(params.phlareClient, resp.Header())

	// Extract exemplars from all series points into a flat slice.
	// Pre-count to avoid repeated slice growth.
	var totalExemplars int
	for _, s := range resp.Msg.Series {
		for _, p := range s.Points {
			totalExemplars += len(p.Exemplars)
		}
	}
	entries := make([]exemplarEntry, 0, totalExemplars)
	for _, s := range resp.Msg.Series {
		// Build series-level labels map for context.
		seriesLabels := make(map[string]string, len(s.Labels))
		for _, lp := range s.Labels {
			seriesLabels[lp.Name] = lp.Value
		}

		for _, p := range s.Points {
			for _, ex := range p.Exemplars {
				lbls := make(map[string]string, len(seriesLabels)+len(ex.Labels))
				for k, v := range seriesLabels {
					lbls[k] = v
				}
				for _, lp := range ex.Labels {
					lbls[lp.Name] = lp.Value
				}
				entries = append(entries, exemplarEntry{
					ProfileID: ex.ProfileId,
					Timestamp: time.UnixMilli(ex.Timestamp),
					Value:     ex.Value,
					SpanID:    ex.SpanId,
					Labels:    lbls,
				})
			}
		}
	}

	// Sort by value descending (highest value = most interesting).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Value > entries[j].Value
	})

	if uint64(len(entries)) > params.TopN {
		entries = entries[:params.TopN]
	}

	if len(entries) == 0 {
		level.Info(logger).Log("msg", "no exemplars found")
	}

	profileType, err := model.ParseProfileTypeSelector(params.ProfileType)
	if err != nil {
		return errors.Wrap(err, "failed to parse profile type")
	}

	// Auto-detect the highest-cardinality labels for table columns
	// (most distinct values = most differentiating, capped by --max-label-columns).
	tableLabels := topCardinalityLabels(entries, params.MaxLabelColumns)

	switch params.Output {
	case outputJSON:
		return outputExemplarsJSON(ctx, entries, from, to, params.ProfileType)
	default:
		return outputExemplarsTable(ctx, entries, profileType.SampleUnit, tableLabels)
	}
}

// topCardinalityLabels returns up to N label names to show as table columns,
// excluding internal labels. It prefers labels with higher cardinality (more
// distinct values = more differentiating). If no high-cardinality labels exist
// (e.g. single-service data), it falls back to the first N labels alphabetically.
func topCardinalityLabels(entries []exemplarEntry, n int) []string {
	if len(entries) == 0 || n <= 0 {
		return nil
	}

	// Count distinct values per label name.
	distinctValues := make(map[string]map[string]struct{})
	for _, e := range entries {
		for k, v := range e.Labels {
			if isInternalLabel(k) {
				continue
			}
			if distinctValues[k] == nil {
				distinctValues[k] = make(map[string]struct{})
			}
			distinctValues[k][v] = struct{}{}
		}
	}

	type labelCardinality struct {
		name        string
		cardinality int
	}
	var candidates []labelCardinality
	for name, vals := range distinctValues {
		candidates = append(candidates, labelCardinality{name, len(vals)})
	}

	// Sort by cardinality descending, then alphabetically for ties.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].cardinality != candidates[j].cardinality {
			return candidates[i].cardinality > candidates[j].cardinality
		}
		return candidates[i].name < candidates[j].name
	})

	if len(candidates) > n {
		candidates = candidates[:n]
	}

	result := make([]string, len(candidates))
	for i, c := range candidates {
		result[i] = c.name
	}
	return result
}

// isInternalLabel returns true for labels that are internal metadata
// (e.g. __name__, __period_type__) and should be hidden from user output.
func isInternalLabel(name string) bool {
	return strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__")
}

// filterLabels returns a copy of labels with internal labels removed.
func filterLabels(labels map[string]string) map[string]string {
	filtered := make(map[string]string, len(labels))
	for k, v := range labels {
		if !isInternalLabel(k) {
			filtered[k] = v
		}
	}
	return filtered
}

func outputExemplarsJSON(ctx context.Context, entries []exemplarEntry, from, to time.Time, profileType string) error {
	type jsonExemplar struct {
		ProfileID string            `json:"profile_id"`
		Timestamp time.Time         `json:"timestamp"`
		Value     int64             `json:"value"`
		SpanID    string            `json:"span_id,omitempty"`
		Labels    map[string]string `json:"labels,omitempty"`
	}
	type jsonOutput struct {
		From        time.Time      `json:"from"`
		To          time.Time      `json:"to"`
		ProfileType string         `json:"profile_type"`
		Exemplars   []jsonExemplar `json:"exemplars"`
	}

	out := jsonOutput{
		From:        from,
		To:          to,
		ProfileType: profileType,
		Exemplars:   make([]jsonExemplar, len(entries)),
	}
	for i, e := range entries {
		out.Exemplars[i] = jsonExemplar{
			ProfileID: e.ProfileID,
			Timestamp: e.Timestamp,
			Value:     e.Value,
			SpanID:    e.SpanID,
			Labels:    filterLabels(e.Labels),
		}
	}

	enc := json.NewEncoder(output(ctx))
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// querySpanExemplars lists span exemplars by calling SelectHeatmap with
// HEATMAP_QUERY_TYPE_SPAN, which is already fully implemented in the backend.
// The heatmap slots each carry at most one exemplar (the highest-value span in
// that time×value bucket), so iterating all slots gives a representative sample
// of the most expensive spans in the requested window.
func querySpanExemplars(ctx context.Context, params *queryExemplarsParams) error {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	level.Info(logger).Log(
		"msg", "querying span exemplars",
		"url", params.URL,
		"from", from,
		"to", to,
		"query", params.Query,
		"type", params.ProfileType,
		"top_n", params.TopN,
	)

	rangeSeconds := to.Sub(from).Seconds()
	stepSeconds := rangeSeconds / float64(params.TopN)
	if stepSeconds < 1 {
		stepSeconds = 1
	}

	limit := int64(params.TopN)
	qc := params.queryClient()
	resp, err := qc.SelectHeatmap(ctx, connect.NewRequest(&querierv1.SelectHeatmapRequest{
		ProfileTypeID: params.ProfileType,
		LabelSelector: params.Query,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		Step:          stepSeconds,
		QueryType:     querierv1.HeatmapQueryType_HEATMAP_QUERY_TYPE_SPAN,
		ExemplarType:  typesv1.ExemplarType_EXEMPLAR_TYPE_SPAN,
		Limit:         &limit,
	}))
	if err != nil {
		return errors.Wrap(err, "failed to query span exemplars")
	}

	logDiagnostics(params.phlareClient, resp.Header())

	var entries []exemplarEntry
	for _, series := range resp.Msg.Series {
		seriesLabels := make(map[string]string, len(series.Labels))
		for _, lp := range series.Labels {
			seriesLabels[lp.Name] = lp.Value
		}
		for _, slot := range series.Slots {
			for _, ex := range slot.Exemplars {
				if ex.SpanId == "" {
					continue
				}
				lbls := make(map[string]string, len(seriesLabels)+len(ex.Labels))
				for k, v := range seriesLabels {
					lbls[k] = v
				}
				for _, lp := range ex.Labels {
					lbls[lp.Name] = lp.Value
				}
				entries = append(entries, exemplarEntry{
					SpanID:    ex.SpanId,
					Timestamp: time.UnixMilli(ex.Timestamp),
					Value:     ex.Value,
					Labels:    lbls,
				})
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Value > entries[j].Value
	})
	if uint64(len(entries)) > params.TopN {
		entries = entries[:params.TopN]
	}

	if len(entries) == 0 {
		level.Info(logger).Log("msg", "no span exemplars found")
	}

	profileType, err := model.ParseProfileTypeSelector(params.ProfileType)
	if err != nil {
		return errors.Wrap(err, "failed to parse profile type")
	}

	tableLabels := topCardinalityLabels(entries, params.MaxLabelColumns)

	switch params.Output {
	case outputJSON:
		return outputSpanExemplarsJSON(ctx, entries, from, to, params.ProfileType)
	default:
		return outputSpanExemplarsTable(ctx, entries, profileType.SampleUnit, tableLabels)
	}
}

func outputSpanExemplarsJSON(ctx context.Context, entries []exemplarEntry, from, to time.Time, profileType string) error {
	type jsonExemplar struct {
		SpanID    string            `json:"span_id"`
		Timestamp time.Time         `json:"timestamp"`
		Value     int64             `json:"value"`
		Labels    map[string]string `json:"labels,omitempty"`
	}
	type jsonOutput struct {
		From        time.Time      `json:"from"`
		To          time.Time      `json:"to"`
		ProfileType string         `json:"profile_type"`
		Exemplars   []jsonExemplar `json:"exemplars"`
	}

	out := jsonOutput{
		From:        from,
		To:          to,
		ProfileType: profileType,
		Exemplars:   make([]jsonExemplar, len(entries)),
	}
	for i, e := range entries {
		out.Exemplars[i] = jsonExemplar{
			SpanID:    e.SpanID,
			Timestamp: e.Timestamp,
			Value:     e.Value,
			Labels:    filterLabels(e.Labels),
		}
	}
	enc := json.NewEncoder(output(ctx))
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputSpanExemplarsTable(ctx context.Context, entries []exemplarEntry, sampleUnit string, labelColumns []string) error {
	headers := []string{"Span ID", "Timestamp", fmt.Sprintf("Value (%s)", sampleUnit)}
	aligns := []int{tablewriter.ALIGN_LEFT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_RIGHT}
	for _, name := range labelColumns {
		headers = append(headers, name)
		aligns = append(aligns, tablewriter.ALIGN_LEFT)
	}

	table := newTableWriter(output(ctx))
	table.SetHeader(headers)
	table.SetColumnAlignment(aligns)

	for _, e := range entries {
		row := []string{e.SpanID, e.Timestamp.Format(time.RFC3339), formatUnit(float64(e.Value), sampleUnit)}
		for _, name := range labelColumns {
			row = append(row, e.Labels[name])
		}
		table.Append(row)
	}
	table.Render()
	return nil
}

func outputExemplarsTable(ctx context.Context, entries []exemplarEntry, sampleUnit string, labelColumns []string) error {
	headers := []string{"Profile ID", "Timestamp", fmt.Sprintf("Value (%s)", sampleUnit)}
	aligns := []int{
		tablewriter.ALIGN_LEFT,
		tablewriter.ALIGN_LEFT,
		tablewriter.ALIGN_RIGHT,
	}

	// Only add Span ID column if any entry has one.
	hasSpanID := false
	for _, e := range entries {
		if e.SpanID != "" {
			hasSpanID = true
			break
		}
	}
	if hasSpanID {
		headers = append(headers, "Span ID")
		aligns = append(aligns, tablewriter.ALIGN_LEFT)
	}

	// Show auto-detected label columns.
	for _, name := range labelColumns {
		headers = append(headers, name)
		aligns = append(aligns, tablewriter.ALIGN_LEFT)
	}

	table := newTableWriter(output(ctx))
	table.SetHeader(headers)
	table.SetColumnAlignment(aligns)

	for _, e := range entries {
		row := []string{
			e.ProfileID,
			e.Timestamp.Format(time.RFC3339),
			formatUnit(float64(e.Value), sampleUnit),
		}
		if hasSpanID {
			row = append(row, e.SpanID)
		}
		for _, name := range labelColumns {
			row = append(row, e.Labels[name])
		}
		table.Append(row)
	}
	table.Render()
	return nil
}
