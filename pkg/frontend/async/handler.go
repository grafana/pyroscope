package async

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
)

// Handler decorates a QuerierServiceHandler with async query support for
// SelectMergeStacktraces. All other RPCs pass through to the embedded
// handler unchanged.
type Handler struct {
	querierv1connect.QuerierServiceHandler
	logger      log.Logger
	coordinator *Coordinator
}

func NewHandler(logger log.Logger, next querierv1connect.QuerierServiceHandler, coordinator *Coordinator) *Handler {
	return &Handler{
		QuerierServiceHandler: next,
		logger:                logger,
		coordinator:           coordinator,
	}
}

// SelectMergeStacktraces honors the request's optional Async field:
//   - Async == nil or Type == DISABLED: run synchronously, return the
//     wrapped handler's response unchanged (Async stays nil).
//   - Async.RequestId != "": treat as a poll. All other request fields
//     are ignored. The response carries only the Async metadata, plus
//     the result payload on SUCCESS.
//   - Async.Type == FORCE with empty RequestId: dispatch in the
//     background and return a response carrying only the Async
//     metadata in IN_PROGRESS.
func (h *Handler) SelectMergeStacktraces(
	ctx context.Context,
	req *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	async := req.Msg.GetAsync()
	if async == nil || async.GetType() == querierv1.AsyncQueryType_ASYNC_QUERY_TYPE_DISABLED {
		return h.QuerierServiceHandler.SelectMergeStacktraces(ctx, req)
	}

	if h.coordinator == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("async queries are disabled (set -query-frontend.async-queries-enabled=true)"))
	}

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	tenantID := tenant.JoinTenantIDs(tenantIDs)

	if async.GetRequestId() != "" {
		return h.poll(ctx, tenantID, async.GetRequestId())
	}
	return h.submit(ctx, tenantID, req.Msg)
}

func (h *Handler) submit(
	ctx context.Context,
	tenantID string,
	req *querierv1.SelectMergeStacktracesRequest,
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	requestID, err := h.coordinator.Submit(ctx, tenantID, req)
	if err != nil {
		// No request ID exists yet, so this line is only findable by tenant:
		// the submission never became part of any request's lifecycle.
		level.Warn(h.logger).Log("msg", "async query submission rejected", "tenant", tenantID, "err", err)
		return nil, connect.NewError(connect.CodeResourceExhausted, err)
	}
	return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
		Async: &querierv1.AsyncQueryResponse{
			RequestId: requestID,
			Status:    querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS,
		},
	}), nil
}

func (h *Handler) poll(
	ctx context.Context,
	tenantID string,
	requestID string,
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	logger := log.With(h.logger, "tenant", tenantID, "request_id", requestID)

	result, err := h.coordinator.PollQuery(ctx, tenantID, requestID)
	if err != nil {
		level.Warn(logger).Log("msg", "async query poll failed", "err", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if result == nil {
		// Also covers a poll for another tenant's request: get() reports a
		// cross-tenant lookup as not found rather than leaking its existence.
		level.Warn(logger).Log("msg", "async query polled but not found")
		return nil, connect.NewError(connect.CodeNotFound, errors.New("async query not found"))
	}

	resp := &querierv1.SelectMergeStacktracesResponse{
		Async: &querierv1.AsyncQueryResponse{RequestId: requestID},
	}
	switch result.Metadata.Status {
	case StatusInProgress:
		resp.Async.Status = querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS
	case StatusSuccess:
		resp.Async.Status = querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS
		if result.Response != nil {
			resp.Flamegraph = result.Response.Flamegraph
			resp.Tree = result.Response.Tree
			resp.Dot = result.Response.Dot
			resp.Pprof = result.Response.Pprof
		}
	case StatusFailure:
		resp.Async.Status = querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_FAILURE
		resp.Async.ErrorMessage = result.Metadata.ErrorMessage
	}

	logger = log.With(logger,
		"status", string(result.Metadata.Status),
		"age", time.Since(result.Metadata.CreatedAt).Round(time.Millisecond),
	)
	switch result.Metadata.Status {
	case StatusInProgress:
		level.Debug(logger).Log("msg", "async query polled, still in progress")
	case StatusFailure:
		level.Info(logger).Log("msg", "async query result returned to client", "err", result.Metadata.ErrorMessage)
	default:
		level.Info(logger).Log("msg", "async query result returned to client")
	}
	return connect.NewResponse(resp), nil
}
