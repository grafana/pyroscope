package async

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// ownerA and ownerB are distinct owner IDs used across tests that pit two
// stores against each other for the same record.
const (
	ownerA = "owner-a"
	ownerB = "owner-b"
)

// backdateHeartbeat rewrites an existing record's LastHeartbeat, simulating a
// query whose owner stopped renewing its lease some time ago.
func backdateHeartbeat(t *testing.T, ctx context.Context, store *Store, tenantID, requestID string, when time.Time) {
	t.Helper()
	metaPath := store.buildPath(tenantID, requestID, metadataFilename)
	var meta Metadata
	require.NoError(t, store.readJSON(ctx, metaPath, &meta))
	meta.LastHeartbeat = when
	require.NoError(t, store.saveJSON(ctx, metaPath, &meta))
}

// readLease builds a leaseHandle from the record's current on-disk state,
// modeling a second, independent writer with its own fresh read.
//
//nolint:unparam
func readLease(t *testing.T, ctx context.Context, store *Store, tenantID, requestID string) leaseHandle {
	t.Helper()
	metaPath := store.buildPath(tenantID, requestID, metadataFilename)
	var meta Metadata
	version, err := store.readMetadataVersioned(ctx, metaPath, &meta)
	require.NoError(t, err)
	return leaseHandle{meta: meta, version: version}
}

type fakeDispatchCall struct {
	tenantID  string
	requestID string
	lease     leaseHandle
	spec      *querierv1.SelectMergeStacktracesRequest
}

// fakeDispatcher records Dispatch calls. atCapacity makes HasCapacity report
// a saturated tenant; onDispatch, if set, runs synchronously after each call.
type fakeDispatcher struct {
	mu               sync.Mutex
	calls            []fakeDispatchCall
	hasCapacityCalls int
	atCapacity       bool // when true, HasCapacity reports no spare capacity; default (false) has capacity.
	onDispatch       func(tenantID, requestID string, spec *querierv1.SelectMergeStacktracesRequest)
}

func (f *fakeDispatcher) HasCapacity(tenantID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hasCapacityCalls++
	return !f.atCapacity
}

func (f *fakeDispatcher) Dispatch(tenantID, requestID string, lease leaseHandle, spec *querierv1.SelectMergeStacktracesRequest) {
	f.mu.Lock()
	f.calls = append(f.calls, fakeDispatchCall{tenantID: tenantID, requestID: requestID, lease: lease, spec: spec})
	onDispatch := f.onDispatch
	f.mu.Unlock()
	if onDispatch != nil {
		onDispatch(tenantID, requestID, spec)
	}
}

// gatedUploadBucket pauses the first Upload of target so a test can inject a
// concurrent write into the gap: reached closes at the gate, the paused
// write continues once proceed closes. One-shot, so retries just block.
type gatedUploadBucket struct {
	objstore.Bucket
	target  string
	once    sync.Once
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
		b.once.Do(func() { close(b.reached) })
		<-b.proceed
	}
	return b.Bucket.Upload(ctx, name, r, opts...)
}

// barrierUploadBucket blocks Uploads of target until n callers have arrived,
// then releases them together -- forcing racing writers into the same claim
// window instead of relying on scheduling luck. Later calls pass through.
type barrierUploadBucket struct {
	objstore.Bucket
	target  string
	n       int
	mu      sync.Mutex
	count   int
	release chan struct{}
}

func newBarrierUploadBucket(inner objstore.Bucket, target string, n int) *barrierUploadBucket {
	return &barrierUploadBucket{Bucket: inner, target: target, n: n, release: make(chan struct{})}
}

func (b *barrierUploadBucket) Upload(ctx context.Context, name string, r io.Reader, opts ...objstore.ObjectUploadOption) error {
	if name == b.target {
		b.mu.Lock()
		b.count++
		last := b.count == b.n
		b.mu.Unlock()
		if last {
			close(b.release)
		}
		<-b.release
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
	require.Equal(t, 0, meta.AdoptionCount)
}

func TestStoreCreate_PersistsSpecBeforeMetadata(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}

	lease, err := store.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)

	_, err = store.bucket.Attributes(ctx, store.buildPath(tenantID, requestID, specFilename))
	require.NoError(t, err)
	_, err = store.bucket.Attributes(ctx, store.buildPath(tenantID, requestID, metadataFilename))
	require.NoError(t, err)

	gotSpec, err := store.readSpec(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.True(t, proto.Equal(spec, gotSpec))

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, StatusInProgress, meta.Status)
	require.Equal(t, lease.meta.Owner, meta.Owner)
}

// A persistent Attributes failure after create()'s write must propagate, not
// seed a nil-version lease that would write unfenced for the record's life.
func TestStoreCreate_TransientVersionReadFailureDoesNotSeedUnfencedLease(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()
	observer := NewStore(log.NewNopLogger(), inner, nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	metaPath := observer.buildPath(tenantID, requestID, metadataFilename)

	wrapped := &attributesErrBucket{
		Bucket:   inner,
		failName: metaPath,
		err:      errors.New("simulated transient storage error"),
	}
	store := NewStore(log.NewNopLogger(), wrapped, nil)

	_, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.Error(t, err)

	// The metadata write itself landed -- create() uploads before ever
	// seeding a version -- only the lease this call would have returned is
	// withheld.
	var meta Metadata
	require.NoError(t, observer.readJSON(ctx, metaPath, &meta))
	require.Equal(t, StatusInProgress, meta.Status)
}

func TestStoreGet_StaleWithSpecStaysInProgress(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	_, err := store.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusInProgress, result.Metadata.Status)
	require.Empty(t, result.Metadata.ErrorMessage)

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
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
	require.NoError(t, store.saveJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), meta))

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusFailure, result.Metadata.Status)
	require.Equal(t, errOrphanedNoSpec.Error(), result.Metadata.ErrorMessage)

	var got Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &got))
	require.Equal(t, StatusFailure, got.Status)

	// Idempotent: a second get() is a no-op, still StatusFailure, no error.
	result2, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusFailure, result2.Metadata.Status)
}

// attributesErrBucket fails Attributes for one object name with a fixed
// error distinct from "not found".
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

// attributesFailAfterFirstBucket lets the first Attributes call on one
// object succeed and fails every later one.
type attributesFailAfterFirstBucket struct {
	objstore.Bucket
	failName string
	err      error
	calls    int
}

func (b *attributesFailAfterFirstBucket) Attributes(ctx context.Context, name string) (objstore.ObjectAttributes, error) {
	if name == b.failName {
		b.calls++
		if b.calls > 1 {
			return objstore.ObjectAttributes{}, b.err
		}
	}
	return b.Bucket.Attributes(ctx, name)
}

// transientErrorOnceBucket fails the next Upload with a plain non-condition
// error, then delegates.
type transientErrorOnceBucket struct {
	objstore.Bucket
	failNext bool
	err      error
}

func (b *transientErrorOnceBucket) Upload(ctx context.Context, name string, r io.Reader, opts ...objstore.ObjectUploadOption) error {
	if b.failNext {
		b.failNext = false
		return b.err
	}
	return b.Bucket.Upload(ctx, name, r, opts...)
}

func TestStoreGet_TransientSpecCheckErrorLeavesQueryInProgress(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	seed := NewStore(log.NewNopLogger(), inner, nil)
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	_, err := seed.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, seed, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	wrapped := &attributesErrBucket{
		Bucket:   inner,
		failName: seed.buildPath(tenantID, requestID, specFilename),
		err:      errors.New("simulated transient storage error"),
	}
	store := NewStore(log.NewNopLogger(), wrapped, nil)

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusInProgress, result.Metadata.Status)
	require.Empty(t, result.Metadata.ErrorMessage)

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
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
	originalToken, err := store.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	store.runAdoption(ctx)

	require.Len(t, dispatcher.calls, 1)
	require.Equal(t, tenantID, dispatcher.calls[0].tenantID)
	require.Equal(t, requestID, dispatcher.calls[0].requestID)
	require.True(t, proto.Equal(spec, dispatcher.calls[0].spec))
	require.NotEqual(t, originalToken.meta.Owner, dispatcher.calls[0].lease.meta.Owner, "adoption must mint a fresh owner token, not reuse the original")

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, StatusInProgress, meta.Status)
	require.Equal(t, dispatcher.calls[0].lease.meta.Owner, meta.Owner)
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
	require.NoError(t, store.saveJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), meta))

	store.runAdoption(ctx)

	require.Empty(t, dispatcher.calls)

	var got Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &got))
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
	// An incomplete varint: guaranteed to fail proto.Unmarshal regardless of
	// message schema, simulating a torn/truncated spec.pb.
	require.NoError(t, store.bucket.Upload(ctx, store.buildPath(tenantID, requestID, specFilename), bytes.NewReader([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF})))

	past := time.Now().Add(-time.Hour).UTC()
	meta := &Metadata{
		RequestID:     requestID,
		TenantID:      tenantID,
		Status:        StatusInProgress,
		CreatedAt:     past,
		LastHeartbeat: past,
	}
	require.NoError(t, store.saveJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), meta))

	store.runAdoption(ctx)

	require.Empty(t, dispatcher.calls)

	var got Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &got))
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
	lease, err := store.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	stale := time.Now().Add(-time.Hour).UTC()
	backdateHeartbeat(t, ctx, store, tenantID, requestID, stale)

	require.NotPanics(t, func() { store.runAdoption(ctx) })

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, StatusInProgress, meta.Status)
	require.Equal(t, lease.meta.Owner, meta.Owner)
	require.WithinDuration(t, stale, meta.LastHeartbeat, time.Second)

	// A nil dispatcher is checked before the capacity peek or any record
	// read, so nothing here ever reaches the point where an attempt is
	// counted.
	require.Equal(t, float64(0), testutil.ToFloat64(store.adoptionAttempts.WithLabelValues(tenantID)))
}

// At capacity, adopt() must not touch the record at all -- no claim, no
// AdoptionCount spend -- so a frontend with spare capacity can adopt it on
// the very next scan.
func TestStoreAdopt_AtCapacityLeavesRecordUntouched(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{atCapacity: true}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	_, err := store.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	var before Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &before))
	stale := time.Now().Add(-time.Hour).UTC()
	backdateHeartbeat(t, ctx, store, tenantID, requestID, stale)

	store.runAdoption(ctx)

	require.Empty(t, dispatcher.calls)
	require.Greater(t, dispatcher.hasCapacityCalls, 0, "HasCapacity must still be consulted")

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, before.Owner, meta.Owner)
	require.WithinDuration(t, stale, meta.LastHeartbeat, time.Second)
	require.Equal(t, 0, meta.AdoptionCount)
	require.Equal(t, StatusInProgress, meta.Status)
	require.Equal(t, float64(0), testutil.ToFloat64(store.adoptionSuccesses.WithLabelValues(tenantID)))
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

	_, err := store.create(ctx, tenantA, requestA, specA)
	require.NoError(t, err)
	_, err = store.create(ctx, tenantB, requestB, specB)
	require.NoError(t, err)
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

// TestStoreAdopt_AdoptionCountIncrementsAcrossClaims proves AdoptionCount
// rides the same metadata write as each successful claim, rather than being
// tracked separately.
func TestStoreAdopt_AdoptionCountIncrementsAcrossClaims(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	_, err := store.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)

	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())
	store.runAdoption(ctx)

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, 1, meta.AdoptionCount)

	// The fakeDispatcher never renews the lease, so the record is stale again
	// and eligible for a second adoption.
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())
	store.runAdoption(ctx)

	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, 2, meta.AdoptionCount)
	require.Equal(t, StatusInProgress, meta.Status)
	require.Len(t, dispatcher.calls, 2)
}

// At defaultMaxAdoptions, a further stale lease is marked failed instead of
// claimed and dispatched again.
func TestStoreAdopt_ExceedsMaxAdoptionsMarksFailureNoDispatch(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	_, err := store.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)

	for i := 0; i < defaultMaxAdoptions; i++ {
		backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())
		store.runAdoption(ctx)
	}
	require.Len(t, dispatcher.calls, defaultMaxAdoptions)

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, defaultMaxAdoptions, meta.AdoptionCount)
	require.Equal(t, StatusInProgress, meta.Status)

	// One more stale lease pushes AdoptionCount to the cap: no further claim
	// or dispatch, a terminal failure is persisted instead.
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())
	store.runAdoption(ctx)

	require.Len(t, dispatcher.calls, defaultMaxAdoptions, "no additional dispatch once the cap is hit")

	var final Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &final))
	require.Equal(t, StatusFailure, final.Status)
	require.Equal(t, errTooManyAdoptions.Error(), final.ErrorMessage)

	require.Equal(t, float64(defaultMaxAdoptions+1), testutil.ToFloat64(store.adoptionAttempts.WithLabelValues(tenantID)))
	require.Equal(t, float64(defaultMaxAdoptions), testutil.ToFloat64(store.adoptionSuccesses.WithLabelValues(tenantID)))
}

func TestStoreCompleteFail_SuccessAlwaysWinsOverFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("complete then fail", func(t *testing.T) {
		store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
		const tenantID = "tenant-a"
		requestID := uuid.New().String()
		lease, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
		require.NoError(t, err)

		want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")}
		require.NoError(t, store.complete(ctx, tenantID, requestID, &lease, want))
		require.NoError(t, store.fail(ctx, tenantID, requestID, &lease, errors.New("late straggler failure")))

		var meta Metadata
		require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
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
		lease, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
		require.NoError(t, err)

		require.NoError(t, store.fail(ctx, tenantID, requestID, &lease, errors.New("first failure")))

		// A real success comes from a fresh execution, not the same stale
		// cache writing twice: read the lease fail() just landed rather than
		// reuse the original.
		lateLease := readLease(t, ctx, store, tenantID, requestID)
		want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("late-success")}
		require.NoError(t, store.complete(ctx, tenantID, requestID, &lateLease, want))

		// A real success always wins over a prior failure: at least one
		// execution produced the correct answer, so it must be reported --
		// regardless of what any differently-outcome'd execution wrote first.
		var meta Metadata
		require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
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
	lease, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)

	want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")}
	require.NoError(t, store.complete(ctx, tenantID, requestID, &lease, want))

	// The original owner heartbeats after a racing execution completed the
	// record: its cached lease is stale, so this reports lost ownership.
	require.ErrorIs(t, store.heartbeat(ctx, tenantID, requestID, &lease), errLostOwnership)

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, StatusSuccess, meta.Status)

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(want, result.Response))
}

// A completion landing while heartbeat()'s write is in flight bumps the
// version first, so the heartbeat is rejected and reports lost ownership.
func TestStoreHeartbeat_RejectedByConcurrentCompletion(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	seed := NewStore(log.NewNopLogger(), inner, nil)
	lease, err := seed.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)

	target := seed.buildPath(tenantID, requestID, metadataFilename)
	gated := newGatedUploadBucket(inner, target)
	heartbeatStore := NewStore(log.NewNopLogger(), gated, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	var heartbeatErr error
	go func() {
		defer wg.Done()
		heartbeatErr = heartbeatStore.heartbeat(ctx, tenantID, requestID, &lease)
	}()
	<-gated.reached

	resp := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")}
	completeStore := NewStore(log.NewNopLogger(), inner, nil)
	require.NoError(t, completeStore.complete(ctx, tenantID, requestID, &lease, resp))

	close(gated.proceed)
	wg.Wait()

	// Version fencing can't distinguish "my own prior write already landed"
	// from "someone else's did" -- heartbeat reports lost ownership either
	// way. The record itself is unaffected: complete() already won.
	require.ErrorIs(t, heartbeatErr, errLostOwnership)

	var meta Metadata
	require.NoError(t, seed.readJSON(ctx, target, &meta))
	require.Equal(t, StatusSuccess, meta.Status)

	result, err := seed.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(resp, result.Response))
}

// A completion landing while adopt()'s claim is in flight bumps the version
// first, so the claim is rejected and nothing is dispatched.
func TestStoreAdopt_ClaimRejectedByConcurrentCompletion(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	seed := NewStore(log.NewNopLogger(), inner, nil)
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	_, err := seed.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, seed, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	target := seed.buildPath(tenantID, requestID, metadataFilename)
	gated := newGatedUploadBucket(inner, target)
	adoptStore := NewStore(log.NewNopLogger(), gated, nil)
	dispatcher := &fakeDispatcher{}
	adoptStore.SetDispatcher(dispatcher)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		adoptStore.adopt(ctx, tenantID, requestID)
	}()
	<-gated.reached

	// Read fresh rather than reuse create()'s lease: backdateHeartbeat's
	// write already moved the version on, and the completion must land on
	// the current version for the racing claim to be the one rejected.
	resp := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("already-done")}
	completeStore := NewStore(log.NewNopLogger(), inner, nil)
	lease := readLease(t, ctx, completeStore, tenantID, requestID)
	require.NoError(t, completeStore.complete(ctx, tenantID, requestID, &lease, resp))

	close(gated.proceed)
	wg.Wait()

	require.Empty(t, dispatcher.calls)

	result, err := seed.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(resp, result.Response))
}

// The headline property: two stores racing runAdoption over the same stale
// record produce exactly one Dispatch, since only one claim can match the
// version both read. A barrier forces the claims to overlap; repeating over
// several records exercises both winner orderings.
func TestStoreAdopt_ConcurrentClaimAllowsExactlyOneDispatch(t *testing.T) {
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		inner := objstore.NewInMemBucket()

		const tenantID = "tenant-a"
		requestID := uuid.New().String()
		seed := NewStore(log.NewNopLogger(), inner, nil)
		spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
		_, err := seed.create(ctx, tenantID, requestID, spec)
		require.NoError(t, err)
		backdateHeartbeat(t, ctx, seed, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

		target := seed.buildPath(tenantID, requestID, metadataFilename)
		barrier := newBarrierUploadBucket(inner, target, 2)
		storeA := NewStore(log.NewNopLogger(), barrier, nil)
		storeB := NewStore(log.NewNopLogger(), barrier, nil)
		storeA.ownerID = ownerA
		storeB.ownerID = ownerB

		var dispatchCount atomic.Int32
		dispatcher := &fakeDispatcher{
			onDispatch: func(_, _ string, _ *querierv1.SelectMergeStacktracesRequest) {
				dispatchCount.Inc()
			},
		}
		storeA.SetDispatcher(dispatcher)
		storeB.SetDispatcher(dispatcher)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); storeA.runAdoption(ctx) }()
		go func() { defer wg.Done(); storeB.runAdoption(ctx) }()
		wg.Wait()

		require.Equal(t, int32(1), dispatchCount.Load())

		var meta Metadata
		require.NoError(t, seed.readJSON(ctx, target, &meta))
		require.True(t,
			strings.HasPrefix(meta.Owner, storeA.ownerID+"/") || strings.HasPrefix(meta.Owner, storeB.ownerID+"/"),
			"meta.Owner = %q, want a token minted by storeA or storeB", meta.Owner)
	}
}

// The record's own owner renews its lease between adopt()'s read and its
// conditional Upload: the claim's If-Match fails, nothing is dispatched, and
// the renewed lease is left untouched.
func TestStoreAdopt_ClaimLosesRaceToConcurrentWriteConditionNotMet(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	seed := NewStore(log.NewNopLogger(), inner, nil)
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	_, err := seed.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, seed, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	target := seed.buildPath(tenantID, requestID, metadataFilename)
	gated := newGatedUploadBucket(inner, target)
	adoptStore := NewStore(log.NewNopLogger(), gated, nil)
	adoptStore.ownerID = "owner-adopter"
	dispatcher := &fakeDispatcher{}
	adoptStore.SetDispatcher(dispatcher)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		adoptStore.adopt(ctx, tenantID, requestID)
	}()
	<-gated.reached

	// The true owner renews its lease while the claim is in flight. Read
	// fresh: backdateHeartbeat's write already moved the version on.
	trueOwner := NewStore(log.NewNopLogger(), inner, nil)
	lease := readLease(t, ctx, trueOwner, tenantID, requestID)
	require.NoError(t, trueOwner.heartbeat(ctx, tenantID, requestID, &lease))

	close(gated.proceed)
	wg.Wait()

	require.Empty(t, dispatcher.calls)
	require.Equal(t, float64(0), testutil.ToFloat64(adoptStore.adoptionSuccesses.WithLabelValues(tenantID)))

	var got Metadata
	require.NoError(t, seed.readJSON(ctx, target, &got))
	require.Equal(t, lease.meta.Owner, got.Owner)
	require.WithinDuration(t, time.Now(), got.LastHeartbeat, 5*time.Second)
}

// TestStoreHeartbeat_TransientErrorIsNotLostOwnership proves a plain upload
// failure is left for the next tick to retry rather than misclassified as
// lost ownership: only a real version conflict earns that sentinel.
func TestStoreHeartbeat_TransientErrorIsNotLostOwnership(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	seed := NewStore(log.NewNopLogger(), inner, nil)
	lease, err := seed.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)

	flaky := &transientErrorOnceBucket{Bucket: inner, failNext: true, err: errors.New("simulated transient upload error")}
	store := NewStore(log.NewNopLogger(), flaky, nil)

	require.NoError(t, store.heartbeat(ctx, tenantID, requestID, &lease))

	// The cached lease is left usable: a second heartbeat against the same
	// version succeeds cleanly, proving the first, transient failure didn't
	// corrupt or discard it.
	require.NoError(t, store.heartbeat(ctx, tenantID, requestID, &lease))

	var meta Metadata
	require.NoError(t, seed.readJSON(ctx, seed.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, StatusInProgress, meta.Status)
}

// After another store adopts the record, the original owner's heartbeat
// reports errLostOwnership.
func TestStoreHeartbeat_ReturnsLostOwnershipAfterAdoption(t *testing.T) {
	ctx := context.Background()
	bucket := objstore.NewInMemBucket()
	storeA := NewStore(log.NewNopLogger(), bucket, nil)
	storeB := NewStore(log.NewNopLogger(), bucket, nil)
	storeA.ownerID = ownerA
	storeB.ownerID = ownerB
	storeB.SetDispatcher(&fakeDispatcher{})

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	leaseA, err := storeA.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, storeA, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	storeB.runAdoption(ctx)

	var claimed Metadata
	require.NoError(t, storeA.readJSON(ctx, storeA.buildPath(tenantID, requestID, metadataFilename), &claimed))
	require.True(t, strings.HasPrefix(claimed.Owner, storeB.ownerID+"/"))

	err = storeA.heartbeat(ctx, tenantID, requestID, &leaseA)
	require.ErrorIs(t, err, errLostOwnership)

	// The blind write, on rejection, must not have written anything at all:
	// there is no defensive fresh-read to fall back to anymore.
	var afterRejectedWrite Metadata
	require.NoError(t, storeA.readJSON(ctx, storeA.buildPath(tenantID, requestID, metadataFilename), &afterRejectedWrite))
	require.Equal(t, claimed, afterRejectedWrite)
}

// TestStoreFail_DropsWhenOwnershipMoved proves fail()'s ownership gate: once
// another store has adopted a record, the original owner's (stale) failure
// is dropped rather than overwriting the new owner's in-progress claim.
func TestStoreFail_DropsWhenOwnershipMoved(t *testing.T) {
	ctx := context.Background()
	bucket := objstore.NewInMemBucket()
	storeA := NewStore(log.NewNopLogger(), bucket, nil)
	storeB := NewStore(log.NewNopLogger(), bucket, nil)
	storeA.ownerID = ownerA
	storeB.ownerID = ownerB
	storeB.SetDispatcher(&fakeDispatcher{})

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	_, err := storeA.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, storeA, tenantID, requestID, time.Now().Add(-time.Hour).UTC())
	leaseA := readLease(t, ctx, storeA, tenantID, requestID) // current as of just before adoption

	storeB.runAdoption(ctx)

	require.NoError(t, storeA.fail(ctx, tenantID, requestID, &leaseA, errors.New("stale execution failure")))

	var meta Metadata
	require.NoError(t, storeA.readJSON(ctx, storeA.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, StatusInProgress, meta.Status)
	require.True(t, strings.HasPrefix(meta.Owner, storeB.ownerID+"/"))
}

// complete() is the one intentionally un-gated write: a deposed owner's late
// success must still be reported, whatever the new owner has written since.
func TestStoreComplete_SucceedsAfterOwnershipMoved(t *testing.T) {
	ctx := context.Background()

	t.Run("original owner completes before new owner writes anything", func(t *testing.T) {
		bucket := objstore.NewInMemBucket()
		storeA := NewStore(log.NewNopLogger(), bucket, nil)
		storeB := NewStore(log.NewNopLogger(), bucket, nil)
		storeA.ownerID = ownerA
		storeB.ownerID = ownerB
		storeB.SetDispatcher(&fakeDispatcher{})

		const tenantID = "tenant-a"
		requestID := uuid.New().String()
		spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
		_, err := storeA.create(ctx, tenantID, requestID, spec)
		require.NoError(t, err)
		backdateHeartbeat(t, ctx, storeA, tenantID, requestID, time.Now().Add(-time.Hour).UTC())
		leaseA := readLease(t, ctx, storeA, tenantID, requestID) // current as of just before adoption

		storeB.runAdoption(ctx)

		// leaseA's cached version is now stale (storeB's claim moved it), so
		// complete()'s own metadata mark is dropped; get() below reports
		// Success via the result.pb anchor instead of a landed write.
		want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("original-owner-result")}
		require.NoError(t, storeA.complete(ctx, tenantID, requestID, &leaseA, want))

		result, err := storeA.get(ctx, tenantID, requestID)
		require.NoError(t, err)
		require.Equal(t, StatusSuccess, result.Metadata.Status)
		require.True(t, proto.Equal(want, result.Response))
	})

	t.Run("original owner completes after new owner's own terminal write", func(t *testing.T) {
		bucket := objstore.NewInMemBucket()
		storeA := NewStore(log.NewNopLogger(), bucket, nil)
		storeB := NewStore(log.NewNopLogger(), bucket, nil)
		storeA.ownerID = ownerA
		storeB.ownerID = ownerB
		storeB.SetDispatcher(&fakeDispatcher{})

		const tenantID = "tenant-a"
		requestID := uuid.New().String()
		spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
		leaseA, err := storeA.create(ctx, tenantID, requestID, spec)
		require.NoError(t, err)
		backdateHeartbeat(t, ctx, storeA, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

		storeB.runAdoption(ctx)

		// storeB's own claimed lease isn't returned by runAdoption; read it
		// back fresh so its fail() call writes conditioned on what it
		// actually claimed with, rather than on some other owner's cache.
		leaseB := readLease(t, ctx, storeB, tenantID, requestID)
		require.NoError(t, storeB.fail(ctx, tenantID, requestID, &leaseB, errors.New("new owner's re-execution failed")))

		// leaseA is stale (the claim and the failure write both moved the
		// version), so this mark is dropped: get() reports Success via the
		// anchor while metadata.json on disk still reads Failure.
		want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("original-owner-result")}
		require.NoError(t, storeA.complete(ctx, tenantID, requestID, &leaseA, want))

		result, err := storeA.get(ctx, tenantID, requestID)
		require.NoError(t, err)
		require.Equal(t, StatusSuccess, result.Metadata.Status)
		require.True(t, proto.Equal(want, result.Response))

		var onDisk Metadata
		require.NoError(t, storeA.readJSON(ctx, storeA.buildPath(tenantID, requestID, metadataFilename), &onDisk))
		require.Equal(t, StatusFailure, onDisk.Status)
	})
}

// A durable result.pb reports success even when complete()'s mark never
// landed, and get() writes nothing back.
func TestStoreGet_InProgressWithExistingResultReportsSuccessWithoutHealing(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	_, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)

	// Simulate complete()'s lost mark directly: result.pb lands, but
	// metadata.json is never touched.
	resp := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("durable-result")}
	data, err := proto.Marshal(resp)
	require.NoError(t, err)
	require.NoError(t, store.bucket.Upload(ctx, store.buildPath(tenantID, requestID, resultFilename), bytes.NewReader(data)))

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(resp, result.Response))

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, StatusInProgress, meta.Status, "the anchor check must not write back to metadata.json")
}

// The anchor also outranks a stale Failure record: a deposed owner's real
// success must not be shadowed by the new owner's terminal write.
func TestStoreGet_FailureWithExistingResultReportsSuccessWithoutHealing(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	lease, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)
	require.NoError(t, store.fail(ctx, tenantID, requestID, &lease, errors.New("stale execution failure")))

	// A deposed owner's late success lands directly, as complete() would
	// upload it: result.pb durable, metadata.json untouched (still Failure).
	resp := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("late-success")}
	data, err := proto.Marshal(resp)
	require.NoError(t, err)
	require.NoError(t, store.bucket.Upload(ctx, store.buildPath(tenantID, requestID, resultFilename), bytes.NewReader(data)))

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(resp, result.Response))

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, StatusFailure, meta.Status, "the anchor check must not write back to metadata.json")
}

// A record whose Attributes never carry a version still gets its durable
// result reported instead of complete() erroring out.
func TestStoreComplete_LandsWhenMetadataReportsNoVersion(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), &noVersionAttributesBucket{objstore.NewInMemBucket()}, nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	lease, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)

	resp := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")}
	require.NoError(t, store.complete(ctx, tenantID, requestID, &lease, resp))

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(resp, result.Response))
}

// TestStoreAdopt_ClaimsUnconditionallyWhenMetadataReportsNoVersion proves
// the same valve in adopt(): a stale record with no reported version is
// still claimed and dispatched instead of being permanently unadoptable.
func TestStoreAdopt_ClaimsUnconditionallyWhenMetadataReportsNoVersion(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), &noVersionAttributesBucket{objstore.NewInMemBucket()}, nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	_, err := store.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	store.runAdoption(ctx)

	require.Len(t, dispatcher.calls, 1)
	require.Equal(t, float64(1), testutil.ToFloat64(store.adoptionSuccesses.WithLabelValues(tenantID)))

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, dispatcher.calls[0].lease.meta.Owner, meta.Owner)
}

// A claim that lands but whose version seed fails stands un-dispatched --
// exactly as if Dispatch had declined -- so a later scan retries it.
func TestStoreAdopt_TransientVersionReadFailureSkipsDispatchButKeepsClaim(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()
	seed := NewStore(log.NewNopLogger(), inner, nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	_, err := seed.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, seed, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	metaPath := seed.buildPath(tenantID, requestID, metadataFilename)
	wrapped := &attributesFailAfterFirstBucket{
		Bucket:   inner,
		failName: metaPath,
		err:      errors.New("simulated transient storage error"),
	}
	store := NewStore(log.NewNopLogger(), wrapped, nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	store.runAdoption(ctx)

	require.Empty(t, dispatcher.calls, "a version-seed failure must not hand the dispatcher an unfenced lease")

	var meta Metadata
	require.NoError(t, seed.readJSON(ctx, metaPath, &meta))
	require.Equal(t, 1, meta.AdoptionCount, "the claim write itself still landed")
	require.NotEmpty(t, meta.Owner)
}

func TestSetDispatcher(t *testing.T) {
	t.Run("succeeds before the service starts", func(t *testing.T) {
		store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
		dispatcher := &fakeDispatcher{}

		require.NotPanics(t, func() { store.SetDispatcher(dispatcher) })
		require.Same(t, Dispatcher(dispatcher), store.dispatcher)
	})

	t.Run("panics once the service has started", func(t *testing.T) {
		ctx := context.Background()
		store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
		require.NoError(t, services.StartAndAwaitRunning(ctx, store))
		defer func() { require.NoError(t, services.StopAndAwaitTerminated(ctx, store)) }()

		require.Panics(t, func() { store.SetDispatcher(&fakeDispatcher{}) })
	})
}

func TestStoreDisableConditionalWrites(t *testing.T) {
	t.Run("succeeds before the service starts", func(t *testing.T) {
		store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)

		require.NotPanics(t, func() { store.DisableConditionalWrites() })
		require.False(t, store.conditionalWritesSupported)
	})

	t.Run("panics once the service has started", func(t *testing.T) {
		ctx := context.Background()
		store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
		require.NoError(t, services.StartAndAwaitRunning(ctx, store))
		defer func() { require.NoError(t, services.StopAndAwaitTerminated(ctx, store)) }()

		require.Panics(t, func() { store.DisableConditionalWrites() })
	})
}

// failIterBucket wraps a Bucket and fails the test if Iter is ever called,
// proving a caller doesn't even list the bucket -- not just that it
// declines to act on what it finds.
type failIterBucket struct {
	objstore.Bucket
	t *testing.T
}

func (b *failIterBucket) Iter(ctx context.Context, dir string, f func(string) error, options ...objstore.IterOption) error {
	b.t.Fatal("Iter must not be called when conditional writes are disabled")
	return nil
}

// TestStoreRunAdoption_NoopWhenConditionalWritesDisabled proves the
// adoption scan never runs in degrade mode: a claim without fencing would
// overwrite a live owner's lease outright.
func TestStoreRunAdoption_NoopWhenConditionalWritesDisabled(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), &failIterBucket{Bucket: objstore.NewInMemBucket(), t: t}, nil)
	store.DisableConditionalWrites()
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	_, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	store.runAdoption(ctx)

	require.Empty(t, dispatcher.calls)
}

// failConditionalUploadBucket wraps a Bucket and fails the test if Upload
// is ever called conditionally, proving degrade mode's writes are
// genuinely unconditional rather than merely tolerating rejection.
type failConditionalUploadBucket struct {
	objstore.Bucket
	t *testing.T
}

func (b *failConditionalUploadBucket) Upload(ctx context.Context, name string, r io.Reader, opts ...objstore.ObjectUploadOption) error {
	if len(opts) > 0 {
		b.t.Fatal("Upload must not be called conditionally when conditional writes are disabled")
	}
	return b.Bucket.Upload(ctx, name, r, opts...)
}

// TestStoreHeartbeatCompleteFail_UnconditionalWhenConditionalWritesDisabled
// proves create()/heartbeat()/complete()/fail() all take the plain,
// unconditional write path in degrade mode.
func TestStoreHeartbeatCompleteFail_UnconditionalWhenConditionalWritesDisabled(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), &failConditionalUploadBucket{Bucket: objstore.NewInMemBucket(), t: t}, nil)
	store.DisableConditionalWrites()

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	lease, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)

	require.NoError(t, store.heartbeat(ctx, tenantID, requestID, &lease))

	want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")}
	require.NoError(t, store.complete(ctx, tenantID, requestID, &lease, want))

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(want, result.Response))

	// fail() shares complete()'s degrade-mode fallback but has no test of its
	// own; exercise it here on a second, fresh record (complete() above
	// already terminated the first).
	requestID2 := uuid.New().String()
	lease2, err := store.create(ctx, tenantID, requestID2, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)
	require.NoError(t, store.fail(ctx, tenantID, requestID2, &lease2, errors.New("simulated failure")))

	result2, err := store.get(ctx, tenantID, requestID2)
	require.NoError(t, err)
	require.Equal(t, StatusFailure, result2.Metadata.Status)
}

// An owner that died between uploading result.pb and marking Success gets
// healed instead of re-dispatched.
func TestStoreAdopt_HealsToSuccessWhenResultAlreadyExists(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	spec := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds"}
	_, err := store.create(ctx, tenantID, requestID, spec)
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	// Simulate the crash window: result.pb is durably written, but the
	// metadata CAS to StatusSuccess never landed.
	resp := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("durable-result")}
	data, err := proto.Marshal(resp)
	require.NoError(t, err)
	require.NoError(t, store.bucket.Upload(ctx, store.buildPath(tenantID, requestID, resultFilename), bytes.NewReader(data)))

	store.runAdoption(ctx)

	require.Empty(t, dispatcher.calls, "a durable result must be healed, never re-dispatched")

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, StatusSuccess, meta.Status)
	require.Empty(t, meta.ErrorMessage)

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(resp, result.Response))
}

// markUnadoptable re-checks leaseExpired on its fresh read: an owner that
// heartbeated since the caller's judgment must not be marked failed.
func TestStoreMarkUnadoptable_AbortsIfOwnerHeartbeatedSinceJudgment(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	_, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	// The owner renews between the staleness judgment and markUnadoptable's
	// CAS. Read fresh: backdateHeartbeat already moved the version on.
	lease := readLease(t, ctx, store, tenantID, requestID)
	require.NoError(t, store.heartbeat(ctx, tenantID, requestID, &lease))

	require.NoError(t, store.markUnadoptable(ctx, tenantID, requestID))

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, StatusInProgress, meta.Status)
	require.Empty(t, meta.ErrorMessage)
}

// TestStoreMarkTooManyAdoptions_AbortsIfOwnerHeartbeatedSinceJudgment mirrors
// the markUnadoptable case above for markTooManyAdoptions.
func TestStoreMarkTooManyAdoptions_AbortsIfOwnerHeartbeatedSinceJudgment(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	_, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	// Read fresh rather than reuse create()'s lease, since backdateHeartbeat's
	// write has already moved the version on.
	lease := readLease(t, ctx, store, tenantID, requestID)
	require.NoError(t, store.heartbeat(ctx, tenantID, requestID, &lease))

	require.NoError(t, store.markTooManyAdoptions(ctx, tenantID, requestID, defaultMaxAdoptions))

	var meta Metadata
	require.NoError(t, store.readJSON(ctx, store.buildPath(tenantID, requestID, metadataFilename), &meta))
	require.Equal(t, StatusInProgress, meta.Status)
	require.Empty(t, meta.ErrorMessage)
}

// Version fencing alone catches the self-adoption blind spot: even when the
// same store adopts its own stale record, the original execution's heartbeat
// sees a changed version -- no comparison against Owner needed.
func TestStoreHeartbeat_ReturnsLostOwnershipAfterSelfAdoption(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	originalToken, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	store.runAdoption(ctx)

	require.Len(t, dispatcher.calls, 1)
	require.NotEqual(t, originalToken.meta.Owner, dispatcher.calls[0].lease.meta.Owner, "self-adoption must still mint a fresh token")

	err = store.heartbeat(ctx, tenantID, requestID, &originalToken)
	require.ErrorIs(t, err, errLostOwnership)
}

// getErrAfterNBucket wraps a Bucket, forcing Get to fail starting on the
// (afterN+1)th call for one specific object name; every earlier call and
// every other name is delegated unchanged.
type getErrAfterNBucket struct {
	objstore.Bucket
	failName string
	afterN   int
	calls    int
	err      error
}

func (b *getErrAfterNBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	if name == b.failName {
		b.calls++
		if b.calls > b.afterN {
			return nil, b.err
		}
	}
	return b.Bucket.Get(ctx, name)
}

// Once markUnadoptable's write has landed, a failed best-effort re-read must
// not leave get() reporting the stale in-progress snapshot.
func TestStoreGet_HealedStateAppliedLocallyWhenReReadFailsAfterMarkUnadoptable(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	seed := NewStore(log.NewNopLogger(), inner, nil)
	past := time.Now().Add(-time.Hour).UTC()
	meta := &Metadata{
		RequestID:     requestID,
		TenantID:      tenantID,
		Status:        StatusInProgress,
		CreatedAt:     past,
		LastHeartbeat: past,
	}
	metaPath := seed.buildPath(tenantID, requestID, metadataFilename)
	require.NoError(t, seed.saveJSON(ctx, metaPath, meta))

	// Get(metaPath) runs twice before the targeted re-read (get()'s initial
	// read, then updateMetadata's); only the third call should fail.
	wrapped := &getErrAfterNBucket{
		Bucket:   inner,
		failName: metaPath,
		afterN:   2,
		err:      errors.New("simulated transient re-read error"),
	}
	store := NewStore(log.NewNopLogger(), wrapped, nil)

	result, err := store.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusFailure, result.Metadata.Status)
	require.Equal(t, errOrphanedNoSpec.Error(), result.Metadata.ErrorMessage)
}

// running() must not scan immediately at startup, so replicas that redeploy
// together don't scan in lockstep.
func TestStoreRunning_StaggersFirstAdoptionScan(t *testing.T) {
	ctx := context.Background()
	store := NewStore(log.NewNopLogger(), objstore.NewInMemBucket(), nil)
	store.scanInterval = time.Hour // long enough that the test window never sees it fire
	dispatcher := &fakeDispatcher{}
	store.SetDispatcher(dispatcher)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	_, err := store.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)
	backdateHeartbeat(t, ctx, store, tenantID, requestID, time.Now().Add(-time.Hour).UTC())

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		_ = store.running(runCtx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	require.Zero(t, dispatcher.hasCapacityCalls, "adoption scan must not run immediately at startup")
}

// A lost CAS race is routine contention and must not inflate
// objstore_bucket_operation_failures_total.
func TestNewStore_LostCASRaceNotCountedAsBucketFailure(t *testing.T) {
	ctx := context.Background()
	reg := prometheus.NewPedanticRegistry()
	inner := objstore.NewInMemBucket()

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	seed := NewStore(log.NewNopLogger(), inner, nil)
	lease, err := seed.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)

	target := seed.buildPath(tenantID, requestID, metadataFilename)
	gated := newGatedUploadBucket(inner, target)
	metered := objstore.WrapWithMetrics(gated, reg, "test")
	heartbeatStore := NewStore(log.NewNopLogger(), metered, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = heartbeatStore.heartbeat(ctx, tenantID, requestID, &lease)
	}()
	<-gated.reached

	require.NoError(t, seed.complete(ctx, tenantID, requestID, &lease, &querierv1.SelectMergeStacktracesResponse{}))
	close(gated.proceed)
	wg.Wait()

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP objstore_bucket_operation_failures_total Total number of operations against a bucket that failed, but were not expected to fail in certain way from caller perspective. Those errors have to be investigated.
		# TYPE objstore_bucket_operation_failures_total counter
		objstore_bucket_operation_failures_total{bucket="test",operation="attributes"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="delete"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="exists"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="get"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="get_range"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="iter"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="upload"} 0
	`), "objstore_bucket_operation_failures_total"))
}

// objectExists/specAbsent run on every poll and adoption attempt, so a
// not-found there is their normal outcome, not a bucket failure.
func TestNewStore_NotFoundNotCountedAsBucketFailure(t *testing.T) {
	ctx := context.Background()
	reg := prometheus.NewPedanticRegistry()
	metered := objstore.WrapWithMetrics(objstore.NewInMemBucket(), reg, "test")
	store := NewStore(log.NewNopLogger(), metered, nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	exists, err := store.objectExists(ctx, store.buildPath(tenantID, requestID, specFilename))
	require.NoError(t, err)
	require.False(t, exists)

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP objstore_bucket_operation_failures_total Total number of operations against a bucket that failed, but were not expected to fail in certain way from caller perspective. Those errors have to be investigated.
		# TYPE objstore_bucket_operation_failures_total counter
		objstore_bucket_operation_failures_total{bucket="test",operation="attributes"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="delete"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="exists"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="get"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="get_range"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="iter"} 0
		objstore_bucket_operation_failures_total{bucket="test",operation="upload"} 0
	`), "objstore_bucket_operation_failures_total"))
}

// alwaysConditionNotMetBucket wraps a Bucket whose conditional (If-Match)
// uploads always fail with a condition-not-met error, for testing a single
// rejected conditional write in isolation.
type alwaysConditionNotMetBucket struct {
	objstore.Bucket
	uploadCalls int
}

var errAlwaysConditionNotMet = errors.New("stub: condition never met")

func (b *alwaysConditionNotMetBucket) Upload(ctx context.Context, name string, r io.Reader, opts ...objstore.ObjectUploadOption) error {
	if len(opts) > 0 {
		b.uploadCalls++
		return errAlwaysConditionNotMet
	}
	return b.Bucket.Upload(ctx, name, r, opts...)
}

func (b *alwaysConditionNotMetBucket) IsConditionNotMetErr(err error) bool {
	return errors.Is(err, errAlwaysConditionNotMet) || b.Bucket.IsConditionNotMetErr(err)
}

// gatedConditionalUploadBucket pauses the first If-Match Upload (recognized
// by len(opts) > 0) while unconditional Uploads pass through; reached closes
// at the gate, the call continues once proceed closes.
type gatedConditionalUploadBucket struct {
	objstore.Bucket
	once    sync.Once
	reached chan struct{}
	proceed chan struct{}
}

func newGatedConditionalUploadBucket(inner objstore.Bucket) *gatedConditionalUploadBucket {
	return &gatedConditionalUploadBucket{
		Bucket:  inner,
		reached: make(chan struct{}),
		proceed: make(chan struct{}),
	}
}

func (b *gatedConditionalUploadBucket) Upload(ctx context.Context, name string, r io.Reader, opts ...objstore.ObjectUploadOption) error {
	if len(opts) > 0 {
		b.once.Do(func() { close(b.reached) })
		<-b.proceed
	}
	return b.Bucket.Upload(ctx, name, r, opts...)
}

// A backend rejecting every conditional write looks identical to a real
// adopter's claim, so heartbeat reports lost ownership on the first attempt.
func TestStoreHeartbeat_RejectedConditionalWriteReturnsLostOwnershipOnFirstAttempt(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()
	seed := NewStore(log.NewNopLogger(), inner, nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	lease, err := seed.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)

	stuck := &alwaysConditionNotMetBucket{Bucket: inner}
	store := NewStore(log.NewNopLogger(), stuck, nil)

	require.ErrorIs(t, store.heartbeat(ctx, tenantID, requestID, &lease), errLostOwnership)
	require.Equal(t, 1, stuck.uploadCalls, "no retry loop: exactly one conditional write is attempted")
}

// complete()'s mark is one best-effort attempt: a rejected write is logged,
// not surfaced, and the anchor still reports success.
func TestStoreComplete_LostMarkIsNotSurfacedAsErrorAndAnchorStillReportsSuccess(t *testing.T) {
	ctx := context.Background()
	inner := objstore.NewInMemBucket()
	seed := NewStore(log.NewNopLogger(), inner, nil)

	const tenantID = "tenant-a"
	requestID := uuid.New().String()
	lease, err := seed.create(ctx, tenantID, requestID, &querierv1.SelectMergeStacktracesRequest{})
	require.NoError(t, err)

	stuck := &alwaysConditionNotMetBucket{Bucket: inner}
	store := NewStore(log.NewNopLogger(), stuck, nil)

	want := &querierv1.SelectMergeStacktracesResponse{Tree: []byte("done")}
	require.NoError(t, store.complete(ctx, tenantID, requestID, &lease, want))
	require.Equal(t, 1, stuck.uploadCalls, "no retry loop: exactly one conditional write is attempted")

	result, err := seed.get(ctx, tenantID, requestID)
	require.NoError(t, err)
	require.Equal(t, StatusSuccess, result.Metadata.Status)
	require.True(t, proto.Equal(want, result.Response))
}
