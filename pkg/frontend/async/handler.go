package async

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	pyrotenant "github.com/grafana/pyroscope/v2/pkg/tenant"
)

// Handler decorates a QuerierServiceHandler with AsyncQuery support. All
// other RPCs pass through to the embedded handler unchanged.
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

func (h *Handler) AsyncQuery(
	ctx context.Context,
	req *connect.Request[querierv1.AsyncQueryRequest],
) (*connect.Response[querierv1.AsyncQueryResponse], error) {
	if h.coordinator == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("async queries are disabled (set -query-frontend.async-queries-enabled=true)"))
	}

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	tenantID := tenant.JoinTenantIDs(tenantIDs)

	if req.Msg.RequestId != "" {
		return h.poll(ctx, tenantID, req.Msg.RequestId)
	}
	return h.submit(ctx, tenantID, req.Msg)
}

func (h *Handler) submit(
	ctx context.Context,
	tenantID string,
	req *querierv1.AsyncQueryRequest,
) (*connect.Response[querierv1.AsyncQueryResponse], error) {
	queryCtx := pyrotenant.InjectTenantID(context.Background(), tenantID)
	resultCh := make(chan QueryResult, 1)

	dispatch, err := h.dispatcherFor(queryCtx, req, resultCh)
	if err != nil {
		return nil, err
	}

	// Reserve the concurrency slot before dispatching so a rejected submit
	// never starts background work.
	requestID, err := h.coordinator.Register(ctx, tenantID, resultCh)
	if err != nil {
		return nil, connect.NewError(connect.CodeResourceExhausted, err)
	}

	go dispatch()

	return connect.NewResponse(&querierv1.AsyncQueryResponse{
		RequestId: requestID,
		Status:    querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS,
	}), nil
}

func (h *Handler) dispatcherFor(
	ctx context.Context,
	req *querierv1.AsyncQueryRequest,
	resultCh chan<- QueryResult,
) (func(), error) {
	switch q := req.Query.(type) {
	case *querierv1.AsyncQueryRequest_SelectMergeStacktraces:
		inner := connect.NewRequest(q.SelectMergeStacktraces)
		return func() {
			resp, err := h.QuerierServiceHandler.SelectMergeStacktraces(ctx, inner)
			if err != nil {
				resultCh <- QueryResult{Err: err}
				return
			}
			resultCh <- QueryResult{Response: &querierv1.AsyncQueryResponse{
				Result: &querierv1.AsyncQueryResponse_SelectMergeStacktraces{
					SelectMergeStacktraces: resp.Msg,
				},
			}}
		}, nil
	case nil:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("either query or request_id must be set"))
	default:
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("async query for %T is not supported", q))
	}
}

func (h *Handler) poll(
	ctx context.Context,
	tenantID string,
	requestID string,
) (*connect.Response[querierv1.AsyncQueryResponse], error) {
	result, err := h.coordinator.PollQuery(ctx, tenantID, requestID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if result == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("async query not found"))
	}

	resp := &querierv1.AsyncQueryResponse{RequestId: requestID}
	switch result.Metadata.Status {
	case StatusInProgress:
		resp.Status = querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS
	case StatusSuccess:
		resp.Status = querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS
		if result.Response != nil {
			resp.Result = result.Response.Result
		}
	case StatusFailure:
		resp.Status = querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_FAILURE
		resp.ErrorMessage = result.Metadata.ErrorMessage
	}

	return connect.NewResponse(resp), nil
}
