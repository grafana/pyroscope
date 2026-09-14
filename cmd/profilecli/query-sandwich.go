package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

type querySandwichParams struct {
	*queryProfileParams
	Function string
	Depth    int
	Output   string
}

func addQuerySandwichParams(queryCmd commander) *querySandwichParams {
	params := &querySandwichParams{queryProfileParams: addQueryProfileParams(queryCmd)}
	queryCmd.Flag("function", "Function to build the sandwich around.").Required().StringVar(&params.Function)
	queryCmd.Flag("depth", "Levels of callers and callees to print. Zero prints every level.").Default("5").IntVar(&params.Depth)
	queryCmd.Flag("profile-id", "Profile ID (UUID) to query a specific profile. Repeatable for multiple IDs.").StringsVar(&params.ProfileIDs)
	queryCmd.Flag("trace-id", "Trace ID (32 hex characters) to filter samples by. Repeatable for multiple traces.").StringsVar(&params.TraceIDs)
	queryCmd.Flag("output", "Output format, one of: tree, json.").Default("tree").EnumVar(&params.Output, "tree", outputJSON)
	return params
}

func querySandwich(ctx context.Context, params *querySandwichParams) error {
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
		"msg", "querying sandwich",
		"url", params.URL, "from", from, "to", to, "query", params.Query,
		"type", params.ProfileType, "function", params.Function, "max_nodes", params.MaxNodes,
	)
	req := &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID:     params.ProfileType,
		LabelSelector:     params.Query,
		Start:             from.UnixMilli(),
		End:               to.UnixMilli(),
		Format:            querierv1.ProfileFormat_PROFILE_FORMAT_SANDWICH,
		SandwichFunction:  &params.Function,
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
		return fmt.Errorf("failed to query sandwich: %w", err)
	}
	logDiagnostics(params.phlareClient, resp.Header())
	if resp.Msg.Sandwich == nil {
		return errors.New("server returned no sandwich report; query sandwich requires a server with v2 sandwich support")
	}
	return outputSandwich(ctx, resp.Msg.Sandwich, params, from, to, profileType)
}

func outputSandwich(ctx context.Context, r *querierv1.SandwichReport, params *querySandwichParams, from, to time.Time, profileType *typesv1.ProfileType) error {
	if params.Output == outputJSON {
		type node struct {
			Name      string  `json:"name"`
			Total     int64   `json:"total"`
			Self      int64   `json:"self"`
			Truncated bool    `json:"truncated,omitempty"`
			Children  []*node `json:"children,omitempty"`
		}
		var conv func(n *querierv1.SandwichNode, depth int) *node
		conv = func(n *querierv1.SandwichNode, depth int) *node {
			if n == nil {
				return nil
			}
			out := &node{Name: sandwichName(r, n), Total: n.Total, Self: n.Self, Truncated: n.Truncated}
			if params.Depth != 0 && depth >= params.Depth {
				return out
			}
			for _, c := range n.Children {
				out.Children = append(out.Children, conv(c, depth+1))
			}
			return out
		}
		out := struct {
			From        time.Time `json:"from"`
			To          time.Time `json:"to"`
			ProfileType string    `json:"profile_type"`
			Function    string    `json:"function"`
			Total       int64     `json:"total"`
			Self        int64     `json:"self"`
			Callers     *node     `json:"callers"`
			Callees     *node     `json:"callees"`
		}{
			From: from, To: to, ProfileType: profileType.ID, Function: params.Function,
			Total: r.Total, Self: r.Self,
			Callers: conv(r.Callers, 0), Callees: conv(r.Callees, 0),
		}
		enc := json.NewEncoder(output(ctx))
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	w := output(ctx)
	unit := profileType.SampleUnit
	if _, err := fmt.Fprintf(w, "Function: %s\n", params.Function); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "In function (%s): %s total, %s self\n\n", unit,
		formatUnit(float64(r.Total), unit), formatUnit(float64(r.Self), unit)); err != nil {
		return err
	}
	// The callers half is rooted at the function, so printing it as a tree reads
	// outwards towards main rather than downwards into the callees.
	for _, half := range []struct {
		title string
		node  *querierv1.SandwichNode
	}{{"Callers (towards main)", r.Callers}, {"Callees", r.Callees}} {
		if _, err := fmt.Fprintf(w, "%s\n", half.title); err != nil {
			return err
		}
		if err := printSandwichNode(w, r, half.node, unit, 0, params.Depth); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

func sandwichName(r *querierv1.SandwichReport, n *querierv1.SandwichNode) string {
	if i := int(n.NameIndex); i >= 0 && i < len(r.Names) {
		return r.Names[i]
	}
	return "?"
}

func printSandwichNode(w interface{ Write([]byte) (int, error) }, r *querierv1.SandwichReport, n *querierv1.SandwichNode, unit string, depth, maxDepth int) error {
	if n == nil {
		return nil
	}
	name := sandwichName(r, n)
	if n.Truncated {
		name += " (truncated)"
	}
	if _, err := fmt.Fprintf(w, "%s%s  %s\n", strings.Repeat("  ", depth+1), name,
		formatUnit(float64(n.Total), unit)); err != nil {
		return err
	}
	if maxDepth != 0 && depth+1 >= maxDepth {
		if len(n.Children) > 0 {
			if _, err := fmt.Fprintf(w, "%s... %d more level(s)\n", strings.Repeat("  ", depth+2), 1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, c := range n.Children {
		if err := printSandwichNode(w, r, c, unit, depth+1, maxDepth); err != nil {
			return err
		}
	}
	return nil
}
