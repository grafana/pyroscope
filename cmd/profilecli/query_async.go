package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"google.golang.org/protobuf/proto"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

// runAsyncSelectMergeStacktraces submits the given request via
// SelectMergeStacktraces with Async.Type=FORCE, then polls until the
// query completes.
func runAsyncSelectMergeStacktraces(
	ctx context.Context,
	client querierv1connect.QuerierServiceClient,
	req *querierv1.SelectMergeStacktracesRequest,
) (*querierv1.SelectMergeStacktracesResponse, error) {
	submitReq := proto.Clone(req).(*querierv1.SelectMergeStacktracesRequest)
	submitReq.Async = &querierv1.AsyncQueryRequest{Type: querierv1.AsyncQueryType_ASYNC_QUERY_TYPE_FORCE}

	submit, err := client.SelectMergeStacktraces(ctx, connect.NewRequest(submitReq))
	if err != nil {
		if connectErr := new(connect.Error); errors.As(err, &connectErr) && connectErr.Code() == connect.CodeUnimplemented {
			return nil, fmt.Errorf("server has async queries disabled (set -query-frontend.async-queries-enabled=true)")
		}
		return nil, err
	}
	if submit.Msg.GetAsync() == nil || submit.Msg.Async.RequestId == "" {
		return nil, fmt.Errorf("server did not return an async request_id")
	}
	requestID := submit.Msg.Async.RequestId
	level.Info(logger).Log("msg", "async query submitted", "request_id", requestID)

	pollReq := &querierv1.SelectMergeStacktracesRequest{
		Async: &querierv1.AsyncQueryRequest{
			Type:      querierv1.AsyncQueryType_ASYNC_QUERY_TYPE_FORCE,
			RequestId: requestID,
		},
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}

		poll, err := client.SelectMergeStacktraces(ctx, connect.NewRequest(pollReq))
		if err != nil {
			return nil, fmt.Errorf("failed to poll async query: %w", err)
		}

		async := poll.Msg.GetAsync()
		if async == nil {
			return nil, fmt.Errorf("server returned no async metadata for poll request %s", requestID)
		}

		switch async.Status {
		case querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS:
			level.Info(logger).Log("msg", "waiting for async query", "request_id", requestID, "elapsed", time.Since(start).Truncate(time.Second))
			continue
		case querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS:
			level.Info(logger).Log("msg", "async query completed", "request_id", requestID, "elapsed", time.Since(start).Truncate(time.Second))
			// Strip the Async marker so the caller sees a normal sync response shape.
			poll.Msg.Async = nil
			return poll.Msg, nil
		case querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_FAILURE:
			return nil, fmt.Errorf("async query failed: %s", async.ErrorMessage)
		default:
			return nil, fmt.Errorf("unexpected async query status: %v", async.Status)
		}
	}
}

// asyncQueryProfileTree mirrors queryProfileTree but executes via the
// async query path on SelectMergeStacktraces.
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
