package async

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// backdateHeartbeat rewrites an existing record's LastHeartbeat, simulating a
// query whose owner stopped renewing its lease some time ago.
func backdateHeartbeat(t *testing.T, ctx context.Context, store *Store, tenantID, requestID string, when time.Time) {
	t.Helper()
	base := store.basePath(tenantID, requestID)
	var meta Metadata
	require.NoError(t, store.readJSON(ctx, base+"metadata.json", &meta))
	meta.LastHeartbeat = when
	require.NoError(t, store.saveJSON(ctx, base+"metadata.json", &meta))
}

type fakeDispatchCall struct {
	tenantID  string
	requestID string
	spec      *querierv1.SelectMergeStacktracesRequest
}

// fakeDispatcher is a test double for Dispatcher. It always records the call;
// onDispatch, if set, runs synchronously afterward. Leaving onDispatch nil
// simulates a dispatcher that declines to do anything, e.g. Coordinator's
// concurrency limit rejecting the adopted query.
type fakeDispatcher struct {
	mu         sync.Mutex
	calls      []fakeDispatchCall
	onDispatch func(tenantID, requestID string, spec *querierv1.SelectMergeStacktracesRequest)
}

func (f *fakeDispatcher) Dispatch(_ context.Context, tenantID, requestID string, spec *querierv1.SelectMergeStacktracesRequest) {
	f.mu.Lock()
	f.calls = append(f.calls, fakeDispatchCall{tenantID: tenantID, requestID: requestID, spec: spec})
	onDispatch := f.onDispatch
	f.mu.Unlock()
	if onDispatch != nil {
		onDispatch(tenantID, requestID, spec)
	}
}

// gatedUploadBucket wraps a Bucket so a test can pause a writer goroutine
// deterministically at "about to write" one specific object, inject a real,
// concurrent write from elsewhere into the gap, then let the paused writer
// proceed. reached closes once the gate is hit; the wrapped Upload blocks
// until proceed is closed.
type gatedUploadBucket struct {
	objstore.Bucket
	target  string
	reached chan struct{}
	proceed chan struct{}
}

func newGatedUploadBucket(inner objstore.Bucket, target string) *gatedUploadBucket {
	return &gatedUploadBucket{
		Bucket:  inner,
		target:  target,
		reached: make(chan struct{}),
		proceed: make(chan struct{}),
	}
}

func (b *gatedUploadBucket) Upload(ctx context.Context, name string, r io.Reader, opts ...objstore.ObjectUploadOption) error {
	if name == b.target {
		close(b.reached)
		<-b.proceed
	}
	return b.Bucket.Upload(ctx, name, r, opts...)
}

func TestMetadataUnmarshal_BackwardCompatible(t *testing.T) {
	raw := []byte(`{
		"request_id": "550e8400-e29b-41d4-a716-446655440000",
		"tenant_id": "tenant-a",
		"status": "in_progress",
		"created_at": "2024-01-01T00:00:00Z",
		"last_heartbeat": "2024-01-01T00:00:05Z"
	}`)

	var meta Metadata
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.Equal(t, StatusInProgress, meta.Status)
	require.Equal(t, "", meta.Owner)
}

func TestStoreCreate_PersistsSpecBeforeMetadata(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}

	require.NoError(t, store.create(ctx, tenantID, requestID, spec))

	base := store.basePath(tenantID, requestID)
	_, err := store.bucket.Attributes(ctx, base+"spec.pb")
	require.NoError(t, err)
	_, err = store.bucket.Attributes(ctx, base+"metadata.json")
	require.NoError(t, err)

	gotSpec, err := store.readSpec(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.True(t, proto.Equal(spec, gotSpec))

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, base+"metadata.json", &meta))
	require.Equal(t, StatusInProgress, meta.Status)
	require.Equal(t, store.ownerID, meta.Owner)
}

func TestStoreGet_StaleWithSpecStaysInProgress(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	require.NoError(t, store.create(ctx, tenantID, requestID, spec))
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusInProgress, result.Metadata.Status)
	require.Empty(t, result.Metadata.ErrorMessage)

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &meta))
	require.Equal(t, StatusInProgress, meta.Status)
	require.Empty(t, meta.ErrorMessage)
}

func TestStoreGet_StaleWithoutSpecPersistsFailure(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	past := time.Now().Add(-time.Hour).UTC()
	meta := &Metadata{
		RequestID:     requestID,
		TenantID:      tenantID,
		Status:        StatusInProgress,
		CreatedAt:     past,
		LastHeartbeat: past,
	}
	require.NoError(t, store.saveJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", meta))

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusFailure, result.Metadata.Status)
	require.Equal(t, errOrphanedNoSpec.Error(), result.Metadata.ErrorMessage)

	var got Metadata
	require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &got))
	require.Equal(t, StatusFailure, got.Status)

	// Idempotent: a second get() is a no-op, still StatusFailure, no error.
	result2, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusFailure, result2.Metadata.Status)
}

// attributesErrBucket wraps a Bucket, forcing Attributes to return a fixed
// error for one specific object name; every other call (and every other
// object name) is delegated unchanged. Used to simulate a transient
// object-storage error that is distinct from "not found".
type attributesErrBucket struct {
	objstore.Bucket
	failName string
	err      error
}

func (b *attributesErrBucket) Attributes(ctx context.Context, name string) (objstore.ObjectAttributes, error) {
	if name == b.failName {
		return objstore.ObjectAttributes{}, b.err
	}
	return b.Bucket.Attributes(ctx, name)
}

func TestStoreGet_TransientSpecCheckErrorLeavesQueryInProgress(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	seed := NewStore(log.NewNopLogger(), inner, nil)
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	require.NoError(t, seed.create(ctx, tenantID, requestID, spec))
	backdateHeartbeat(t, ctx, seed, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	wrapped := &attributesErrBucket{
		Bucket:   inner,
		failName: seed.basePath(tenantID, requestID) + "spec.pb",
		err:      errors.New("simulated transient storage error"),
	}
	store := NewStore(log.NewNopLogger(), wrapped, nil)

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusInProgress, result.Metadata.Status)
	require.Empty(t, result.Metadata.ErrorMessage)

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &meta))
	require.Equal(t, StatusInProgress, meta.Status)
}

func TestStoreAdopt_StaleWithSpecClaimsAndDispatches(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	require.NoError(t, store.create(ctx, tenantID, requestID, spec))
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	store.runAdoption(ctx)

	require.Len(t, dispatcher.calls, 1)
	require.Equal(t, tenantID, dispatcher.calls[0].tenantID)
	require.Equal(t, requestID, dispatcher.calls[0].requestID)
	require.True(t, proto.Equal(spec, dispatcher.calls[0].spec))

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &meta))
	require.Equal(t, StatusInProgress, meta.Status)
	require.Equal(t, store.ownerID, meta.Owner)
	require.WithinDuration(t, time.Now(), meta.LastHeartbeat, 5*time.Second)

	require.Equal(t, float64(1), testutil.ToFloat64(store.adoptionAttempts.WithLabelValues(tenantID)))
	require.Equal(t, float64(1), testutil.ToFloat64(store.adoptionSuccesses.WithLabelValues(tenantID)))
}

func TestStoreAdopt_StaleWithoutSpecMarksFailureNoDispatch(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	past := time.Now().Add(-time.Hour).UTC()
	meta := &Metadata{
		RequestID:     requestID,
		TenantID:      tenantID,
		Status:        StatusInProgress,
		CreatedAt:     past,
		LastHeartbeat: past,
	}
	require.NoError(t, store.saveJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", meta))

	store.runAdoption(ctx)

	require.Empty(t, dispatcher.calls)

	var got Metadata
	require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &got))
	require.Equal(t, StatusFailure, got.Status)
	require.Equal(t, errOrphanedNoSpec.Error(), got.ErrorMessage)

	require.Equal(t, float64(0), testutil.ToFloat64(store.adoptionAttempts.WithLabelValues(tenantID)))
	require.Equal(t, float64(0), testutil.ToFloat64(store.adoptionSuccesses.WithLabelValues(tenantID)))
}

func TestStoreAdopt_CorruptSpecMarksUnadoptable(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	base := store.basePath(tenantID, requestID)
	// An incomplete varint: guaranteed to fail proto.Unmarshal regardless of
	// message schema, simulating a torn/truncated spec.pb.
	require.NoError(t, store.bucket.Upload(ctx, base+"spec.pb", bytes.NewReader([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF})))

	past := time.Now().Add(-time.Hour).UTC()
	meta := &Metadata{
		RequestID:     requestID,
		TenantID:      tenantID,
		Status:        StatusInProgress,
		CreatedAt:     past,
		LastHeartbeat: past,
	}
	require.NoError(t, store.saveJSON(ctx, base+"metadata.json", meta))

	store.runAdoption(ctx)

	require.Empty(t, dispatcher.calls)

	var got Metadata
	require.NoError(t, store.readJSON(ctx, base+"metadata.json", &got))
	require.Equal(t, StatusFailure, got.Status)
	require.Equal(t, errOrphanedNoSpec.Error(), got.ErrorMessage)

	// A corrupt spec is unreadable, not a genuine adoption candidate.
	require.Equal(t, float64(0), testutil.ToFloat64(store.adoptionAttempts.WithLabelValues(tenantID)))
}

func TestStoreAdopt_NilDispatcherIsNoop(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	// dispatcher intentionally left nil: SetDispatcher is never called.

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	require.NoError(t, store.create(ctx, tenantID, requestID, spec))
	stale := time.Now().Add(-time.Hour).UTC()
	backdateHeartbeat(t, ctx, store, tenantID, requestID, stale)

	require.NotPanics(t, func() { store.runAdoption(ctx) })

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &meta))
	require.Equal(t, StatusInProgress, meta.Status)
	require.Equal(t, store.ownerID, meta.Owner)
	require.WithinDuration(t, stale, meta.LastHeartbeat, time.Second)

	// A nil dispatcher is still a stale+spec-present candidate found; it
	// counts as an attempt even though nothing further happens.
	require.Equal(t, float64(1), testutil.ToFloat64(store.adoptionAttempts.WithLabelValues(tenantID)))
}

func TestStoreAdopt_ClaimSucceedsEvenIfDispatcherDeclines(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{} // no onDispatch hook: declines to run anything further.
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	require.NoError(t, store.create(ctx, tenantID, requestID, spec))
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	store.runAdoption(ctx)

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &meta))
	require.Equal(t, store.ownerID, meta.Owner)
	require.WithinDuration(t, time.Now(), meta.LastHeartbeat, 5*time.Second)
	require.Equal(t, float64(1), testutil.ToFloat64(store.adoptionSuccesses.WithLabelValues(tenantID)))
}

func TestStoreAdopt_MultiTenantScanIsIsolated(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const (
		tenantA = "tenant-a"
		tenantB = "tenant-b"
	)
	requestA, requestB := uuid.New().String(), uuid.New().String()
	specA := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", LabelSelector: `{tenant="a"}`}
	specB := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds", LabelSelector: `{tenant="b"}`}

	require.NoError(t, store.create(ctx, tenantA, requestA, specA))
	require.NoError(t, store.create(ctx, tenantB, requestB, specB))
	backdateHeartbeat(t, ctx, store, tenantA, requestA, time.Now().Add(-time.Hour).UTC())
	backdateHeartbeat(t, ctx, store, tenantB, requestB, time.Now().Add(-time.Hour).UTC())

	store.runAdoption(ctx)

	require.Len(t, dispatcher.calls, 2)
	byTenant := make(map[string]fakeDispatchCall, 2)
	for _, c := range dispatcher.calls {
		byTenant[c.tenantID] = c
	}
	require.Equal(t, requestA, byTenant[tenantA].requestID)
	require.Equal(t, requestB, byTenant[tenantB].requestID)
	require.True(t, proto.Equal(specA, byTenant[tenantA].spec))
	require.True(t, proto.Equal(specB, byTenant[tenantB].spec))

	require.Equal(t, float64(1), testutil.ToFloat64(store.adoptionAttempts.WithLabelValues(tenantA)))
	require.Equal(t, float64(1), testutil.ToFloat64(store.adoptionAttempts.WithLabelValues(tenantB)))
	require.Equal(t, float64(1), testutil.ToFloat64(store.adoptionSuccesses.WithLabelValues(tenantA)))
	require.Equal(t, float64(1), testutil.ToFloat64(store.adoptionSuccesses.WithLabelValues(tenantB)))
}

// TestStoreGet_AnchorReportsSuccessDespiteLaterFailureWrite is the headline,
// fully sequential regression test for the primary invariant: once a
// success has landed, get() reports it regardless of any later write.
func TestStoreGet_AnchorReportsSuccessDespiteLaterFailureWrite(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	require.NoError(t, store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{}))

	resp := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")}
	require.NoError(t, store.complete(ctx, tenantID, requestID, resp))
	require.NoError(t, store.fail(ctx, tenantID, requestID, errors.New("late straggler failure")))

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(resp, result.Response))
}

// TestStoreComplete_SuccessOverwritesPriorFailure isolates the guard-polarity
// fix from get()'s anchor: it asserts on the raw metadata.json record
// directly, not through get().
func TestStoreComplete_SuccessOverwritesPriorFailure(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	require.NoError(t, store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{}))

	require.NoError(t, store.fail(ctx, tenantID, requestID, errors.New("first failure")))
	require.NoError(t, store.complete(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesResponse{Tree: []byte("late-success")}))

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &meta))
	require.Equal(t, StatusSuccess, meta.Status)
	require.Empty(t, meta.ErrorMessage)
}

// TestStoreAdopt_SkipsClaimAndDispatchWhenResultAlreadyExists proves the
// early (pre-readSpec) anchor check in adopt(): a candidate that already
// succeeded is skipped entirely, without ever consulting the dispatcher.
func TestStoreAdopt_SkipsClaimAndDispatchWhenResultAlreadyExists(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	require.NoError(t, store.create(ctx, tenantID, requestID, spec))
	require.NoError(t, store.complete(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")}))

	stale := Metadata{
		RequestID:     requestID,
		TenantID:      tenantID,
		Status:        StatusInProgress,
		LastHeartbeat: time.Now().Add(-time.Hour).UTC(),
	}
	store.adopt(ctx, stale)

	require.Empty(t, dispatcher.calls)
}

// TestStoreHeartbeat_CanRevertSoftFailureToInProgress documents an accepted
// residual: a metadata write carrying a stale, already-in-flight read of
// Status: InProgress (captured before a racing fail() lands) can still land
// afterward and revert the failure. Nothing guards against this -- the
// guard only inspects the read that already happened, not what the object
// holds by the time the write lands. This is never a permanent wrong
// answer (the residual state space always converges once every dispatched
// execution has exited), so it must not be "fixed" by a future change.
func TestStoreHeartbeat_CanRevertSoftFailureToInProgress(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	require.NoError(t, store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{}))

	base := store.basePath(tenantID, requestID)
	var staleRead Metadata
	require.NoError(t, store.readJSON(ctx, base+"metadata.json", &staleRead))

	require.NoError(t, store.fail(ctx, tenantID, requestID, errors.New("straggler failure")))

	staleRead.LastHeartbeat = time.Now().UTC()
	staleRead.Owner = store.ownerID
	require.NoError(t, store.saveJSON(ctx, base+"metadata.json", &staleRead))

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, base+"metadata.json", &meta))
	require.Equal(t, StatusInProgress, meta.Status)
}

func TestStoreCompleteFail_SuccessAlwaysWinsOverFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("complete then fail", func(t *testing.T) {
		store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
		const tenantID = "tenant-a"
		requestID := uuid.New().String()
		require.NoError(t, store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{}))

		want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")}
		require.NoError(t, store.complete(ctx, tenantID, requestID, want))
		require.NoError(t, store.fail(ctx, tenantID, requestID, errors.New("late straggler failure")))

		var meta Metadata
		require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &meta))
		require.Equal(t, StatusSuccess, meta.Status)
		require.Empty(t, meta.ErrorMessage)

		result, err := store.get(ctx, tenantID, requestID)
		require.NoError(t, err)
		require.Equal(t, StatusSuccess, result.Metadata.Status)
		require.True(t, proto.Equal(want, result.Response))
	})

	t.Run("fail then complete", func(t *testing.T) {
		store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
		const tenantID = "tenant-a"
		requestID := uuid.New().String()
		require.NoError(t, store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{}))

		require.NoError(t, store.fail(ctx, tenantID, requestID, errors.New("first failure")))
		want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("late-success")}
		require.NoError(t, store.complete(ctx, tenantID, requestID, want))

		// A real success always wins over a prior failure: at least one
		// execution produced the correct answer, so it must be reported --
		// regardless of what any differently-outcome'd execution wrote first.
		var meta Metadata
		require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &meta))
		require.Equal(t, StatusSuccess, meta.Status)
		require.Empty(t, meta.ErrorMessage)

		result, err := store.get(ctx, tenantID, requestID)
		require.NoError(t, err)
		require.Equal(t, StatusSuccess, result.Metadata.Status)
		require.True(t, proto.Equal(want, result.Response))
	})
}

func TestStoreHeartbeat_DoesNotResurrectTerminalRecord(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	require.NoError(t, store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{}))

	want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")}
	require.NoError(t, store.complete(ctx, tenantID, requestID, want))

	// Simulates the original owner's still-alive awaitResult goroutine ticking
	// a heartbeat after a racing execution has already completed the record.
	require.NoError(t, store.heartbeat(ctx, tenantID, requestID))

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &meta))
	require.Equal(t, StatusSuccess, meta.Status)

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(want, result.Response))
}

// TestStoreGet_AnchorSurvivesHeartbeatGatedRevert reproduces a persisted
// success being reverted to in_progress by a stale, concurrently-in-flight
// heartbeat write, then proves get() still reports the success anyway.
func TestStoreGet_AnchorSurvivesHeartbeatGatedRevert(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	seed := NewStore(log.NewNopLogger(), inner, nil)
	require.NoError(t, seed.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{}))

	target := seed.basePath(tenantID, requestID) + "metadata.json"
	gated := newGatedUploadBucket(inner, target)
	heartbeatStore := NewStore(log.NewNopLogger(), gated, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = heartbeatStore.heartbeat(ctx, tenantID, requestID)
	}()
	<-gated.reached

	resp := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")}
	completeStore := NewStore(log.NewNopLogger(), inner, nil)
	require.NoError(t, completeStore.complete(ctx, tenantID, requestID, resp))

	close(gated.proceed)
	wg.Wait()

	// Informational only: proves the interleaving actually landed -- the raw
	// record was reverted by the stale heartbeat write.
	var raw Metadata
	require.NoError(t, seed.readJSON(ctx, seed.basePath(tenantID, requestID)+"metadata.json", &raw))
	require.Equal(t, StatusInProgress, raw.Status)

	result, err := seed.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(resp, result.Response))
}

// TestStoreAdopt_ClaimGatedByConcurrentCompletion reproduces a concurrent
// completion landing while adopt()'s claim write is in flight. Only the
// primary (client-visible, via get()) invariant is asserted -- gating at the
// claim Upload itself lands in the accepted, timing-dependent sliver where a
// stale claim and a wasted redispatch are tolerated.
func TestStoreAdopt_ClaimGatedByConcurrentCompletion(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	seed := NewStore(log.NewNopLogger(), inner, nil)
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	require.NoError(t, seed.create(ctx, tenantID, requestID, spec))
	backdateHeartbeat(t, ctx, seed, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	var snapshot Metadata
	require.NoError(t, seed.readJSON(ctx, seed.basePath(tenantID, requestID)+"metadata.json", &snapshot))

	target := seed.basePath(tenantID, requestID) + "metadata.json"
	gated := newGatedUploadBucket(inner, target)
	adoptStore := NewStore(log.NewNopLogger(), gated, nil)
	adoptStore.SetDispatcher(&fakeDispatcher{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		adoptStore.adopt(ctx, snapshot)
	}()
	<-gated.reached

	resp := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("already-done")}
	completeStore := NewStore(log.NewNopLogger(), inner, nil)
	require.NoError(t, completeStore.complete(ctx, tenantID, requestID, resp))

	close(gated.proceed)
	wg.Wait()

	result, err := seed.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(resp, result.Response))
}

func TestStoreAdopt_ReReadAbortsOnConcurrentCompletion(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	require.NoError(t, store.create(ctx, tenantID, requestID, spec))
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	// Snapshot metadata the way runAdoption's scan would, then simulate the
	// true owner completing the query while adopt()'s readSpec round trip is
	// "in flight" -- i.e. after the scan's read but before the claim write.
	var stale Metadata
	require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &stale))
	require.NoError(t, store.complete(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesResponse{Tree: []byte("already-done")}))

	store.adopt(ctx, stale)

	require.Empty(t, dispatcher.calls)

	var final Metadata
	require.NoError(t, store.readJSON(ctx, store.basePath(tenantID, requestID)+"metadata.json", &final))
	require.Equal(t, StatusSuccess, final.Status)

	// result.pb already exists by the time adopt() runs, so its entry-point
	// anchor check short-circuits before readSpec/adoptionAttempts is ever
	// reached -- a strictly earlier abort than the re-read this test was
	// originally named for, with the same net outcome (no dispatch).
	require.Equal(t, float64(0), testutil.ToFloat64(store.adoptionAttempts.WithLabelValues(tenantID)))
	require.Equal(t, float64(0), testutil.ToFloat64(store.adoptionSuccesses.WithLabelValues(tenantID)))
}

func TestStoreAdopt_ConcurrentRaceProducesSingleTerminalStatus(t *testing.T) {
	ctx := context.Background()
	bucket := objstore.NewInMemBucket()
	storeA := NewStore(log.NewNopLogger(), bucket, nil)
	storeB := NewStore(log.NewNopLogger(), bucket, nil)
	storeA.ownerID = "owner-a"
	storeB.ownerID = "owner-b"

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	require.NoError(t, storeA.create(ctx, tenantID, requestID, spec))
	backdateHeartbeat(t, ctx, storeA, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	// Simulates two independent re-executions of the same adopted query
	// finishing around the same time, one on each store. Which action (the
	// success path or the failure path) actually reaches the store first is
	// randomized per run rather than fixed, so both orderings are exercised
	// across repeated runs: whichever terminal write lands first must win,
	// regardless of which action it is.
	successWritesFirst := rand.Intn(2) == 0

	var dispatchCount atomic.Int32
	var completeCalled atomic.Bool
	var writesDone sync.WaitGroup
	raceDispatcher := &fakeDispatcher{
		onDispatch: func(_, _ string, _ *querierv1.SelectMergeStacktracesRequest) {
			isFirstDispatch := dispatchCount.Inc() == 1
			doComplete := isFirstDispatch == successWritesFirst
			delay := time.Millisecond
			if !isFirstDispatch {
				delay = 50 * time.Millisecond
			}
			writesDone.Add(1)
			go func() {
				defer writesDone.Done()
				time.Sleep(delay)
				if doComplete {
					_ = storeA.complete(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")})
					completeCalled.Store(true)
				} else {
					_ = storeB.fail(ctx, tenantID, requestID, errors.New("simulated concurrent failure"))
				}
			}()
		},
	}
	storeA.SetDispatcher(raceDispatcher)
	storeB.SetDispatcher(raceDispatcher)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); storeA.runAdoption(ctx) }()
	go func() { defer wg.Done(); storeB.runAdoption(ctx) }()
	wg.Wait()

	// Both writers -- not just whichever is faster -- must have actually
	// attempted their write before asserting first-terminal-wins semantics;
	// otherwise a reintroduced clobber by the slower writer would go unnoticed.
	writesDone.Wait()

	// Order-independent invariant: if either racer succeeded, the query is
	// genuinely successful and must be reported as such regardless of which
	// terminal write physically landed last; only if neither racer ever
	// called complete() may the final answer be failure.
	wantStatus := StatusFailure
	if completeCalled.Load() {
		wantStatus = StatusSuccess
	}
	result, err := storeA.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, wantStatus, result.Metadata.Status,
		"a real success must be reported regardless of write ordering")
}
