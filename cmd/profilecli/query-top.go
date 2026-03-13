package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"connectrpc.com/connect"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log/level"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

type queryTopParams struct {
	*queryParams
	ProfileType string
	TopN        uint64
	LabelNames  []string
	Output      string
}

func addQueryTopParams(queryCmd commander) *queryTopParams {
	params := new(queryTopParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("top-n", "Number of top results to show.").Default("10").Uint64Var(&params.TopN)
	queryCmd.Flag("label-names", "Label name(s) to group by. Can be specified multiple times.").Default(model.LabelNameServiceName).StringsVar(&params.LabelNames)
	queryCmd.Flag("output", "Output format, one of: table, json.").Default("table").StringVar(&params.Output)
	return params
}

func queryTop(ctx context.Context, params *queryTopParams) error {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	level.Info(logger).Log(
		"msg", "querying top series",
		"url", params.URL,
		"from", from,
		"to", to,
		"query", params.Query,
		"type", params.ProfileType,
		"labels", fmt.Sprintf("%v", params.LabelNames),
		"top_n", params.TopN,
	)

	stepSeconds := to.Sub(from).Seconds()

	qc := params.queryClient()
	resp, err := qc.SelectSeries(ctx, connect.NewRequest(&querierv1.SelectSeriesRequest{
		ProfileTypeID: params.ProfileType,
		LabelSelector: params.Query,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		Step:          stepSeconds,
		GroupBy:       params.LabelNames,
	}))
	if err != nil {
		return errors.Wrap(err, "failed to query series")
	}

	logDiagnostics(params.phlareClient, resp.Header())

	type seriesTotal struct {
		labelValues []string
		total       float64
	}

	totals := make([]seriesTotal, 0, len(resp.Msg.Series))
	for _, s := range resp.Msg.Series {
		var total float64
		for _, p := range s.Points {
			total += p.Value
		}
		lbls := model.Labels(s.Labels)
		vals := make([]string, len(params.LabelNames))
		for i, name := range params.LabelNames {
			if v := lbls.Get(name); v != "" {
				vals[i] = v
			} else {
				vals[i] = "<unknown>"
			}
		}
		totals = append(totals, seriesTotal{labelValues: vals, total: total})
	}

	sort.Slice(totals, func(i, j int) bool {
		return totals[i].total > totals[j].total
	})

	if uint64(len(totals)) > params.TopN {
		totals = totals[:params.TopN]
	}

	profileType, err := model.ParseProfileTypeSelector(params.ProfileType)
	if err != nil {
		return errors.Wrap(err, "failed to parse profile type")
	}

	switch params.Output {
	case "json":
		type jsonSeries struct {
			Labels map[string]string `json:"labels"`
			Total  float64           `json:"total"`
		}
		type jsonOutput struct {
			From        time.Time    `json:"from"`
			To          time.Time    `json:"to"`
			ProfileType string       `json:"profile_type"`
			Series      []jsonSeries `json:"series"`
		}
		out := jsonOutput{
			From:        from,
			To:          to,
			ProfileType: params.ProfileType,
			Series:      make([]jsonSeries, len(totals)),
		}
		for i, t := range totals {
			lbls := make(map[string]string, len(params.LabelNames))
			for j, name := range params.LabelNames {
				lbls[name] = t.labelValues[j]
			}
			out.Series[i] = jsonSeries{Labels: lbls, Total: t.total}
		}
		enc := json.NewEncoder(output(ctx))
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return err
		}
	default:
		headers := append([]string{"Rank"}, params.LabelNames...)
		headers = append(headers, fmt.Sprintf("Total (%s)", profileType.SampleUnit))
		aligns := make([]int, len(headers))
		aligns[0] = tablewriter.ALIGN_RIGHT
		for i := 1; i < len(headers)-1; i++ {
			aligns[i] = tablewriter.ALIGN_LEFT
		}
		aligns[len(aligns)-1] = tablewriter.ALIGN_RIGHT

		table := tablewriter.NewWriter(output(ctx))
		table.SetHeader(headers)
		table.SetColumnAlignment(aligns)
		for i, t := range totals {
			row := []string{fmt.Sprintf("%d", i+1)}
			row = append(row, t.labelValues...)
			row = append(row, formatUnit(t.total, profileType.SampleUnit))
			table.Append(row)
		}
		table.Render()
	}

	return nil
}

func formatUnit(v float64, unit string) string {
	switch unit {
	case "nanoseconds":
		return time.Duration(int64(v)).String()
	case "bytes":
		return humanize.Bytes(uint64(v))
	default:
		return humanize.FormatFloat("#,###.##", v)
	}
}
