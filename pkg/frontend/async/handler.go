package async

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	pyrotenant "github.com/grafana/pyroscope/v2/pkg/tenant"
)

// QueryFn executes a query synchronously and returns the response.
type QueryFn func(ctx context.Context, req *queryv1.QueryRequest) (*queryv1.QueryResponse, error)

// Handler implements queryv1connect.QueryFrontendServiceHandler. Sync Query is
// always served. AsyncQuery returns Unimplemented when coordinator is nil
// (i.e. the AsyncQueriesEnabled flag is off).
type Handler struct {
	coordinator *Coordinator
	queryFn     QueryFn
}

func NewHandler(coordinator *Coordinator, queryFn QueryFn) *Handler {
	return &Handler{
		coordinator: coordinator,
		queryFn:     queryFn,
	}
}

func (h *Handler) Query(
	ctx context.Context,
	req *connect.Request[queryv1.QueryRequest],
) (*connect.Response[queryv1.QueryResponse], error) {
	resp, err := h.queryFn(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) AsyncQuery(
	ctx context.Context,
	req *connect.Request[queryv1.AsyncQueryRequest],
) (*connect.Response[queryv1.AsyncQueryResponse], error) {
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

	if req.Msg.Query == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("either query or request_id must be set"))
	}
	return h.submit(ctx, tenantID, req.Msg.Query)
}

func (h *Handler) submit(
	ctx context.Context,
	tenantID string,
	req *queryv1.QueryRequest,
) (*connect.Response[queryv1.AsyncQueryResponse], error) {
	// Use a detached context with the tenant injected so the query survives
	// even if the HTTP request context is cancelled when we return the
	// IN_PROGRESS response.
	queryCtx := pyrotenant.InjectTenantID(context.Background(), tenantID)

	resultCh := make(chan QueryResult, 1)
	go func() {
		resp, err := h.queryFn(queryCtx, req)
		resultCh <- QueryResult{Response: resp, Err: err}
	}()

	requestID, err := h.coordinator.PromoteToAsync(ctx, tenantID, resultCh)
	if err != nil {
		return nil, connect.NewError(connect.CodeResourceExhausted, err)
	}

	return connect.NewResponse(&queryv1.AsyncQueryResponse{
		RequestId: requestID,
		Status:    queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS,
	}), nil
}

func (h *Handler) poll(
	ctx context.Context,
	tenantID string,
	requestID string,
) (*connect.Response[queryv1.AsyncQueryResponse], error) {
	result, err := h.coordinator.PollQuery(ctx, tenantID, requestID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if result == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("async query not found"))
	}

	resp := &queryv1.AsyncQueryResponse{
		RequestId: requestID,
	}

	switch result.Metadata.Status {
	case StatusInProgress:
		resp.Status = queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS
	case StatusSuccess:
		resp.Status = queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS
		resp.Response = result.Response
	case StatusFailure:
		resp.Status = queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_FAILURE
		resp.ErrorMessage = result.Metadata.ErrorMessage
	}

	return connect.NewResponse(resp), nil
}
