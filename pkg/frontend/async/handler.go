package async

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	pyrotenant "github.com/grafana/pyroscope/v2/pkg/tenant"
)

// QueryFn executes a query and returns the response.
type QueryFn func(ctx context.Context, req *queryv1.QueryRequest) (*queryv1.QueryResponse, error)

// HandlerLimits is the subset of limits needed by the handler.
type HandlerLimits interface {
	Limits
	AsyncQueryThreshold(tenantID string) time.Duration
}

// Handler implements queryv1connect.QueryFrontendServiceHandler.
type Handler struct {
	coordinator *Coordinator
	queryFn     QueryFn
	limits      HandlerLimits
}

func NewHandler(coordinator *Coordinator, limits HandlerLimits, queryFn QueryFn) *Handler {
	return &Handler{
		coordinator: coordinator,
		queryFn:     queryFn,
		limits:      limits,
	}
}

func (h *Handler) Query(
	ctx context.Context,
	req *connect.Request[queryv1.QueryRequest],
) (*connect.Response[queryv1.QueryResponse], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	tenantID := tenant.JoinTenantIDs(tenantIDs)

	if req.Msg.RequestId != "" {
		return h.poll(ctx, tenantID, req.Msg.RequestId)
	}

	return h.execute(ctx, tenantID, req.Msg)
}

func (h *Handler) execute(
	ctx context.Context,
	tenantID string,
	req *queryv1.QueryRequest,
) (*connect.Response[queryv1.QueryResponse], error) {
	// Use a detached context with the tenant injected so the query survives
	// even if the HTTP request context is cancelled (which happens when we
	// return an async IN_PROGRESS response).
	queryCtx := pyrotenant.InjectTenantID(context.Background(), tenantID)

	resultCh := make(chan QueryResult, 1)
	go func() {
		resp, err := h.queryFn(queryCtx, req)
		resultCh <- QueryResult{Response: resp, Err: err}
	}()

	// If the client explicitly requested async, promote immediately.
	if req.Async {
		return h.promoteToAsync(ctx, tenantID, resultCh)
	}

	// Otherwise, wait up to the threshold for a sync response.
	threshold := h.limits.AsyncQueryThreshold(tenantID)
	if threshold <= 0 {
		// Auto-async disabled: wait for the result synchronously.
		res := <-resultCh
		if res.Err != nil {
			return nil, res.Err
		}
		return connect.NewResponse(res.Response), nil
	}

	timer := time.NewTimer(threshold)
	defer timer.Stop()

	select {
	case res := <-resultCh:
		if res.Err != nil {
			return nil, res.Err
		}
		return connect.NewResponse(res.Response), nil
	case <-timer.C:
		return h.promoteToAsync(ctx, tenantID, resultCh)
	}
}

func (h *Handler) promoteToAsync(
	ctx context.Context,
	tenantID string,
	resultCh <-chan QueryResult,
) (*connect.Response[queryv1.QueryResponse], error) {
	requestID, err := h.coordinator.PromoteToAsync(ctx, tenantID, resultCh)
	if err != nil {
		return nil, connect.NewError(connect.CodeResourceExhausted, err)
	}

	return connect.NewResponse(&queryv1.QueryResponse{
		RequestId: requestID,
		Status:    queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS,
	}), nil
}

func (h *Handler) poll(
	ctx context.Context,
	tenantID string,
	requestID string,
) (*connect.Response[queryv1.QueryResponse], error) {
	result, err := h.coordinator.PollQuery(ctx, tenantID, requestID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if result == nil {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}

	resp := &queryv1.QueryResponse{
		RequestId: requestID,
	}

	switch result.Metadata.Status {
	case StatusInProgress:
		resp.Status = queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS
	case StatusSuccess:
		resp.Status = queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS
		if result.Response != nil {
			resp.Reports = result.Response.Reports
		}
	case StatusFailure:
		resp.Status = queryv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_FAILURE
		resp.ErrorMessage = result.Metadata.ErrorMessage
	}

	return connect.NewResponse(resp), nil
}
