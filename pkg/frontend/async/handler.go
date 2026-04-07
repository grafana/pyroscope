package async

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
)

// Handler implements the AsyncQuerierServiceHandler interface.
type Handler struct {
	coordinator *Coordinator
	upstream    querierv1connect.QuerierServiceHandler
}

func NewHandler(coordinator *Coordinator, upstream querierv1connect.QuerierServiceHandler) *Handler {
	return &Handler{
		coordinator: coordinator,
		upstream:    upstream,
	}
}

func (h *Handler) SelectMergeProfile(
	ctx context.Context,
	req *connect.Request[querierv1.SelectMergeProfileAsyncRequest],
) (*connect.Response[querierv1.SelectMergeProfileAsyncResponse], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	tenantID := tenant.JoinTenantIDs(tenantIDs)

	if req.Msg.RequestId != "" {
		return h.poll(ctx, tenantID, req.Msg.RequestId)
	}
	return h.start(ctx, tenantID, req)
}

func (h *Handler) start(
	ctx context.Context,
	tenantID string,
	req *connect.Request[querierv1.SelectMergeProfileAsyncRequest],
) (*connect.Response[querierv1.SelectMergeProfileAsyncResponse], error) {
	// Build the closure that performs the actual synchronous query.
	// We copy the request headers so the downstream handler sees the tenant context.
	headers := req.Header().Clone()
	queryFn := func(ctx context.Context) (*profilev1.Profile, error) {
		syncReq := connect.NewRequest(req.Msg.Request)
		for k, vals := range headers {
			for _, v := range vals {
				syncReq.Header().Add(k, v)
			}
		}
		resp, err := h.upstream.SelectMergeProfile(ctx, syncReq)
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}

	requestID, err := h.coordinator.StartQuery(ctx, tenantID, queryFn)
	if err != nil {
		return nil, connect.NewError(connect.CodeResourceExhausted, err)
	}

	return connect.NewResponse(&querierv1.SelectMergeProfileAsyncResponse{
		RequestId: requestID,
		Status:    querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS,
	}), nil
}

func (h *Handler) poll(
	ctx context.Context,
	tenantID string,
	requestID string,
) (*connect.Response[querierv1.SelectMergeProfileAsyncResponse], error) {
	result, err := h.coordinator.PollQuery(ctx, tenantID, requestID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if result == nil {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}

	resp := &querierv1.SelectMergeProfileAsyncResponse{
		RequestId: requestID,
	}

	switch result.Metadata.Status {
	case StatusInProgress:
		resp.Status = querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_IN_PROGRESS
	case StatusSuccess:
		resp.Status = querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_SUCCESS
		resp.Profile = result.Profile
	case StatusFailure:
		resp.Status = querierv1.AsyncQueryStatus_ASYNC_QUERY_STATUS_FAILURE
		resp.ErrorMessage = result.Metadata.ErrorMessage
	}

	return connect.NewResponse(resp), nil
}
