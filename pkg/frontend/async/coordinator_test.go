package async

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
)

// fakeQuerierHandler is a QuerierServiceHandler that records every
// SelectMergeStacktraces call it receives and returns a canned response or
// error. If block is set, calls wait for it to close before returning,
// letting tests hold a dispatched query open to exercise concurrency
// limits.
type fakeQuerierHandler struct {
	querierv1connect.UnimplementedQuerierServiceHandler

	resp  *querierv1.SelectMergeStacktracesResponse
	err   error
	block <-chan struct{}

	mu    sync.Mutex
	calls []*querierv1.SelectMergeStacktracesRequest
}

func (f *fakeQuerierHandler) SelectMergeStacktraces(
	_ context.Context,
	req *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	f.mu.Lock()
	f.calls = append(f.calls, req.Msg)
	f.mu.Unlock()

	if f.block != nil {
		<-f.block
	}
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(f.resp), nil
}

func (f *fakeQuerierHandler) callsSnapshot() []*querierv1.SelectMergeStacktracesRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*querierv1.SelectMergeStacktracesRequest(nil), f.calls...)
}

// fixedLimits is a Limits fake allowing exactly max concurrent async
// queries, the same for every tenant.
type fixedLimits struct {
	max int
}

func (l fixedLimits) MaxAsyncQueryConcurrency(string) int { return l.max }

// unlimitedLimits is a Limits fake that never rejects a submit or dispatch.
type unlimitedLimits struct{}

func (unlimitedLimits) MaxAsyncQueryConcurrency(string) int { return 1 << 30 }

func TestCoordinatorSubmit_PersistsSpecStripsAsyncAndDispatches(t *testing.T) {
	ctx := context.Background()
	logger := log.NewNopLogger()
	store := NewStore(logger, objstore.NewInMemBucket(), nil)

	want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("tree")}
	fakeNext := &fakeQuerierHandler{resp: want}
	coordinator := NewCoordinator(logger, store, fixedLimits{max: 5}, fakeNext, nil)

	const tenantID = "tenant-a"
	req := &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		Async:         &querierv1.AsyncQueryRequest{Type: querierv1.AsyncQueryType_ASYNC_QUERY_TYPE_FORCE},
	}

	requestID, err := coordinator.Submit(ctx, tenantID, req)
	require.NoError(t, err)
	require.NotEmpty(t, requestID)

	require.Eventually(t, func() bool {
		result, err := store.get(ctx, tenantID, requestID)
		return err == nil && result != nil && result.Metadata.Status == StatusSuccess
	}, time.Second, 10*time.Millisecond)

	calls := fakeNext.callsSnapshot()
	require.Len(t, calls, 1)
	require.Nil(t, calls[0].GetAsync())

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.True(t, proto.Equal(want, result.Response))
}

func TestCoordinatorSubmit_ConcurrencyLimitRejects(t *testing.T) {
	ctx := context.Background()
	logger := log.NewNopLogger()
	store := NewStore(logger, objstore.NewInMemBucket(), nil)

	block := make(chan struct{})
	defer close(block)
	fakeNext := &fakeQuerierHandler{resp: &querierv1.SelectMergeStacktracesResponse{}, block: block}
	coordinator := NewCoordinator(logger, store, fixedLimits{max: 1}, fakeNext, nil)

	const tenantID = "tenant-a"
	req := &querierv1.SelectMergeStacktracesRequest{}

	firstID, err := coordinator.Submit(ctx, tenantID, req)
	require.NoError(t, err)
	require.NotEmpty(t, firstID)

	_, err = coordinator.Submit(ctx, tenantID, req)
	require.Error(t, err)
}

func TestCoordinatorDispatch_ConcurrencyLimitSkipsExecution(t *testing.T) {
	ctx := context.Background()
	logger := log.NewNopLogger()
	store := NewStore(logger, objstore.NewInMemBucket(), nil)

	fakeNext := &fakeQuerierHandler{resp: &querierv1.SelectMergeStacktracesResponse{}}
	coordinator := NewCoordinator(logger, store, fixedLimits{max: 1}, fakeNext, nil)

	const tenantID = "tenant-a"
	require.NoError(t, coordinator.tryAcquire(tenantID))

	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	coordinator.Dispatch(ctx, tenantID, "adopted-request-id", spec)

	require.Empty(t, fakeNext.callsSnapshot())
}

func TestCoordinatorSubmit_TenantIsolation(t *testing.T) {
	ctx := context.Background()
	logger := log.NewNopLogger()
	store := NewStore(logger, objstore.NewInMemBucket(), nil)

	block := make(chan struct{})
	defer close(block)
	fakeNext := &fakeQuerierHandler{resp: &querierv1.SelectMergeStacktracesResponse{}, block: block}
	coordinator := NewCoordinator(logger, store, fixedLimits{max: 1}, fakeNext, nil)

	req := &querierv1.SelectMergeStacktracesRequest{}

	tenantAID, err := coordinator.Submit(ctx, "tenant-a", req)
	require.NoError(t, err)
	require.NotEmpty(t, tenantAID)

	tenantBID, err := coordinator.Submit(ctx, "tenant-b", req)
	require.NoError(t, err)
	require.NotEmpty(t, tenantBID)
}

func TestAdoption_EndToEnd_ResumesOrphanedQuery(t *testing.T) {
	ctx := context.Background()
	logger := log.NewNopLogger()
	bucket := objstore.NewInMemBucket()
	store := NewStore(logger, bucket, nil)

	fakeNext := &fakeQuerierHandler{resp: &querierv1.SelectMergeStacktracesResponse{Tree: []byte("t")}}
	coordinator := NewCoordinator(logger, store, unlimitedLimits{}, fakeNext, nil)
	store.SetDispatcher(coordinator)

	const (
		tenantID  = "tenant-a"
		requestID = "550e8400-e29b-41d4-a716-446655440002"
	)
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	require.NoError(t, store.create(ctx, tenantID, requestID, spec))
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour))

	store.runAdoption(ctx)

	require.Eventually(t, func() bool {
		r, err := store.get(ctx, tenantID, requestID)
		return err == nil && r != nil && r.Metadata.Status != StatusInProgress
	}, time.Second, 10*time.Millisecond)

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(fakeNext.resp, result.Response))
	require.Len(t, fakeNext.callsSnapshot(), 1)
}
