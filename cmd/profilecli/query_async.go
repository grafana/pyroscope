package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

// runAsyncSelectMergeStacktraces submits a SelectMergeStacktraces query via
// the experimental AsyncQuery RPC and polls until it completes.
func runAsyncSelectMergeStacktraces(
	ctx context.Context,
	client querierv1connect.QuerierServiceClient,
	req *querierv1.SelectMergeStacktracesRequest,
) (*querierv1.SelectMergeStacktracesResponse, error) {
	submit, err := client.AsyncQuery(ctx, connect.NewRequest(&querierv1.AsyncQueryRequest{
		Query: &querierv1.AsyncQueryRequest_SelectMergeStacktraces{
			SelectMergeStacktraces: req,
		},
	}))
	if err != nil {
		if connectErr := new(connect.Error); errors.As(err, &connectErr) && connectErr.Code() == connect.CodeUnimplemented {
			return nil, fmt.Errorf("server has async queries disabled (set -query-frontend.async-queries-enabled=true)")
		}
		return nil, err
	}
	level.Info(logger).Log("msg", "async query submitted", "request_id", submit.Msg.RequestId)

	pollReq := &querierv1.AsyncQueryRequest{RequestId: submit.Msg.RequestId}
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
		case querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS:
			level.Info(logger).Log("msg", "waiting for async query", "request_id", poll.Msg.RequestId, "elapsed", time.Since(start).Truncate(time.Second))
			continue
		case querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS:
			level.Info(logger).Log("msg", "async query completed", "request_id", poll.Msg.RequestId, "elapsed", time.Since(start).Truncate(time.Second))
			result, ok := poll.Msg.Result.(*querierv1.AsyncQueryResponse_SelectMergeStacktraces)
			if !ok || result.SelectMergeStacktraces == nil {
				return nil, fmt.Errorf("missing result of type select_merge_stacktraces in response")
			}
			return result.SelectMergeStacktraces, nil
		case querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_FAILURE:
			return nil, fmt.Errorf("async query failed: %s", poll.Msg.ErrorMessage)
		default:
			return nil, fmt.Errorf("unexpected async query status: %v", poll.Msg.Status)
		}
	}
}

// runAsyncSelectMergeSpanProfile submits a SelectMergeSpanProfile query via
// the experimental AsyncQuery RPC and polls until it completes.
func runAsyncSelectMergeSpanProfile(
	ctx context.Context,
	client querierv1connect.QuerierServiceClient,
	req *querierv1.SelectMergeSpanProfileRequest,
) (*querierv1.SelectMergeSpanProfileResponse, error) {
	submit, err := client.AsyncQuery(ctx, connect.NewRequest(&querierv1.AsyncQueryRequest{
		Query: &querierv1.AsyncQueryRequest_SelectMergeSpanProfile{
			SelectMergeSpanProfile: req,
		},
	}))
	if err != nil {
		if connectErr := new(connect.Error); errors.As(err, &connectErr) && connectErr.Code() == connect.CodeUnimplemented {
			return nil, fmt.Errorf("server has async queries disabled (set -query-frontend.async-queries-enabled=true)")
		}
		return nil, err
	}
	level.Info(logger).Log("msg", "async query submitted", "request_id", submit.Msg.RequestId)

	pollReq := &querierv1.AsyncQueryRequest{RequestId: submit.Msg.RequestId}
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
		case querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS:
			level.Info(logger).Log("msg", "waiting for async query", "request_id", poll.Msg.RequestId, "elapsed", time.Since(start).Truncate(time.Second))
			continue
		case querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS:
			level.Info(logger).Log("msg", "async query completed", "request_id", poll.Msg.RequestId, "elapsed", time.Since(start).Truncate(time.Second))
			result, ok := poll.Msg.Result.(*querierv1.AsyncQueryResponse_SelectMergeSpanProfile)
			if !ok || result.SelectMergeSpanProfile == nil {
				return nil, fmt.Errorf("missing result of type select_merge_span_profile in response")
			}
			return result.SelectMergeSpanProfile, nil
		case querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_FAILURE:
			return nil, fmt.Errorf("async query failed: %s", poll.Msg.ErrorMessage)
		default:
			return nil, fmt.Errorf("unexpected async query status: %v", poll.Msg.Status)
		}
	}
}

// asyncQuerySpanProfile mirrors querySpanProfile but executes via the async
// query API.
func asyncQuerySpanProfile(ctx context.Context, params *queryProfileParams, from, to time.Time) (*googlev1.Profile, error) {
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

	resp, err := runAsyncSelectMergeSpanProfile(ctx, params.queryClient(), req)
	if err != nil {
		return nil, err
	}

	tree, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](resp.Tree)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal tree: %w", err)
	}
	ty, err := model.ParseProfileTypeSelector(params.ProfileType)
	if err != nil {
		return nil, err
	}
	return pprof.FromTree(tree, ty, req.End*1e6), nil
}

// asyncQueryProfileTree mirrors queryProfileTree but executes via the async
// query API. It always requests TREE format because that is the only shape
// the experimental AsyncQuery RPC currently supports.
func asyncQueryProfileTree(ctx context.Context, params *queryProfileParams, from, to time.Time, locations []*typesv1.Location) (*googlev1.Profile, error) {
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
		req.StackTraceSelector = &typesv1.StackTraceSelector{CallSite: locations}
	}
	if len(params.ProfileIDs) > 0 {
		req.ProfileIdSelector = params.ProfileIDs
	}

	resp, err := runAsyncSelectMergeStacktraces(ctx, params.queryClient(), req)
	if err != nil {
		return nil, err
	}

	tree, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](resp.Tree)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal tree: %w", err)
	}
	ty, err := model.ParseProfileTypeSelector(params.ProfileType)
	if err != nil {
		return nil, err
	}
	return pprof.FromTree(tree, ty, req.End*1e6), nil
}
