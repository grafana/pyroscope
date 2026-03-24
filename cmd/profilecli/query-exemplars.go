package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
	ProfileType string
	LabelNames  []string
	Output      string
	TopN        uint64
}

func addQueryExemplarsParams(queryCmd commander) *queryExemplarsParams {
	params := new(queryExemplarsParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("label-names", "Label name(s) to group by. Can be specified multiple times.").StringsVar(&params.LabelNames)
	queryCmd.Flag("output", "Output format, one of: table, json.").Default("table").StringVar(&params.Output)
	queryCmd.Flag("top-n", "Maximum number of exemplars to show.").Default("100").Uint64Var(&params.TopN)
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
		GroupBy:       params.LabelNames,
		ExemplarType:  typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL,
	}))
	if err != nil {
		return errors.Wrap(err, "failed to query exemplars")
	}

	logDiagnostics(params.phlareClient, resp.Header())

	// Extract exemplars from all series points into a flat slice.
	var entries []exemplarEntry
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
		return nil
	}

	profileType, err := model.ParseProfileTypeSelector(params.ProfileType)
	if err != nil {
		return errors.Wrap(err, "failed to parse profile type")
	}

	switch params.Output {
	case "json":
		return outputExemplarsJSON(ctx, entries, from, to, params.ProfileType)
	default:
		return outputExemplarsTable(ctx, entries, profileType.SampleUnit)
	}
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
			Labels:    e.Labels,
		}
	}

	enc := json.NewEncoder(output(ctx))
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputExemplarsTable(ctx context.Context, entries []exemplarEntry, sampleUnit string) error {
	headers := []string{"Rank", "Profile ID", "Timestamp", fmt.Sprintf("Value (%s)", sampleUnit), "Span ID"}
	aligns := []int{
		tablewriter.ALIGN_RIGHT,
		tablewriter.ALIGN_LEFT,
		tablewriter.ALIGN_LEFT,
		tablewriter.ALIGN_RIGHT,
		tablewriter.ALIGN_LEFT,
	}

	// Collect all unique label names across entries for additional columns.
	labelNameSet := make(map[string]struct{})
	for _, e := range entries {
		for k := range e.Labels {
			labelNameSet[k] = struct{}{}
		}
	}
	var labelNames []string
	for k := range labelNameSet {
		labelNames = append(labelNames, k)
	}
	sort.Strings(labelNames)

	for _, name := range labelNames {
		headers = append(headers, name)
		aligns = append(aligns, tablewriter.ALIGN_LEFT)
	}

	table := tablewriter.NewWriter(output(ctx))
	table.SetHeader(headers)
	table.SetColumnAlignment(aligns)

	for i, e := range entries {
		row := []string{
			fmt.Sprintf("%d", i+1),
			e.ProfileID,
			e.Timestamp.Format(time.RFC3339),
			formatUnit(float64(e.Value), sampleUnit),
			e.SpanID,
		}
		for _, name := range labelNames {
			row = append(row, e.Labels[name])
		}
		table.Append(row)
	}
	table.Render()
	return nil
}
