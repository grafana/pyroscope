package async

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
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
	defaultTTL                  = 30 * time.Minute
	defaultHeartbeatInterval    = 15 * time.Second
	defaultLeaseTimeout         = 45 * time.Second
	defaultAdoptionScanInterval = 30 * time.Second
	cleanupInterval             = 5 * time.Minute
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

// Metadata describes the state of a single async query. It is persisted as
// metadata.json alongside the query's result in object storage.
type Metadata struct {
	RequestID     string    `json:"request_id"`
	TenantID      string    `json:"tenant_id"`
	Status        Status    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	LastHeartbeat time.Time `json:"last_heartbeat,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	Owner         string    `json:"owner,omitempty"`
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
	Dispatch(ctx context.Context, tenantID, requestID string, spec *querierv1.SelectMergeStacktracesRequest)
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

	adoptionAttempts  *prometheus.CounterVec
	adoptionSuccesses *prometheus.CounterVec
}

// NewStore returns a Store backed by the given bucket. The returned Store is a
// dskit service that, once started, periodically deletes entries older than the
// configured TTL and scans for adoptable queries; callers are responsible for
// starting and stopping it via its embedded Service.
func NewStore(logger log.Logger, bucket objstore.Bucket, reg prometheus.Registerer) *Store {
	hostname, _ := os.Hostname()
	s := &Store{
		logger:            logger,
		bucket:            bucket,
		ttl:               defaultTTL,
		heartbeatInterval: defaultHeartbeatInterval,
		leaseTimeout:      defaultLeaseTimeout,
		scanInterval:      defaultAdoptionScanInterval,
		ownerID:           hostname,
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

// SetDispatcher wires the adoption scan to a Dispatcher. Must be called before
// the Store's Service is started: s.dispatcher is a plain field, never
// synchronized against running()'s reads of it, so calling this after the
// Service has started is a genuine data race, not merely a logic error. nil is
// safe (adoption is then a no-op), e.g. when Store runs standalone or in tests.
func (s *Store) SetDispatcher(d Dispatcher) { s.dispatcher = d }

func (s *Store) basePath(tenantID, requestID string) string {
	return storagePrefix + tenantID + "/" + requestID + "/"
}

func (s *Store) create(ctx context.Context, tenantID, requestID string, spec *querierv1.SelectMergeStacktracesRequest) error {
	base := s.basePath(tenantID, requestID)

	data, err := proto.Marshal(spec)
	if err != nil {
		return fmt.Errorf("failed to marshal spec: %w", err)
	}
	// spec.pb must land before metadata.json: metadata.json existing is what a
	// poller/scanner treats as "this record is real", so writing spec first means
	// a crash between the two uploads can never leave a spec-less in_progress
	// record that this code path created.
	if err := s.bucket.Upload(ctx, base+"spec.pb", bytes.NewReader(data)); err != nil {
		return fmt.Errorf("failed to upload spec: %w", err)
	}

	now := time.Now().UTC()
	meta := &Metadata{
		RequestID:     requestID,
		TenantID:      tenantID,
		Status:        StatusInProgress,
		CreatedAt:     now,
		LastHeartbeat: now,
		Owner:         s.ownerID,
	}
	return s.saveJSON(ctx, base+"metadata.json", meta)
}

// errSpecCorrupt indicates spec.pb exists but its contents don't decode.
// Unlike a transient read error, retrying can never fix this.
var errSpecCorrupt = errors.New("spec is present but could not be decoded")

func (s *Store) readSpec(ctx context.Context, tenantID, requestID string) (*querierv1.SelectMergeStacktracesRequest, error) {
	data, err := s.readRaw(ctx, s.basePath(tenantID, requestID)+"spec.pb")
	if err != nil {
		return nil, err
	}
	var spec querierv1.SelectMergeStacktracesRequest
	if err := proto.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("%w: %v", errSpecCorrupt, err)
	}
	return &spec, nil
}

// objectExists reports whether the object at path is confirmed present. Any
// error other than IsObjNotFoundErr is inconclusive (e.g. a transient
// object-storage error) and must not be treated as either presence or
// absence.
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
	exists, err := s.objectExists(ctx, s.basePath(tenantID, requestID)+"spec.pb")
	if err != nil {
		return false, err
	}
	return !exists, nil
}

// resultExists reports whether result.pb has been written. Only successful
// executions write result.pb, so its existence proves success even when
// metadata.json disagrees. Racing duplicate executions may replace it with
// another valid result: existence, not content, is the invariant.
func (s *Store) resultExists(ctx context.Context, tenantID, requestID string) (bool, error) {
	return s.objectExists(ctx, s.basePath(tenantID, requestID)+"result.pb")
}

func (s *Store) heartbeat(ctx context.Context, tenantID, requestID string) error {
	base := s.basePath(tenantID, requestID)
	var meta Metadata
	if err := s.readJSON(ctx, base+"metadata.json", &meta); err != nil {
		return err
	}
	if meta.Status != StatusInProgress {
		// Already terminal: a lingering heartbeat from a still-alive duplicate
		// execution (post-adoption) must not resurrect a completed/failed record.
		return nil
	}
	meta.LastHeartbeat = time.Now().UTC()
	meta.Owner = s.ownerID
	return s.saveJSON(ctx, base+"metadata.json", &meta)
}

// leaseExpired reports whether meta describes an in-progress query whose
// owner has stopped renewing its lease, making it eligible for adoption.
func (s *Store) leaseExpired(meta *Metadata) bool {
	return meta.Status == StatusInProgress && !meta.LastHeartbeat.IsZero() && time.Since(meta.LastHeartbeat) > s.leaseTimeout
}

func (s *Store) complete(ctx context.Context, tenantID, requestID string, resp *querierv1.SelectMergeStacktracesResponse) error {
	base := s.basePath(tenantID, requestID)

	data, err := proto.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	if err := s.bucket.Upload(ctx, base+"result.pb", bytes.NewReader(data)); err != nil {
		return fmt.Errorf("failed to upload result: %w", err)
	}

	var meta Metadata
	if err := s.readJSON(ctx, base+"metadata.json", &meta); err != nil {
		return fmt.Errorf("failed to read metadata: %w", err)
	}
	if meta.Status == StatusSuccess {
		// Already recorded success (this execution's own write racing itself,
		// or a duplicate's); nothing left to do.
		return nil
	}
	// A real success always wins, including over a prior Failure: result.pb
	// was just durably written above, so at least one execution produced the
	// correct answer and it must be reported regardless of what any other,
	// differently-outcome'd execution wrote.
	meta.Status = StatusSuccess
	meta.ErrorMessage = ""
	meta.LastHeartbeat = time.Now().UTC()
	meta.Owner = s.ownerID
	return s.saveJSON(ctx, base+"metadata.json", &meta)
}

func (s *Store) fail(ctx context.Context, tenantID, requestID string, queryErr error) error {
	base := s.basePath(tenantID, requestID)
	var meta Metadata
	if err := s.readJSON(ctx, base+"metadata.json", &meta); err != nil {
		return fmt.Errorf("failed to read metadata: %w", err)
	}
	if meta.Status != StatusInProgress {
		return nil
	}
	meta.Status = StatusFailure
	meta.ErrorMessage = queryErr.Error()
	meta.LastHeartbeat = time.Now().UTC()
	meta.Owner = s.ownerID
	return s.saveJSON(ctx, base+"metadata.json", &meta)
}

// errOrphanedNoSpec is the single, shared definition of "unrecoverable" used by
// both get() and the adoption scan: a stale in-progress record whose spec is
// gone can never be resumed.
var errOrphanedNoSpec = errors.New("query orphaned: owner heartbeat expired and no request spec is available for adoption")

func (s *Store) markUnadoptable(ctx context.Context, tenantID, requestID string) error {
	level.Warn(s.logger).Log("msg", "async query is unrecoverable, marking failed", "tenant", tenantID, "request_id", requestID)
	return s.fail(ctx, tenantID, requestID, errOrphanedNoSpec)
}

// buildSuccessResult fetches and unmarshals result.pb, returning a Result
// with Status forced to Success. Callers only reach this once they've
// already established (via meta.Status or result.pb's own existence) that
// the query succeeded.
func (s *Store) buildSuccessResult(ctx context.Context, tenantID, requestID string, meta Metadata) (*Result, error) {
	data, err := s.readRaw(ctx, s.basePath(tenantID, requestID)+"result.pb")
	if err != nil {
		return nil, fmt.Errorf("failed to read result: %w", err)
	}
	var resp querierv1.SelectMergeStacktracesResponse
	if err := proto.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}
	meta.Status = StatusSuccess
	return &Result{Metadata: meta, Response: &resp}, nil
}

// healToSuccess best-effort corrects metadata.json's Status once result.pb's
// existence has already proven a real success occurred. No guard is needed:
// this write always converges to the one fixed value the anchor already
// proved correct, unlike every other write in this file, so it's safe to
// issue redundantly and to ignore on failure.
// The caller's snapshot may overwrite a concurrent writer's Owner or
// LastHeartbeat, but both are advisory once a record is terminal: heartbeats
// no-op on terminal records, adoption stops at the anchor, and cleanup keys
// on object timestamps, so neither field drives any further decision.
func (s *Store) healToSuccess(ctx context.Context, tenantID, requestID string, meta Metadata) {
	meta.Status = StatusSuccess
	meta.ErrorMessage = ""
	if err := s.saveJSON(ctx, s.basePath(tenantID, requestID)+"metadata.json", &meta); err != nil {
		level.Warn(s.logger).Log("msg", "failed to heal metadata to success", "request_id", requestID, "err", err)
	}
}

func (s *Store) get(ctx context.Context, tenantID, requestID string) (*Result, error) {
	if _, err := uuid.Parse(requestID); err != nil {
		return nil, fmt.Errorf("invalid request ID: %w", err)
	}

	base := s.basePath(tenantID, requestID)

	var meta Metadata
	if err := s.readJSON(ctx, base+"metadata.json", &meta); err != nil {
		if s.bucket.IsObjNotFoundErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	// Tenant isolation: the metadata must belong to the requesting tenant.
	if meta.TenantID != tenantID {
		return nil, nil
	}

	// Object storage has no compare-and-swap, so a stale writer can always
	// overwrite metadata.json and make its Status lie. result.pb is different:
	// it is written exactly once, by a successful execution, and never
	// modified, so once it exists nothing can un-write it. Check for it first
	// so a poll reports success whenever a result genuinely exists, no matter
	// what the metadata says.
	if meta.Status != StatusSuccess {
		exists, err := s.resultExists(ctx, tenantID, requestID)
		if err != nil {
			level.Warn(s.logger).Log("msg", "failed to check result existence; falling back to persisted status", "request_id", requestID, "err", err)
		} else if exists {
			result, err := s.buildSuccessResult(ctx, tenantID, requestID, meta)
			if err != nil {
				return nil, err
			}
			s.healToSuccess(ctx, tenantID, requestID, meta)
			return result, nil
		}
	}

	if s.leaseExpired(&meta) {
		absent, err := s.specAbsent(ctx, tenantID, requestID)
		if err != nil {
			// Inconclusive (e.g. a transient object-storage error): leave the
			// record in progress rather than risk permanently failing a
			// live, adoptable query. A later poll or the adoption scan will
			// re-evaluate it.
			level.Warn(s.logger).Log("msg", "failed to check spec existence; leaving query in progress", "request_id", requestID, "err", err)
		} else if absent {
			if err := s.markUnadoptable(ctx, tenantID, requestID); err != nil {
				level.Warn(s.logger).Log("msg", "failed to persist orphaned query state", "request_id", requestID, "err", err)
			} else {
				meta.Status = StatusFailure
				meta.ErrorMessage = errOrphanedNoSpec.Error()
			}
		}
		// A stale lease with the spec still present means the owner died but
		// the query is recoverable, so in-progress is the truthful answer.
		// The poll path deliberately never adopts because many clients can
		// poll concurrently. Instead, the periodic adoption scan will claim
		// and re-run it.
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

// jitteredInterval returns a duration uniformly distributed in
// [0.8*d, 1.2*d), i.e. d plus or minus 20%.
func jitteredInterval(d time.Duration) time.Duration {
	return time.Duration((0.8 + 0.4*rand.Float64()) * float64(d))
}

func (s *Store) running(ctx context.Context) error {
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer cleanupTicker.Stop()

	// Randomizing the initial delay breaks the synchronized start across
	// frontends; re-randomizing the period each tick keeps it broken.
	adoptionTimer := time.NewTimer(rand.N(s.scanInterval))
	defer adoptionTimer.Stop()

	s.runCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-cleanupTicker.C:
			s.runCleanup(ctx)
		case <-adoptionTimer.C:
			s.runAdoption(ctx)
			adoptionTimer.Reset(jitteredInterval(s.scanInterval))
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

// runAdoption lists every object under storagePrefix but filters to just
// metadata.json. The number of listed objects is bounded by cleanup()'s TTL.
//
// No cap on adoptions per tick: actual query re-execution is already
// bounded per tenant by Coordinator.Dispatch's own tryAcquire. This does
// not protect against the aggregate burst of re-executions summed across
// many tenants at once, e.g. right after one replica dies while holding
// queries for many tenants. A claim whose dispatch is declined does no
// further work.
//
// A query that repeatedly kills its owner (e.g. one that OOMs the frontend)
// is re-adopted at most once per leaseTimeout+scanInterval, and cleanup()
// bounds the loop: spec.pb is written once and never rewritten, so it ages
// out after the TTL even while metadata.json stays fresh, at which point the
// next scan finds no spec and persists a terminal failure. Worst case is
// roughly ttl/(leaseTimeout+scanInterval) re-executions, each still subject
// to the tenant's concurrency limit.
func (s *Store) runAdoption(ctx context.Context) {
	err := s.bucket.Iter(ctx, storagePrefix, func(name string) error {
		if !strings.HasSuffix(name, "metadata.json") {
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
		s.adopt(ctx, meta)
		return nil
	}, objstore.WithRecursiveIter())
	if err != nil {
		level.Warn(s.logger).Log("msg", "async query adoption scan failed", "err", err)
	}
}

func (s *Store) adopt(ctx context.Context, meta Metadata) {
	if exists, err := s.resultExists(ctx, meta.TenantID, meta.RequestID); err != nil {
		level.Warn(s.logger).Log("msg", "failed to check result existence before adoption; proceeding", "request_id", meta.RequestID, "err", err)
	} else if exists {
		s.healToSuccess(ctx, meta.TenantID, meta.RequestID, meta)
		return
	}

	spec, err := s.readSpec(ctx, meta.TenantID, meta.RequestID)
	if err != nil {
		if s.bucket.IsObjNotFoundErr(err) || errors.Is(err, errSpecCorrupt) {
			if err2 := s.markUnadoptable(ctx, meta.TenantID, meta.RequestID); err2 != nil {
				level.Warn(s.logger).Log("msg", "failed to mark unadoptable query as failed", "request_id", meta.RequestID, "err", err2)
			}
			return
		}
		level.Warn(s.logger).Log("msg", "failed to read spec during adoption", "request_id", meta.RequestID, "err", err)
		return
	}

	s.adoptionAttempts.WithLabelValues(meta.TenantID).Inc()
	if s.dispatcher == nil {
		level.Warn(s.logger).Log("msg", "found adoptable async query but no dispatcher is configured", "request_id", meta.RequestID)
		return
	}

	// meta is a snapshot from the scan; readSpec above was a real round trip
	// during which the true owner could have completed, failed, or already
	// been re-claimed. Re-read immediately before the claim write so that
	// window can't revert a now-terminal (or now-fresh) record.
	//
	// Two scanners can still both pass this check and both claim; the
	// consequence is bounded to one duplicate execution, and the result stays
	// correct because complete()'s terminal guard and the result.pb anchor
	// make the first success stick.
	base := s.basePath(meta.TenantID, meta.RequestID)
	var fresh Metadata
	if err := s.readJSON(ctx, base+"metadata.json", &fresh); err != nil {
		level.Warn(s.logger).Log("msg", "failed to re-read metadata before claim", "request_id", meta.RequestID, "err", err)
		return
	}
	if !s.leaseExpired(&fresh) {
		return
	}
	if exists, err := s.resultExists(ctx, meta.TenantID, meta.RequestID); err != nil {
		level.Warn(s.logger).Log("msg", "failed to check result existence before claim; proceeding", "request_id", meta.RequestID, "err", err)
	} else if exists {
		s.healToSuccess(ctx, meta.TenantID, meta.RequestID, fresh)
		return
	}

	previousOwner := fresh.Owner
	leaseAge := time.Since(fresh.LastHeartbeat)
	fresh.Owner = s.ownerID
	fresh.LastHeartbeat = time.Now().UTC()
	if err := s.saveJSON(ctx, base+"metadata.json", &fresh); err != nil {
		level.Warn(s.logger).Log("msg", "failed to claim orphaned query", "request_id", meta.RequestID, "err", err)
		return
	}
	s.adoptionSuccesses.WithLabelValues(meta.TenantID).Inc()
	level.Info(s.logger).Log("msg", "adopted orphaned async query", "tenant", meta.TenantID, "request_id", meta.RequestID, "previous_owner", previousOwner, "lease_age", leaseAge)
	s.dispatcher.Dispatch(ctx, meta.TenantID, meta.RequestID, spec)
}
