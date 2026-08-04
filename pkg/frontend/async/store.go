package async

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

const (
	storagePrefix               = "async-queries/"
	metadataFilename            = "metadata.json"
	specFilename                = "spec.pb"
	resultFilename              = "result.pb"
	defaultTTL                  = 30 * time.Minute
	defaultHeartbeatInterval    = 15 * time.Second
	defaultLeaseTimeout         = 45 * time.Second
	defaultAdoptionScanInterval = 30 * time.Second
	defaultMaxAdoptions         = 3
	cleanupInterval             = 5 * time.Minute
)

var (
	// errSpecCorrupt marks a spec that exists but does not decode: unlike a
	// transient read error, retrying can never fix it.
	errSpecCorrupt = errors.New("spec is present but could not be decoded")

	// errOrphanedNoSpec: a stale in-progress record whose spec is gone can
	// never be resumed.
	errOrphanedNoSpec = errors.New("query orphaned: owner heartbeat expired and no request spec is available for adoption")

	errTooManyAdoptions = errors.New("query exceeded the maximum number of adoption attempts")

	// errLostOwnership: another execution has claimed the record; the caller
	// must stop writing and cancel its work.
	errLostOwnership = errors.New("async query ownership has moved to another owner")
)

// Status represents the lifecycle state of an async query.
type Status string

const (
	// StatusInProgress indicates the query is still executing.
	StatusInProgress Status = "in_progress"
	// StatusSuccess indicates the query completed and a result is available.
	StatusSuccess Status = "success"
	// StatusFailure indicates the query failed; ErrorMessage holds the reason.
	StatusFailure Status = "failure"
)

// terminal reports whether s is final.
func (s Status) terminal() bool {
	return s != StatusInProgress
}

// Metadata describes the state of a single async query. It is persisted as
// metadata.json alongside the query's result in object storage.
type Metadata struct {
	RequestID     string    `json:"request_id"`
	TenantID      string    `json:"tenant_id"`
	Status        Status    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	LastHeartbeat time.Time `json:"last_heartbeat,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	// Owner is a per-execution token (see newOwnerToken), kept for logs only;
	// fencing relies on the object version alone.
	Owner         string `json:"owner,omitempty"`
	AdoptionCount int    `json:"adoption_count,omitempty"`
}

// Result bundles a query's Metadata with its decoded Response. Response is only
// populated when Metadata.Status is StatusSuccess.
type Result struct {
	Metadata Metadata
	Response *querierv1.SelectMergeStacktracesResponse
}

// Dispatcher re-runs a query that Store has adopted after its owner's lease
// expired. Implemented by Coordinator; wired into Store via SetDispatcher.
type Dispatcher interface {
	// HasCapacity is a read-only peek, not a reservation: Dispatch has its
	// own gate and may still decline.
	HasCapacity(tenantID string) bool
	// Dispatch runs spec under lease, so the execution's own writes are
	// fenced by the version the adoption claim produced.
	Dispatch(tenantID, requestID string, lease leaseHandle, spec *querierv1.SelectMergeStacktracesRequest)
}

// Store persists async query state and results in object storage. It also runs
// as a dskit service that periodically removes expired entries and adopts
// queries whose owner's lease has expired.
type Store struct {
	services.Service
	logger            log.Logger
	bucket            objstore.Bucket
	ttl               time.Duration
	heartbeatInterval time.Duration
	leaseTimeout      time.Duration
	scanInterval      time.Duration
	ownerID           string
	dispatcher        Dispatcher

	conditionalWritesSupported bool

	adoptionAttempts  *prometheus.CounterVec
	adoptionSuccesses *prometheus.CounterVec
}

// NewStore returns a Store backed by bucket. Metadata writes are fenced with
// If-Match conditions on the object version; call DisableConditionalWrites
// when the backend does not enforce them. The returned Store is a dskit
// service that periodically deletes expired entries and adopts orphaned
// queries.
func NewStore(logger log.Logger, bucket objstore.Bucket, reg prometheus.Registerer) *Store {
	hostname, _ := os.Hostname()
	s := &Store{
		logger:                     logger,
		bucket:                     wrapExpectedErrs(bucket),
		ttl:                        defaultTTL,
		heartbeatInterval:          defaultHeartbeatInterval,
		leaseTimeout:               defaultLeaseTimeout,
		scanInterval:               defaultAdoptionScanInterval,
		ownerID:                    hostname,
		conditionalWritesSupported: true,
		adoptionAttempts: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_async_queries_adoption_attempts_total",
			Help: "Total number of attempts to adopt an async query whose owner appears to have died.",
		}, []string{"tenant"}),
		adoptionSuccesses: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Name: "pyroscope_async_queries_adoption_successes_total",
			Help: "Total number of async queries successfully claimed by a new owner after adoption.",
		}, []string{"tenant"}),
	}
	s.Service = services.NewBasicService(s.starting, s.running, s.stopping)
	return s
}

// SetDispatcher must be called before the service starts: dispatcher is read
// by running() without synchronization, so a late call panics. nil disables
// adoption.
func (s *Store) SetDispatcher(d Dispatcher) {
	if state := s.State(); state != services.New {
		panic(fmt.Sprintf("SetDispatcher must be called before the store service starts; service state is %s", state))
	}
	s.dispatcher = d
}

// DisableConditionalWrites puts the Store in degraded mode: metadata writes
// are unconditional and the adoption scan is disabled, since an unfenced
// claim would overwrite a live owner's lease. Must be called before the
// service starts.
func (s *Store) DisableConditionalWrites() {
	if state := s.State(); state != services.New {
		panic(fmt.Sprintf("DisableConditionalWrites must be called before the store service starts; service state is %s", state))
	}
	s.conditionalWritesSupported = false
}

func (s *Store) buildPath(tenantID, requestID, filename string) string {
	return path.Join(storagePrefix, tenantID, requestID, filename)
}

// newOwnerToken mints a per-execution identity: a bare hostname would read
// back unchanged when a store adopts its own record.
func (s *Store) newOwnerToken() string {
	return s.ownerID + "/" + uuid.New().String()[:8]
}

// create persists spec and an initial in-progress Metadata, returning a
// leaseHandle for the caller to dispatch the execution under.
func (s *Store) create(ctx context.Context, tenantID, requestID string, spec *querierv1.SelectMergeStacktracesRequest) (leaseHandle, error) {
	data, err := proto.Marshal(spec)
	if err != nil {
		return leaseHandle{}, fmt.Errorf("failed to marshal spec: %w", err)
	}
	// spec.pb lands first: metadata.json is what makes the record visible, so
	// a crash between the two uploads cannot leave a spec-less record.
	if err := s.bucket.Upload(ctx, s.buildPath(tenantID, requestID, specFilename), bytes.NewReader(data)); err != nil {
		return leaseHandle{}, fmt.Errorf("failed to upload spec: %w", err)
	}

	now := time.Now().UTC()
	meta := &Metadata{
		RequestID:     requestID,
		TenantID:      tenantID,
		Status:        StatusInProgress,
		CreatedAt:     now,
		LastHeartbeat: now,
		Owner:         s.newOwnerToken(),
	}
	metaPath := s.buildPath(tenantID, requestID, metadataFilename)
	if err := s.saveJSON(ctx, metaPath, meta); err != nil {
		return leaseHandle{}, err
	}
	version, err := s.currentVersionOrNil(ctx, metaPath)
	if err != nil {
		// The write landed but its version is unconfirmed: don't seed an
		// unfenced lease. The record ages into the normal adoption path.
		return leaseHandle{}, fmt.Errorf("failed to read metadata version after create: %w", err)
	}
	return leaseHandle{meta: *meta, version: version}, nil
}

func (s *Store) readSpec(ctx context.Context, tenantID, requestID string) (*querierv1.SelectMergeStacktracesRequest, error) {
	data, err := s.readRaw(ctx, s.buildPath(tenantID, requestID, specFilename))
	if err != nil {
		return nil, err
	}
	var spec querierv1.SelectMergeStacktracesRequest
	if err := proto.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("%w: %v", errSpecCorrupt, err)
	}
	return &spec, nil
}

// objectExists reports confirmed presence; any error other than not-found is
// inconclusive and must not be read as either answer.
func (s *Store) objectExists(ctx context.Context, path string) (bool, error) {
	_, err := s.bucket.Attributes(ctx, path)
	if err == nil {
		return true, nil
	}
	if s.bucket.IsObjNotFoundErr(err) {
		return false, nil
	}
	return false, err
}

// specAbsent reports whether spec.pb is confirmed gone.
func (s *Store) specAbsent(ctx context.Context, tenantID, requestID string) (bool, error) {
	exists, err := s.objectExists(ctx, s.buildPath(tenantID, requestID, specFilename))
	if err != nil {
		return false, err
	}
	return !exists, nil
}

// heartbeat renews the lease with a blind write against the cached version:
// any competing write bumps the version, so the condition alone detects lost
// ownership.
func (s *Store) heartbeat(ctx context.Context, tenantID, requestID string, lease *leaseHandle) error {
	meta := lease.meta
	meta.LastHeartbeat = time.Now().UTC()
	metaPath := s.buildPath(tenantID, requestID, metadataFilename)

	if err := s.saveJSONConditional(ctx, metaPath, &meta, lease.version); err != nil {
		if s.bucket.IsConditionNotMetErr(err) {
			// Either a real claim landed, or this owner's own earlier write
			// did and its ack was lost -- the bucket can't tell them apart.
			// Both are safe to treat as deposed: at worst a still-good
			// execution self-orphans and the record ages into the normal
			// adoption path, like a crashed owner.
			level.Debug(s.logger).Log("msg", "heartbeat lost race to a new owner", "tenant", tenantID, "request_id", requestID)
			return errLostOwnership
		}
		level.Warn(s.logger).Log("msg", "failed to write heartbeat; next tick retries with the same cached version", "tenant", tenantID, "request_id", requestID, "err", err)
		return nil
	}

	lease.meta = meta
	if !s.conditionalWritesSupported {
		return nil // no version to refresh in degrade mode
	}
	attrs, err := s.bucket.Attributes(ctx, metaPath)
	if err != nil {
		// The write landed; only the refresh failed. Keep the stale version
		// rather than nil: the next write self-orphans (bounded and
		// self-healing), while an unconditional fallback could stomp a new
		// owner's claim.
		level.Warn(s.logger).Log("msg", "failed to refresh lease version after heartbeat; a future tick may self-orphan", "tenant", tenantID, "request_id", requestID, "err", err)
		return nil
	}
	lease.version = attrs.Version
	return nil
}

// leaseExpired reports whether meta describes an in-progress query whose
// owner has stopped renewing its lease, making it eligible for adoption.
func (s *Store) leaseExpired(meta *Metadata) bool {
	return meta.Status == StatusInProgress && !meta.LastHeartbeat.IsZero() && time.Since(meta.LastHeartbeat) > s.leaseTimeout
}

// complete uploads the result, then makes one best-effort fenced attempt to
// mark the record Success: a lost mark is covered by get()'s anchor check
// and by adoption's healing pass.
func (s *Store) complete(ctx context.Context, tenantID, requestID string, lease *leaseHandle, resp *querierv1.SelectMergeStacktracesResponse) error {
	data, err := proto.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	if err := s.bucket.Upload(ctx, s.buildPath(tenantID, requestID, resultFilename), bytes.NewReader(data)); err != nil {
		return fmt.Errorf("failed to upload result: %w", err)
	}

	meta := lease.meta
	meta.Status = StatusSuccess
	meta.ErrorMessage = ""
	meta.LastHeartbeat = time.Now().UTC()
	metaPath := s.buildPath(tenantID, requestID, metadataFilename)
	if err := s.saveJSONConditional(ctx, metaPath, &meta, lease.version); err != nil {
		// result.pb is durable and served via the anchor even if this mark
		// never lands. Losing to a new owner is routine contention, so Debug.
		msg := "failed to record successful result in metadata; result is durable and served via the anchor"
		if s.bucket.IsConditionNotMetErr(err) {
			level.Debug(s.logger).Log("msg", msg, "tenant", tenantID, "request_id", requestID, "err", err)
		} else {
			level.Warn(s.logger).Log("msg", msg, "tenant", tenantID, "request_id", requestID, "err", err)
		}
	}
	return nil
}

// fail makes one fenced attempt: a version conflict means ownership moved
// and the new owner produces the outcome, so a stale failure is dropped.
func (s *Store) fail(ctx context.Context, tenantID, requestID string, lease *leaseHandle, queryErr error) error {
	meta := lease.meta
	meta.Status = StatusFailure
	meta.ErrorMessage = queryErr.Error()
	meta.LastHeartbeat = time.Now().UTC()
	metaPath := s.buildPath(tenantID, requestID, metadataFilename)
	err := s.saveJSONConditional(ctx, metaPath, &meta, lease.version)
	if err != nil && s.bucket.IsConditionNotMetErr(err) {
		level.Debug(s.logger).Log("msg", "failure mark lost race to a new owner; new owner reports the outcome", "tenant", tenantID, "request_id", requestID)
		return nil
	}
	return err
}

// markUnadoptable fails a record whose spec is confirmed gone. Not
// owner-gated: without a spec nobody, old owner or new, can produce an
// outcome.
func (s *Store) markUnadoptable(ctx context.Context, tenantID, requestID string) error {
	level.Warn(s.logger).Log("msg", "async query is unrecoverable, marking failed", "tenant", tenantID, "request_id", requestID)
	metaPath := s.buildPath(tenantID, requestID, metadataFilename)
	err := s.updateMetadata(ctx, metaPath, func(meta *Metadata) (bool, error) {
		if meta.Status.terminal() {
			return false, nil
		}
		if !s.leaseExpired(meta) {
			// The owner heartbeated since the caller's staleness judgment.
			return false, nil
		}
		meta.Status = StatusFailure
		meta.ErrorMessage = errOrphanedNoSpec.Error()
		meta.LastHeartbeat = time.Now().UTC()
		return true, nil
	})
	if err != nil && s.bucket.IsConditionNotMetErr(err) {
		return nil
	}
	return err
}

// markTooManyAdoptions fails a record already adopted defaultMaxAdoptions
// times. Not owner-gated: the refusing scanner is never the current owner.
func (s *Store) markTooManyAdoptions(ctx context.Context, tenantID, requestID string, count int) error {
	level.Warn(s.logger).Log("msg", "async query exceeded max adoption attempts, marking failed", "tenant", tenantID, "request_id", requestID, "count", count)
	metaPath := s.buildPath(tenantID, requestID, metadataFilename)
	err := s.updateMetadata(ctx, metaPath, func(meta *Metadata) (bool, error) {
		if meta.Status.terminal() {
			return false, nil
		}
		if !s.leaseExpired(meta) {
			// The owner heartbeated since the caller's staleness judgment.
			return false, nil
		}
		meta.Status = StatusFailure
		meta.ErrorMessage = errTooManyAdoptions.Error()
		meta.LastHeartbeat = time.Now().UTC()
		return true, nil
	})
	if err != nil && s.bucket.IsConditionNotMetErr(err) {
		return nil
	}
	return err
}

// healToSuccess repairs a record whose result.pb is durable but whose
// metadata never received the Success mark, instead of re-dispatching it.
func (s *Store) healToSuccess(ctx context.Context, tenantID, requestID string) error {
	metaPath := s.buildPath(tenantID, requestID, metadataFilename)
	err := s.updateMetadata(ctx, metaPath, func(meta *Metadata) (bool, error) {
		if meta.Status.terminal() {
			return false, nil
		}
		meta.Status = StatusSuccess
		meta.ErrorMessage = ""
		meta.LastHeartbeat = time.Now().UTC()
		return true, nil
	})
	if err != nil && s.bucket.IsConditionNotMetErr(err) {
		return nil
	}
	return err
}

// buildSuccessResult fetches result.pb and pairs it with meta.
func (s *Store) buildSuccessResult(ctx context.Context, tenantID, requestID string, meta Metadata) (*Result, error) {
	data, err := s.readRaw(ctx, s.buildPath(tenantID, requestID, resultFilename))
	if err != nil {
		return nil, fmt.Errorf("failed to read result: %w", err)
	}
	var resp querierv1.SelectMergeStacktracesResponse
	if err := proto.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}
	return &Result{Metadata: meta, Response: &resp}, nil
}

func (s *Store) get(ctx context.Context, tenantID, requestID string) (*Result, error) {
	if _, err := uuid.Parse(requestID); err != nil {
		return nil, fmt.Errorf("invalid request ID: %w", err)
	}

	metaPath := s.buildPath(tenantID, requestID, metadataFilename)

	var meta Metadata
	if err := s.readJSON(ctx, metaPath, &meta); err != nil {
		if s.bucket.IsObjNotFoundErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	// Tenant isolation: the metadata must belong to the requesting tenant.
	if meta.TenantID != tenantID {
		return nil, nil
	}

	if meta.Status == StatusSuccess {
		return s.buildSuccessResult(ctx, tenantID, requestID, meta)
	}

	// Unlike adopt()'s equivalent check, a transient error here is
	// non-blocking: a wrong answer on the poll path is corrected by the next
	// poll, where adopt()'s could re-dispatch a completed query.
	exists, err := s.objectExists(ctx, s.buildPath(tenantID, requestID, resultFilename))
	if err != nil {
		level.Warn(s.logger).Log("msg", "failed to check result existence; reporting persisted status", "tenant", tenantID, "request_id", requestID, "err", err)
	} else if exists {
		// A durable result proves success no matter what metadata says --
		// including a stale Failure. Report it without writing back; wrong
		// metadata heals via the scanner or ages out via TTL.
		meta.Status = StatusSuccess
		return s.buildSuccessResult(ctx, tenantID, requestID, meta)
	}

	if s.leaseExpired(&meta) {
		absent, err := s.specAbsent(ctx, tenantID, requestID)
		if err != nil {
			// Inconclusive: leave the record in progress rather than risk
			// permanently failing a live, adoptable query.
			level.Warn(s.logger).Log("msg", "failed to check spec existence; leaving query in progress", "tenant", tenantID, "request_id", requestID, "err", err)
		} else if absent {
			if err := s.markUnadoptable(ctx, tenantID, requestID); err != nil {
				level.Warn(s.logger).Log("msg", "failed to persist orphaned query state", "tenant", tenantID, "request_id", requestID, "err", err)
			} else if err := s.readJSON(ctx, metaPath, &meta); err != nil {
				// The mark landed even though this re-read failed; report the
				// state just written, not the stale snapshot.
				level.Warn(s.logger).Log("msg", "failed to re-read metadata after marking unadoptable", "tenant", tenantID, "request_id", requestID, "err", err)
				meta.Status = StatusFailure
				meta.ErrorMessage = errOrphanedNoSpec.Error()
			}
		}
		// Stale lease, spec present: recoverable, so in-progress is truthful.
		// The adoption scan re-runs it; polls never adopt, since many clients
		// can poll at once.
	}

	if meta.Status == StatusSuccess {
		return s.buildSuccessResult(ctx, tenantID, requestID, meta)
	}

	return &Result{Metadata: meta}, nil
}

func (s *Store) cleanup(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-s.ttl)
	deleted := 0

	err := s.bucket.Iter(ctx, storagePrefix, func(name string) error {
		if strings.HasSuffix(name, "/") {
			return nil
		}

		attrs, err := s.bucket.Attributes(ctx, name)
		if err != nil {
			level.Warn(s.logger).Log("msg", "failed to get attributes", "object", name, "err", err)
			return nil
		}

		if attrs.LastModified.Before(cutoff) {
			if err := s.bucket.Delete(ctx, name); err != nil {
				level.Warn(s.logger).Log("msg", "failed to delete old async query result", "object", name, "err", err)
			} else {
				deleted++
			}
		}
		return nil
	}, objstore.WithRecursiveIter())

	if err != nil {
		return deleted, fmt.Errorf("cleanup iteration failed: %w", err)
	}

	return deleted, nil
}

func (s *Store) saveJSON(ctx context.Context, path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.bucket.Upload(ctx, path, bytes.NewReader(data))
}

func (s *Store) readJSON(ctx context.Context, path string, v any) error {
	reader, err := s.bucket.Get(ctx, path)
	if err != nil {
		return err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read: %w", err)
	}

	return json.Unmarshal(data, v)
}

func (s *Store) readRaw(ctx context.Context, path string) ([]byte, error) {
	reader, err := s.bucket.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (s *Store) starting(context.Context) error { return nil }
func (s *Store) stopping(error) error           { return nil }

func (s *Store) running(ctx context.Context) error {
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer cleanupTicker.Stop()

	s.runCleanup(ctx)

	// Stagger the first scan so a fleet redeployed together doesn't scan in
	// lockstep.
	select {
	case <-time.After(time.Duration(rand.Int63n(int64(s.scanInterval)))):
	case <-ctx.Done():
		return nil
	}

	adoptionTicker := time.NewTicker(s.scanInterval)
	defer adoptionTicker.Stop()
	s.runAdoption(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-cleanupTicker.C:
			s.runCleanup(ctx)
		case <-adoptionTicker.C:
			s.runAdoption(ctx)
		}
	}
}

func (s *Store) runCleanup(ctx context.Context) {
	deleted, err := s.cleanup(ctx)
	if err != nil {
		level.Warn(s.logger).Log("msg", "async query cleanup failed", "err", err)
		return
	}
	if deleted > 0 {
		level.Info(s.logger).Log("msg", "cleaned up old async query results", "deleted", deleted)
	}
}

// runAdoption re-dispatches in-progress records whose lease has expired.
// Claims are CAS writes, so at most one racing claimer per record ever
// dispatches.
func (s *Store) runAdoption(ctx context.Context) {
	if !s.conditionalWritesSupported {
		// An unfenced claim would overwrite a live owner's lease.
		return
	}
	err := s.bucket.Iter(ctx, storagePrefix, func(name string) error {
		if path.Base(name) != metadataFilename {
			return nil
		}
		var meta Metadata
		if err := s.readJSON(ctx, name, &meta); err != nil {
			level.Warn(s.logger).Log("msg", "failed to read metadata during adoption scan", "object", name, "err", err)
			return nil
		}
		if !s.leaseExpired(&meta) {
			return nil
		}
		s.adopt(ctx, meta.TenantID, meta.RequestID)
		return nil
	}, objstore.WithRecursiveIter())
	if err != nil {
		level.Warn(s.logger).Log("msg", "async query adoption scan failed", "err", err)
	}
}

func (s *Store) adopt(ctx context.Context, tenantID, requestID string) {
	if s.dispatcher == nil {
		level.Warn(s.logger).Log("msg", "found adoptable async query but no dispatcher is configured", "tenant", tenantID, "request_id", requestID)
		return
	}

	// Best-effort peek, not atomic with the claim: a racing Submit can still
	// waste one claim. It only prevents the systematic case -- a saturated
	// frontend draining the adoption budget without executing.
	if !s.dispatcher.HasCapacity(tenantID) {
		return
	}

	metaPath := s.buildPath(tenantID, requestID, metadataFilename)

	var meta Metadata
	version, err := s.readMetadataVersioned(ctx, metaPath, &meta)
	if err != nil {
		level.Warn(s.logger).Log("msg", "failed to read metadata during adoption", "tenant", tenantID, "request_id", requestID, "err", err)
		return
	}
	if !s.leaseExpired(&meta) {
		return
	}

	// The owner may have died between uploading result.pb and marking
	// Success: heal instead of re-dispatching.
	resultExists, err := s.objectExists(ctx, s.buildPath(tenantID, requestID, resultFilename))
	if err != nil {
		level.Warn(s.logger).Log("msg", "failed to check result existence during adoption; skipping adoption this scan", "tenant", tenantID, "request_id", requestID, "err", err)
		return
	}
	if resultExists {
		if err := s.healToSuccess(ctx, tenantID, requestID); err != nil {
			level.Warn(s.logger).Log("msg", "failed to heal orphaned success result", "tenant", tenantID, "request_id", requestID, "err", err)
		}
		return
	}

	spec, err := s.readSpec(ctx, tenantID, requestID)
	if err != nil {
		if s.bucket.IsObjNotFoundErr(err) || errors.Is(err, errSpecCorrupt) {
			if err2 := s.markUnadoptable(ctx, tenantID, requestID); err2 != nil {
				level.Warn(s.logger).Log("msg", "failed to mark unadoptable query as failed", "tenant", tenantID, "request_id", requestID, "err", err2)
			}
			return
		}
		level.Warn(s.logger).Log("msg", "failed to read spec during adoption", "tenant", tenantID, "request_id", requestID, "err", err)
		return
	}

	s.adoptionAttempts.WithLabelValues(tenantID).Inc()

	// Counted before the cap check: a refused adoption is still an attempt.
	if meta.AdoptionCount >= defaultMaxAdoptions {
		if err := s.markTooManyAdoptions(ctx, tenantID, requestID, meta.AdoptionCount); err != nil {
			level.Warn(s.logger).Log("msg", "failed to persist too-many-adoptions failure", "tenant", tenantID, "request_id", requestID, "err", err)
		}
		return
	}

	token := s.newOwnerToken()
	previousOwner := meta.Owner
	leaseAge := time.Since(meta.LastHeartbeat)
	meta.Owner = token
	meta.LastHeartbeat = time.Now().UTC()
	meta.AdoptionCount++

	if err := s.saveJSONConditional(ctx, metaPath, &meta, version); err != nil {
		if s.bucket.IsConditionNotMetErr(err) {
			level.Debug(s.logger).Log("msg", "adoption claim lost race to another owner", "tenant", tenantID, "request_id", requestID)
			return
		}
		level.Warn(s.logger).Log("msg", "failed to claim orphaned query", "tenant", tenantID, "request_id", requestID, "err", err)
		return
	}
	s.adoptionSuccesses.WithLabelValues(tenantID).Inc()
	level.Info(s.logger).Log("msg", "adopted orphaned async query", "tenant", tenantID, "request_id", requestID, "previous_owner", previousOwner, "lease_age", leaseAge)

	claimedVersion, err := s.currentVersionOrNil(ctx, metaPath)
	if err != nil {
		// The claim landed but its version is unconfirmed: don't dispatch an
		// unfenced lease. The claim stands and ages back into the adoption
		// path, as if Dispatch had declined.
		level.Warn(s.logger).Log("msg", "failed to read claimed metadata version; not dispatching this scan", "tenant", tenantID, "request_id", requestID, "err", err)
		return
	}
	s.dispatcher.Dispatch(tenantID, requestID, leaseHandle{meta: meta, version: claimedVersion}, spec)
}
