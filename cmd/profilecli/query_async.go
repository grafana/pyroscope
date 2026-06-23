package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/query/v1/queryv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

// runAsyncQuery submits an async query and polls until it completes.
// Returns a clear error when the server does not support async queries.
func runAsyncQuery(ctx context.Context, client queryv1connect.QueryFrontendServiceClient, req *queryv1.QueryRequest) (*queryv1.QueryResponse, error) {
	submit, err := client.AsyncQuery(ctx, connect.NewRequest(&queryv1.AsyncQueryRequest{Query: req}))
	if err != nil {
		if connectErr := new(connect.Error); errors.As(err, &connectErr) && connectErr.Code() == connect.CodeUnimplemented {
			return nil, fmt.Errorf("server has async queries disabled (set -query-frontend.async-queries-enabled=true)")
		}
		return nil, err
	}
	level.Info(logger).Log("msg", "async query submitted", "request_id", submit.Msg.RequestId)

	pollReq := &queryv1.AsyncQueryRequest{RequestId: submit.Msg.RequestId}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}

		poll, err := client.AsyncQuery(ctx, connect.NewRequest(pollReq))
		if err != nil {
			return nil, fmt.Errorf("failed to poll async query: %w", err)
		}

		switch poll.Msg.Status {
		case queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS:
			level.Info(logger).Log("msg", "waiting for async query", "request_id", poll.Msg.RequestId, "elapsed", time.Since(start).Truncate(time.Second))
			continue
		case queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS:
			level.Info(logger).Log("msg", "async query completed", "request_id", poll.Msg.RequestId, "elapsed", time.Since(start).Truncate(time.Second))
			return poll.Msg.Response, nil
		case queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_FAILURE:
			return nil, fmt.Errorf("async query failed: %s", poll.Msg.ErrorMessage)
		default:
			return nil, fmt.Errorf("unexpected async query status: %v", poll.Msg.Status)
		}
	}
}

// asyncQueryProfile submits a pprof async query and returns the unmarshaled profile.
func asyncQueryProfile(ctx context.Context, client queryv1connect.QueryFrontendServiceClient, profileTypeID, labelSelector string, start, end int64, maxNodes int64, stackTraceSelector *typesv1.StackTraceSelector, profileIDs []string) (*googlev1.Profile, error) {
	req := &queryv1.QueryRequest{
		StartTime:     start,
		EndTime:       end,
		LabelSelector: labelSelectorWithProfileType(labelSelector, profileTypeID),
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PPROF,
			Pprof: &queryv1.PprofQuery{
				MaxNodes:           maxNodes,
				StackTraceSelector: stackTraceSelector,
				ProfileIdSelector:  profileIDs,
			},
		}},
	}
	resp, err := runAsyncQuery(ctx, client, req)
	if err != nil {
		return nil, err
	}
	return extractProfileFromResponse(resp)
}

// asyncQuerySeries submits a series-labels async query and returns the labels set.
func asyncQuerySeries(ctx context.Context, client queryv1connect.QueryFrontendServiceClient, labelSelector string, labelNames []string, start, end int64) ([]*typesv1.Labels, error) {
	req := &queryv1.QueryRequest{
		StartTime:     start,
		EndTime:       end,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_SERIES_LABELS,
			SeriesLabels: &queryv1.SeriesLabelsQuery{
				LabelNames: labelNames,
			},
		}},
	}
	resp, err := runAsyncQuery(ctx, client, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Reports) == 0 {
		return nil, nil
	}
	if resp.Reports[0].SeriesLabels == nil {
		return nil, fmt.Errorf("unexpected report type: expected series_labels")
	}
	return resp.Reports[0].SeriesLabels.SeriesLabels, nil
}

// asyncQueryTimeSeries submits a time-series async query and returns the series.
func asyncQueryTimeSeries(ctx context.Context, client queryv1connect.QueryFrontendServiceClient, profileTypeID, labelSelector string, start, end int64, step float64, groupBy []string, exemplarType typesv1.ExemplarType) ([]*typesv1.Series, error) {
	req := &queryv1.QueryRequest{
		StartTime:     start,
		EndTime:       end,
		LabelSelector: labelSelectorWithProfileType(labelSelector, profileTypeID),
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
			TimeSeries: &queryv1.TimeSeriesQuery{
				Step:         step,
				GroupBy:      groupBy,
				ExemplarType: exemplarType,
			},
		}},
	}
	resp, err := runAsyncQuery(ctx, client, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Reports) == 0 {
		return nil, nil
	}
	if resp.Reports[0].TimeSeries == nil {
		return nil, fmt.Errorf("unexpected report type: expected time_series")
	}
	return resp.Reports[0].TimeSeries.TimeSeries, nil
}

// labelSelectorWithProfileType combines a label selector with a profile type ID.
func labelSelectorWithProfileType(labelSelector, profileTypeID string) string {
	profileType, err := model.ParseProfileTypeSelector(profileTypeID)
	if err != nil {
		return labelSelector
	}
	ptMatcher := model.SelectorFromProfileType(profileType)
	if labelSelector == "" || labelSelector == "{}" {
		return "{" + ptMatcher.String() + "}"
	}
	return labelSelector[:len(labelSelector)-1] + "," + ptMatcher.String() + "}"
}

// extractProfileFromResponse extracts a pprof profile from a QueryResponse.
func extractProfileFromResponse(resp *queryv1.QueryResponse) (*googlev1.Profile, error) {
	if len(resp.Reports) == 0 {
		return &googlev1.Profile{}, nil
	}
	if resp.Reports[0].Pprof == nil {
		return nil, fmt.Errorf("unexpected report type: expected pprof")
	}
	var p googlev1.Profile
	if err := pprof.Unmarshal(resp.Reports[0].Pprof.Pprof, &p); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pprof from response: %w", err)
	}
	return &p, nil
}
