package async

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"google.golang.org/protobuf/proto"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	pyrotenant "github.com/grafana/pyroscope/v2/pkg/tenant"
)

// Handler decorates a QuerierServiceHandler with async query support for
// SelectMergeStacktraces. All other RPCs pass through to the embedded
// handler unchanged.
type Handler struct {
	querierv1connect.QuerierServiceHandler
	coordinator *Coordinator
}

func NewHandler(next querierv1connect.QuerierServiceHandler, coordinator *Coordinator) *Handler {
	return &Handler{
		QuerierServiceHandler: next,
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
	queryCtx := pyrotenant.InjectTenantID(context.Background(), tenantID)
	resultCh := make(chan QueryResult, 1)

	// Strip the Async marker before dispatching so the wrapped handler
	// treats this as an ordinary sync request.
	inner := proto.Clone(req).(*querierv1.SelectMergeStacktracesRequest)
	inner.Async = nil

	// Reserve the concurrency slot before dispatching so a rejected
	// submit never starts background work.
	requestID, err := h.coordinator.Register(ctx, tenantID, resultCh)
	if err != nil {
		return nil, connect.NewError(connect.CodeResourceExhausted, err)
	}

	go func() {
		resp, err := h.QuerierServiceHandler.SelectMergeStacktraces(queryCtx, connect.NewRequest(inner))
		if err != nil {
			resultCh <- QueryResult{Err: err}
			return
		}
		resultCh <- QueryResult{Response: resp.Msg}
	}()

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
	result, err := h.coordinator.PollQuery(ctx, tenantID, requestID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if result == nil {
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
		}
	case StatusFailure:
		resp.Async.Status = querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_FAILURE
		resp.Async.ErrorMessage = result.Metadata.ErrorMessage
	}

	return connect.NewResponse(resp), nil
}
