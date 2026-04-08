package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"connectrpc.com/connect"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1/ingesterv1connect"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/query/v1/queryv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/storegateway/v1/storegatewayv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/v2/pkg/api/connect"
	querydiagnostics "github.com/grafana/pyroscope/v2/pkg/frontend/readpath/queryfrontend/diagnostics"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/operations"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

func (c *phlareClient) queryClient() querierv1connect.QuerierServiceClient {
	return querierv1connect.NewQuerierServiceClient(
		c.httpClient(),
		c.URL,
		append(
			connectapi.DefaultClientOptions(),
			c.protocolOption(),
		)...,
	)
}

func (c *phlareClient) queryFrontendClient() queryv1connect.QueryFrontendServiceClient {
	return queryv1connect.NewQueryFrontendServiceClient(
		c.httpClient(),
		c.URL,
		append(
			connectapi.DefaultClientOptions(),
			c.protocolOption(),
		)...,
	)
}

func (c *phlareClient) storeGatewayClient() storegatewayv1connect.StoreGatewayServiceClient {
	return storegatewayv1connect.NewStoreGatewayServiceClient(
		c.httpClient(),
		c.URL,
		append(
			connectapi.DefaultClientOptions(),
			c.protocolOption(),
		)...,
	)
}

func (c *phlareClient) ingesterClient() ingesterv1connect.IngesterServiceClient {
	return ingesterv1connect.NewIngesterServiceClient(
		c.httpClient(),
		c.URL,
		append(
			connectapi.DefaultClientOptions(),
			c.protocolOption(),
		)...,
	)
}

type queryParams struct {
	*phlareClient
	From  string
	To    string
	Query string
}

func (p *queryParams) parseFromTo() (from time.Time, to time.Time, err error) {
	from, err = operations.ParseTime(p.From)
	if err != nil {
		return time.Time{}, time.Time{}, errors.Wrap(err, "failed to parse from")
	}
	to, err = operations.ParseTime(p.To)
	if err != nil {
		return time.Time{}, time.Time{}, errors.Wrap(err, "failed to parse to")
	}

	if to.Before(from) {
		return time.Time{}, time.Time{}, errors.Wrap(err, "from cannot be after")
	}

	return from, to, nil
}

func addQueryParams(queryCmd commander) *queryParams {
	params := new(queryParams)
	params.phlareClient = addPhlareClient(queryCmd)

	queryCmd.Flag("from", "Beginning of the query.").Default("now-1h").StringVar(&params.From)
	queryCmd.Flag("to", "End of the query.").Default("now").StringVar(&params.To)
	queryCmd.Flag("query", "Label selector to query.").Default("{}").StringVar(&params.Query)
	return params
}

type queryProfileParams struct {
	*queryParams
	ProfileType        string
	StacktraceSelector []string
	SpanSelector       []string
	ProfileIDs         []string
	MaxNodes           int64
}

func addQueryProfileParams(queryCmd commander) *queryProfileParams {
	params := new(queryProfileParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("stacktrace-selector", "Only query locations with those symbols. Provide multiple times starting with the root").StringsVar(&params.StacktraceSelector)
	queryCmd.Flag("span-selector", "Only query profiles with the given span IDs. Provide multiple times for multiple spans.").StringsVar(&params.SpanSelector)
	queryCmd.Flag("max-nodes", "Maximum number of nodes to return in the profile").Int64Var(&params.MaxNodes)
	return params
}

// validateQueryProfileParams checks for mutual exclusion between flags and
// validates the --profile-id format when provided.
func validateQueryProfileParams(params *queryProfileParams) error {
	if len(params.SpanSelector) > 0 && len(params.StacktraceSelector) > 0 {
		return errors.New("--span-selector and --stacktrace-selector cannot be used together")
	}

	// --profile-id and --span-selector serve different purposes and cannot be combined.
	// ProfileIdSelector uses profile_id (UUID) for drilling down from exemplars.
	// SpanSelector uses span_id for span-filtered queries. See PR #4872.
	if len(params.ProfileIDs) > 0 && len(params.SpanSelector) > 0 {
		return errors.New("--profile-id and --span-selector cannot be used together. --profile-id selects a specific profile by UUID (from exemplar queries). --span-selector filters by trace span ID.")
	}

	// Validate each --profile-id is a valid UUID if provided.
	for _, id := range params.ProfileIDs {
		if _, err := uuid.Parse(id); err != nil {
			return errors.New("--profile-id must be a valid UUID (e.g. 550e8400-e29b-41d4-a716-446655440000). Did you mean --span-selector for span IDs?")
		}
	}

	return nil
}

func queryProfile(ctx context.Context, params *queryProfileParams, outputFlag string, force bool, profileTree bool, async bool) (err error) {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}
	level.Info(logger).Log("msg", "query aggregated profile from profile store", "url", params.URL, "from", from, "to", to, "query", params.Query, "type", params.ProfileType)

	if err := validateQueryProfileParams(params); err != nil {
		return err
	}

	if async {
		return queryProfileAsync(ctx, params, from, to, outputFlag, force)
	}

	var profile *googlev1.Profile

	if len(params.SpanSelector) > 0 {
		level.Info(logger).Log("msg", "selecting with span selector", "spans", fmt.Sprintf("%v", params.SpanSelector))
		profile, err = querySpanProfile(ctx, params, from, to)
	} else {
		var locations []*typesv1.Location
		if len(params.StacktraceSelector) > 0 {
			locations = make([]*typesv1.Location, 0, len(params.StacktraceSelector))
			for _, cs := range params.StacktraceSelector {
				locations = append(locations, &typesv1.Location{
					Name: cs,
				})
			}
			level.Info(logger).Log("msg", "selecting with stackstrace selector", "call-site", fmt.Sprintf("%#+v", params.StacktraceSelector))
		}

		if profileTree {
			profile, err = queryProfileTree(ctx, params, from, to, locations)
		} else {
			profile, err = queryProfilePprof(ctx, params, from, to, locations)
		}
	}
	if err != nil {
		return err
	}

	return outputMergeProfile(ctx, outputFlag, force, profile)
}

func querySpanProfile(ctx context.Context, params *queryProfileParams, from time.Time, to time.Time) (*googlev1.Profile, error) {
	req := &querierv1.SelectMergeSpanProfileRequest{
		ProfileTypeID: params.ProfileType,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		LabelSelector: params.Query,
		SpanSelector:  params.SpanSelector,
		Format:        querierv1.ProfileFormat_PROFILE_FORMAT_TREE,
	}

	if params.MaxNodes > 0 {
		req.MaxNodes = &params.MaxNodes
	}

	qc := params.phlareClient.queryClient()
	resp, err := qc.SelectMergeSpanProfile(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, errors.Wrap(err, "failed to query span profile")
	}

	logDiagnostics(params.phlareClient, resp.Header())

	tree, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](resp.Msg.Tree)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal tree")
	}

	ty, err := model.ParseProfileTypeSelector(params.ProfileType)
	if err != nil {
		return nil, err
	}

	return pprof.FromTree(tree, ty, req.End*1e6), nil
}

func queryProfilePprof(ctx context.Context, params *queryProfileParams, from time.Time, to time.Time, locations []*typesv1.Location) (*googlev1.Profile, error) {
	req := &querierv1.SelectMergeProfileRequest{
		ProfileTypeID: params.ProfileType,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		LabelSelector: params.Query,
	}

	if params.MaxNodes > 0 {
		req.MaxNodes = &params.MaxNodes
	}

	if len(params.StacktraceSelector) > 0 {
		req.StackTraceSelector = &typesv1.StackTraceSelector{
			CallSite: locations,
		}
	}

	// ProfileIdSelector uses profile_id (UUID), NOT span_id. See PR #4872.
	if len(params.ProfileIDs) > 0 {
		req.ProfileIdSelector = params.ProfileIDs
	}

	qc := params.phlareClient.queryClient()

	resp, err := qc.SelectMergeProfile(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}

	logDiagnostics(params.phlareClient, resp.Header())

	return resp.Msg, err
}

// queryViaFrontendService sends a query via the QueryFrontendService and handles
// async responses transparently. If the server returns IN_PROGRESS, it polls
// until the query completes. Returns errFrontendUnsupported if the server
// doesn't support the QueryFrontendService.
var errFrontendUnsupported = fmt.Errorf("server does not support QueryFrontendService")

func queryViaFrontendService(ctx context.Context, client queryv1connect.QueryFrontendServiceClient, req *queryv1.QueryRequest) (*queryv1.QueryResponse, error) {
	resp, err := client.Query(ctx, connect.NewRequest(req))
	if err != nil {
		if connectErr := new(connect.Error); errors.As(err, &connectErr) {
			if connectErr.Code() == connect.CodeUnimplemented || connectErr.Code() == connect.CodeNotFound {
				return nil, errFrontendUnsupported
			}
		}
		return nil, err
	}

	// Sync response — no async promotion happened.
	if resp.Msg.RequestId == "" {
		return resp.Msg, nil
	}

	level.Info(logger).Log("msg", "query promoted to async", "request_id", resp.Msg.RequestId)

	// Poll until the query completes.
	pollReq := &queryv1.QueryRequest{RequestId: resp.Msg.RequestId}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}

		resp, err = client.Query(ctx, connect.NewRequest(pollReq))
		if err != nil {
			return nil, errors.Wrap(err, "failed to poll async query")
		}

		switch resp.Msg.Status {
		case queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS:
			level.Info(logger).Log("msg", "waiting for async query", "request_id", resp.Msg.RequestId, "elapsed", time.Since(start).Truncate(time.Second))
			continue
		case queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS:
			level.Info(logger).Log("msg", "async query completed", "request_id", resp.Msg.RequestId, "elapsed", time.Since(start).Truncate(time.Second))
			return resp.Msg, nil
		case queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_FAILURE:
			return nil, fmt.Errorf("async query failed: %s", resp.Msg.ErrorMessage)
		default:
			return nil, fmt.Errorf("unexpected async query status: %v", resp.Msg.Status)
		}
	}
}

func queryProfileAsync(ctx context.Context, params *queryProfileParams, from time.Time, to time.Time, outputFlag string, force bool) error {
	req := &queryv1.QueryRequest{
		StartTime:     from.UnixMilli(),
		EndTime:       to.UnixMilli(),
		LabelSelector: buildProfileLabelSelector(params),
		Async:         true,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PPROF,
			Pprof:     buildPprofQuery(params),
		}},
	}

	client := params.phlareClient.queryFrontendClient()
	resp, err := queryViaFrontendService(ctx, client, req)
	if err != nil {
		if errors.Is(err, errFrontendUnsupported) {
			return fmt.Errorf("the server does not support async queries; try without --async")
		}
		return err
	}

	profile, err := extractProfileFromResponse(resp)
	if err != nil {
		return err
	}
	return outputMergeProfile(ctx, outputFlag, force, profile)
}

func buildProfileLabelSelector(params *queryProfileParams) string {
	profileType, err := model.ParseProfileTypeSelector(params.ProfileType)
	if err != nil {
		return params.Query
	}
	ptMatcher := model.SelectorFromProfileType(profileType)
	if params.Query == "" || params.Query == "{}" {
		return "{" + ptMatcher.String() + "}"
	}
	// Strip trailing "}" and append the profile type matcher.
	return params.Query[:len(params.Query)-1] + "," + ptMatcher.String() + "}"
}

func buildPprofQuery(params *queryProfileParams) *queryv1.PprofQuery {
	q := &queryv1.PprofQuery{}
	if params.MaxNodes > 0 {
		q.MaxNodes = params.MaxNodes
	}
	if len(params.StacktraceSelector) > 0 {
		locations := make([]*typesv1.Location, 0, len(params.StacktraceSelector))
		for _, cs := range params.StacktraceSelector {
			locations = append(locations, &typesv1.Location{Name: cs})
		}
		q.StackTraceSelector = &typesv1.StackTraceSelector{CallSite: locations}
	}
	if len(params.ProfileIDs) > 0 {
		q.ProfileIdSelector = params.ProfileIDs
	}
	return q
}

func extractProfileFromResponse(resp *queryv1.QueryResponse) (*googlev1.Profile, error) {
	if len(resp.Reports) == 0 {
		return &googlev1.Profile{}, nil
	}
	report := resp.Reports[0]
	if report.Pprof == nil {
		return nil, fmt.Errorf("unexpected report type: expected pprof")
	}
	var p googlev1.Profile
	if err := pprof.Unmarshal(report.Pprof.Pprof, &p); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal pprof from response")
	}
	return &p, nil
}

func queryProfileTree(ctx context.Context, params *queryProfileParams, from time.Time, to time.Time, locations []*typesv1.Location) (*googlev1.Profile, error) {
	req := &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: params.ProfileType,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		LabelSelector: params.Query,
		Format:        querierv1.ProfileFormat_PROFILE_FORMAT_TREE,
	}

	if params.MaxNodes > 0 {
		req.MaxNodes = &params.MaxNodes
	}

	if len(params.StacktraceSelector) > 0 {
		req.StackTraceSelector = &typesv1.StackTraceSelector{
			CallSite: locations,
		}
	}

	// ProfileIdSelector uses profile_id (UUID), NOT span_id. See PR #4872.
	if len(params.ProfileIDs) > 0 {
		req.ProfileIdSelector = params.ProfileIDs
	}

	qc := params.phlareClient.queryClient()
	resp, err := qc.SelectMergeStacktraces(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, errors.Wrap(err, "failed to query")
	}

	logDiagnostics(params.phlareClient, resp.Header())

	tree, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](resp.Msg.Tree)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal tree")
	}

	ty, err := model.ParseProfileTypeSelector(params.ProfileType)
	if err != nil {
		return nil, err
	}

	return pprof.FromTree(tree, ty, req.End*1e6), nil
}

func selectMergeProfile(ctx context.Context, client *phlareClient, outputFlag string, force bool, req *querierv1.SelectMergeProfileRequest) error {
	qc := client.queryClient()
	resp, err := qc.SelectMergeProfile(ctx, connect.NewRequest(req))
	if err != nil {
		return errors.Wrap(err, "failed to query")
	}

	logDiagnostics(client, resp.Header())

	return outputMergeProfile(ctx, outputFlag, force, resp.Msg)
}

func logDiagnostics(client *phlareClient, headers http.Header) {
	if !client.CollectDiagnostics {
		return
	}

	diagID := headers.Get(querydiagnostics.IdHeader)

	if diagID != "" {
		level.Info(logger).Log(
			"msg", "query diagnostics",
			"diagnostics_id", diagID,
		)
	}
}

type queryGoPGOParams struct {
	*queryProfileParams
	KeepLocations    uint32
	AggregateCallees bool
}

func addQueryGoPGOParams(queryCmd commander) *queryGoPGOParams {
	params := new(queryGoPGOParams)
	params.queryProfileParams = addQueryProfileParams(queryCmd)
	queryCmd.Flag("keep-locations", "Number of leaf locations to keep.").Default("5").Uint32Var(&params.KeepLocations)
	queryCmd.Flag("aggregate-callees", "Default: true. Aggregate samples for the same callee by ignoring the line numbers in the leaf locations. Use --aggregate-callees to enable or --no-aggregate-callees to disable.").Default("true").BoolVar(&params.AggregateCallees)
	return params
}

func queryGoPGO(ctx context.Context, params *queryGoPGOParams, outputFlag string, force bool) (err error) {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}
	level.Info(logger).Log("msg", "querying pprof profile for Go PGO",
		"url", params.URL,
		"query", params.Query,
		"from", from,
		"to", to,
		"type", params.ProfileType,
		"output", outputFlag,
		"keep-locations", params.KeepLocations,
		"aggregate-callees", params.AggregateCallees,
	)
	return selectMergeProfile(ctx, params.phlareClient, outputFlag, force,
		&querierv1.SelectMergeProfileRequest{
			ProfileTypeID: params.ProfileType,
			Start:         from.UnixMilli(),
			End:           to.UnixMilli(),
			LabelSelector: params.Query,
			StackTraceSelector: &typesv1.StackTraceSelector{
				GoPgo: &typesv1.GoPGO{
					KeepLocations:    params.KeepLocations,
					AggregateCallees: params.AggregateCallees,
				},
			},
		})
}

type querySeriesParams struct {
	*queryParams
	LabelNames []string
	APIType    string
	Output     string
}

func addQuerySeriesParams(queryCmd commander) *querySeriesParams {
	params := new(querySeriesParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("label-names", "Filter returned labels to the supplied label names. Without any filter all labels are returned.").StringsVar(&params.LabelNames)
	queryCmd.Flag("api-type", "Which API type to query (querier, ingester or store-gateway).").Default("querier").StringVar(&params.APIType)
	queryCmd.Flag("output", "Output format, one of: table, json.").Default("table").StringVar(&params.Output)
	return params
}

func querySeries(ctx context.Context, params *querySeriesParams) (err error) {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	level.Info(logger).Log("msg", fmt.Sprintf("query series from %s", params.APIType), "url", params.URL, "from", from, "to", to, "labelNames", fmt.Sprintf("%q", params.LabelNames))

	var result []*typesv1.Labels
	switch params.APIType {
	case "querier":
		qc := params.phlareClient.queryClient()
		resp, err := qc.Series(ctx, connect.NewRequest(&querierv1.SeriesRequest{
			Start:      from.UnixMilli(),
			End:        to.UnixMilli(),
			Matchers:   []string{params.Query},
			LabelNames: params.LabelNames,
		}))
		if err != nil {
			return errors.Wrap(err, "failed to query")
		}
		logDiagnostics(params.phlareClient, resp.Header())
		result = resp.Msg.LabelsSet
	case "ingester":
		ic := params.phlareClient.ingesterClient()
		resp, err := ic.Series(ctx, connect.NewRequest(&ingestv1.SeriesRequest{
			Start:      from.UnixMilli(),
			End:        to.UnixMilli(),
			Matchers:   []string{params.Query},
			LabelNames: params.LabelNames,
		}))
		if err != nil {
			return errors.Wrap(err, "failed to query")
		}
		result = resp.Msg.LabelsSet
	case "store-gateway":
		sc := params.phlareClient.storeGatewayClient()
		resp, err := sc.Series(ctx, connect.NewRequest(&ingestv1.SeriesRequest{
			Start:      from.UnixMilli(),
			End:        to.UnixMilli(),
			Matchers:   []string{params.Query},
			LabelNames: params.LabelNames,
		}))
		if err != nil {
			return errors.Wrap(err, "failed to query")
		}
		result = resp.Msg.LabelsSet
	default:
		return errors.Errorf("unknown api type %s", params.APIType)
	}

	return outputSeries(ctx, result, params.Output, from, to)
}

type queryLabelValuesCardinalityParams struct {
	*queryParams
	TopN uint64
}

func addQueryLabelValuesCardinalityParams(queryCmd commander) *queryLabelValuesCardinalityParams {
	params := new(queryLabelValuesCardinalityParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("top-n", "Show the top N high cardinality label values").Default("20").Uint64Var(&params.TopN)
	return params
}

func queryLabelValuesCardinality(ctx context.Context, params *queryLabelValuesCardinalityParams) (err error) {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}

	level.Info(logger).Log("msg", "query label names", "url", params.URL, "from", from, "to", to)

	qc := params.phlareClient.queryClient()
	resp, err := qc.LabelNames(ctx, connect.NewRequest(&typesv1.LabelNamesRequest{
		Start:    from.UnixMilli(),
		End:      to.UnixMilli(),
		Matchers: []string{params.Query},
	}))
	if err != nil {
		return errors.Wrap(err, "failed to query")
	}
	logDiagnostics(params.phlareClient, resp.Header())

	level.Info(logger).Log("msg", fmt.Sprintf("received %d label names", len(resp.Msg.Names)))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	result := make([]struct {
		count int
		name  string
	}, len(resp.Msg.Names))

	for idx := range resp.Msg.Names {
		idx := idx
		g.Go(func() error {
			name := resp.Msg.Names[idx]
			resp, err := qc.LabelValues(gctx, connect.NewRequest(&typesv1.LabelValuesRequest{
				Name:     name,
				Start:    from.UnixMilli(),
				End:      to.UnixMilli(),
				Matchers: []string{params.Query},
			}))
			if err != nil {
				return fmt.Errorf("failed to query label values for %s: %w", name, err)
			}

			result[idx].name = name
			result[idx].count = len(resp.Msg.Names)

			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	// sort the result
	sort.Slice(result, func(i, j int) bool {
		return result[i].count > result[j].count
	})

	table := newTableWriter(output(ctx))
	table.SetHeader([]string{"LabelName", "Value count"})
	if len(result) > int(params.TopN) {
		result = result[:params.TopN]
	}
	for _, r := range result {
		table.Append([]string{r.name, humanize.FormatInteger("#,###.", r.count)})
	}
	table.Render()

	return nil
}
