package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"connectrpc.com/connect"
	"github.com/dustin/go-humanize"
	"github.com/go-kit/log/level"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1/ingesterv1connect"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/api/gen/proto/go/storegateway/v1/storegatewayv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/operations"
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
}

func addQueryProfileParams(queryCmd commander) *queryProfileParams {
	params := new(queryProfileParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("profile-type", "Profile type to query.").Default("process_cpu:cpu:nanoseconds:cpu:nanoseconds").StringVar(&params.ProfileType)
	queryCmd.Flag("stacktrace-selector", "Only query locations with those symbols. Provide multiple times starting with the root").StringsVar(&params.StacktraceSelector)
	return params
}

func queryProfile(ctx context.Context, params *queryProfileParams, outputFlag string) (err error) {
	from, to, err := params.parseFromTo()
	if err != nil {
		return err
	}
	level.Info(logger).Log("msg", "query aggregated profile from profile store", "url", params.URL, "from", from, "to", to, "query", params.Query, "type", params.ProfileType)

	req := &querierv1.SelectMergeProfileRequest{
		ProfileTypeID: params.ProfileType,
		Start:         from.UnixMilli(),
		End:           to.UnixMilli(),
		LabelSelector: params.Query,
	}

	if len(params.StacktraceSelector) > 0 {
		locations := make([]*typesv1.Location, 0, len(params.StacktraceSelector))
		for _, cs := range params.StacktraceSelector {
			locations = append(locations, &typesv1.Location{
				Name: cs,
			})
		}
		req.StackTraceSelector = &typesv1.StackTraceSelector{
			CallSite: locations,
		}
		level.Info(logger).Log("msg", "selecting with stackstrace selector", "call-site", fmt.Sprintf("%#+v", params.StacktraceSelector))
	}

	return selectMergeProfile(ctx, params.phlareClient, outputFlag, req)
}

func selectMergeProfile(ctx context.Context, client *phlareClient, outputFlag string, req *querierv1.SelectMergeProfileRequest) error {
	qc := client.queryClient()
	resp, err := qc.SelectMergeProfile(ctx, connect.NewRequest(req))
	if err != nil {
		return errors.Wrap(err, "failed to query")
	}

	return outputMergeProfile(ctx, outputFlag, resp.Msg)
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
	queryCmd.Flag("aggregate-callees", "Aggregate samples for the same callee by ignoring the line numbers in the leaf locations.").Default("true").BoolVar(&params.AggregateCallees)
	return params
}

func queryGoPGO(ctx context.Context, params *queryGoPGOParams, outputFlag string) (err error) {
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
	return selectMergeProfile(ctx, params.phlareClient, outputFlag,
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
}

func addQuerySeriesParams(queryCmd commander) *querySeriesParams {
	params := new(querySeriesParams)
	params.queryParams = addQueryParams(queryCmd)
	queryCmd.Flag("label-names", "Filter returned labels to the supplied label names. Without any filter all labels are returned.").StringsVar(&params.LabelNames)
	queryCmd.Flag("api-type", "Which API type to query (querier, ingester or store-gateway).").Default("querier").StringVar(&params.APIType)
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

	err = outputSeries(result)
	return err
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

	table := tablewriter.NewWriter(output(ctx))
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
