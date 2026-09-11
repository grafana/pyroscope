package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/olekukonko/tablewriter"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

type queryFunctionsParams struct {
	*queryProfileParams
	Output string
}

func addQueryFunctionsParams(queryCmd commander) *queryFunctionsParams {
	params := &queryFunctionsParams{queryProfileParams: addQueryProfileParams(queryCmd)}
	queryCmd.Flag("profile-id", "Profile ID (UUID) to query a specific profile. Repeatable for multiple IDs. Use 'query exemplars profile' to find IDs.").StringsVar(&params.ProfileIDs)
	queryCmd.Flag("trace-id", "Trace ID (32 hex characters) to filter samples by. Repeatable for multiple traces.").StringsVar(&params.TraceIDs)
	queryCmd.Flag("output", "Output format, one of: table, json.").Default("table").EnumVar(&params.Output, "table", outputJSON)
	return params
}

func queryFunctions(ctx context.Context, params *queryFunctionsParams) error {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}
	if err := validateQueryProfileParams(params.queryProfileParams); err != nil {
		return err
	}
	profileType, err := model.ParseProfileTypeSelector(params.ProfileType)
	if err != nil {
		return fmt.Errorf("failed to parse profile type: %w", err)
	}
	level.Info(logger).Log(
		"msg", "querying function table",
		"url", params.URL, "from", from, "to", to, "query", params.Query,
		"type", params.ProfileType, "max_nodes", params.MaxNodes,
	)
	req := &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID:     params.ProfileType,
		LabelSelector:     params.Query,
		Start:             from.UnixMilli(),
		End:               to.UnixMilli(),
		Format:            querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS,
		SpanSelector:      params.SpanSelector,
		ProfileIdSelector: params.ProfileIDs,
		TraceIdSelector:   params.TraceIDs,
	}
	if params.MaxNodes != 0 {
		req.MaxNodes = &params.MaxNodes
	}
	if len(params.StacktraceSelector) > 0 {
		req.StackTraceSelector = new(typesv1.StackTraceSelector)
		for _, name := range params.StacktraceSelector {
			req.StackTraceSelector.CallSite = append(req.StackTraceSelector.CallSite, &typesv1.Location{Name: name})
		}
	}
	resp, err := params.queryClient().SelectMergeStacktraces(ctx, connect.NewRequest(req))
	if err != nil {
		return fmt.Errorf("failed to query functions: %w", err)
	}
	logDiagnostics(params.phlareClient, resp.Header())
	if resp.Msg.Functions == nil {
		return errors.New("server returned no function table; query functions requires a server with v2 function-table support")
	}
	return outputFunctions(ctx, resp.Msg.Functions, params.Output, from, to, profileType)
}

func outputFunctions(ctx context.Context, result *querierv1.FunctionTable, format string, from, to time.Time, profileType *typesv1.ProfileType) error {
	if format == outputJSON {
		type functionRow struct {
			Name  string `json:"name"`
			Self  int64  `json:"self"`
			Total int64  `json:"total"`
		}
		out := struct {
			From        time.Time     `json:"from"`
			To          time.Time     `json:"to"`
			ProfileType string        `json:"profile_type"`
			Total       int64         `json:"total"`
			Functions   []functionRow `json:"functions"`
		}{
			From: from, To: to, ProfileType: profileType.ID, Total: result.Total,
			Functions: make([]functionRow, len(result.Functions)),
		}
		for i, row := range result.Functions {
			out.Functions[i] = functionRow{Name: row.Name, Self: row.Self, Total: row.Total}
		}
		enc := json.NewEncoder(output(ctx))
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	w := output(ctx)
	if _, err := fmt.Fprintf(w, "Profile total (%s): %s\n", profileType.SampleUnit, formatUnit(float64(result.Total), profileType.SampleUnit)); err != nil {
		return err
	}
	table := newTableWriter(w)
	table.SetAutoWrapText(false)
	table.SetHeader([]string{"Rank", "Function", fmt.Sprintf("Self (%s)", profileType.SampleUnit), "Self %", fmt.Sprintf("Total (%s)", profileType.SampleUnit), "Total %"})
	table.SetColumnAlignment([]int{tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_LEFT, tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_RIGHT, tablewriter.ALIGN_RIGHT})
	for i, row := range result.Functions {
		var selfPercent, totalPercent float64
		if result.Total != 0 {
			selfPercent = 100 * float64(row.Self) / float64(result.Total)
			totalPercent = 100 * float64(row.Total) / float64(result.Total)
		}
		table.Append([]string{
			fmt.Sprint(i + 1), row.Name,
			formatUnit(float64(row.Self), profileType.SampleUnit), fmt.Sprintf("%.2f%%", selfPercent),
			formatUnit(float64(row.Total), profileType.SampleUnit), fmt.Sprintf("%.2f%%", totalPercent),
		})
	}
	table.Render()
	return nil
}
