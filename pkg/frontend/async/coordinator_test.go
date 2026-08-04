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

// fakeQuerierHandler records SelectMergeStacktraces calls and returns a
// canned response or error; if block is set, calls wait for it to close.
type fakeQuerierHandler struct {
	querierv1connect.UnimplementedQuerierServiceHandler

	resp  *querierv1.SelectMergeStacktracesResponse
	err   error
	block <-chan struct{}
	// returned, if set, is closed when a call exits, so tests can order
	// events on the handler's completion rather than on scheduling.
	returned chan struct{}

	mu    sync.Mutex
	calls []*querierv1.SelectMergeStacktracesRequest
}

func (f *fakeQuerierHandler) SelectMergeStacktraces(
	ctx context.Context,
	req *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	f.mu.Lock()
	f.calls = append(f.calls, req.Msg)
	f.mu.Unlock()

	if f.returned != nil {
		defer close(f.returned)
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
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
	logger := log.NewNopLogger()
	store := NewStore(logger, objstore.NewInMemBucket(), nil)

	fakeNext := &fakeQuerierHandler{resp: &querierv1.SelectMergeStacktracesResponse{}}
	coordinator := NewCoordinator(logger, store, fixedLimits{max: 1}, fakeNext, nil)

	const tenantID = "tenant-a"
	require.NoError(t, coordinator.tryAcquire(tenantID))

	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	coordinator.Dispatch(tenantID, "adopted-request-id", leaseHandle{}, spec)

	require.Empty(t, fakeNext.callsSnapshot())
}

// TestCoordinatorHasCapacity_ReflectsInFlightWithoutMutating proves
// HasCapacity is a pure peek: it tracks capacity as tryAcquire consumes it,
// but never itself changes inFlight or the gauge.
func TestCoordinatorHasCapacity_ReflectsInFlightWithoutMutating(t *testing.T) {
	logger := log.NewNopLogger()
	store := NewStore(logger, objstore.NewInMemBucket(), nil)
	fakeNext := &fakeQuerierHandler{resp: &querierv1.SelectMergeStacktracesResponse{}}
	coordinator := NewCoordinator(logger, store, fixedLimits{max: 1}, fakeNext, nil)

	const tenantID = "tenant-a"
	require.True(t, coordinator.HasCapacity(tenantID))
	require.True(t, coordinator.HasCapacity(tenantID), "a peek must not itself consume capacity")

	require.NoError(t, coordinator.tryAcquire(tenantID))
	require.False(t, coordinator.HasCapacity(tenantID))
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
	_, err := store.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
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

// On errLostOwnership, awaitResult must cancel the still-running query and
// release the concurrency slot instead of pinning it forever.
func TestCoordinatorDispatch_LostOwnershipCancelsQueryAndReleasesSlot(t *testing.T) {
	ctx := context.Background()
	logger := log.NewNopLogger()
	store := NewStore(logger, objstore.NewInMemBucket(), nil)
	store.heartbeatInterval = 5 * time.Millisecond

	block := make(chan struct{}) // deliberately never closed
	fakeNext := &fakeQuerierHandler{resp: &querierv1.SelectMergeStacktracesResponse{}, block: block}
	coordinator := NewCoordinator(logger, store, fixedLimits{max: 1}, fakeNext, nil)

	const tenantID = "tenant-a"
	requestID, err := coordinator.Submit(ctx, tenantID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)
	require.False(t, coordinator.HasCapacity(tenantID), "the submitted query holds the only slot")

	// Simulate another store's adoption claim landing: stamp a different
	// owner token directly, as adopt()'s CAS write would.
	metaPath := store.buildPath(tenantID, requestID, metadataFilename)
	var meta Metadata
	require.NoError(t, store.readJSON(ctx, metaPath, &meta))
	meta.Owner = "other-store/deadbeef"
	require.NoError(t, store.saveJSON(ctx, metaPath, &meta))

	// The next heartbeat tick observes errLostOwnership, cancels the still-
	// blocked query, and releases the slot -- without block ever closing.
	require.Eventually(t, func() bool {
		return coordinator.HasCapacity(tenantID)
	}, time.Second, 5*time.Millisecond, "slot must be released once ownership is lost, even though the query never completes naturally")
}

// A query that has already completed when lost ownership is detected must be
// reported, not discarded: the query finishes while the losing heartbeat
// write is deliberately held in flight.
func TestCoordinatorDispatch_LostOwnershipReportsCompletedResult(t *testing.T) {
	ctx := context.Background()
	logger := log.NewNopLogger()
	gate := newGatedConditionalUploadBucket(objstore.NewInMemBucket())
	store := NewStore(logger, gate, nil)
	store.heartbeatInterval = 5 * time.Millisecond

	want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("tree")}
	block := make(chan struct{})
	fakeNext := &fakeQuerierHandler{resp: want, block: block, returned: make(chan struct{})}
	coordinator := NewCoordinator(logger, store, fixedLimits{max: 1}, fakeNext, nil)

	const tenantID = "tenant-a"
	requestID, err := coordinator.Submit(ctx, tenantID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)

	// The first heartbeat tick has read the record (observing the original
	// owner) and is now blocked mid-write, holding that now-stale version.
	<-gate.reached

	// Simulate another store's adoption claim landing in the gap: stamp a
	// different owner token directly, as adopt()'s CAS write would.
	metaPath := store.buildPath(tenantID, requestID, metadataFilename)
	var meta Metadata
	require.NoError(t, store.readJSON(ctx, metaPath, &meta))
	meta.Owner = "other-store/deadbeef"
	require.NoError(t, store.saveJSON(ctx, metaPath, &meta))

	// Let the query finish before the supervisor's cancel can interrupt it:
	// once the handler has returned, its successful outcome is fixed, and
	// the supervisor's blocking drain picks it up whenever the send lands.
	close(block)
	<-fakeNext.returned
	close(gate.proceed)

	require.Eventually(t, func() bool {
		result, err := store.get(ctx, tenantID, requestID)
		return err == nil && result != nil && result.Metadata.Status == StatusSuccess
	}, time.Second, 5*time.Millisecond, "a completed result must be reported even when ownership is lost")

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.True(t, proto.Equal(want, result.Response))
}
