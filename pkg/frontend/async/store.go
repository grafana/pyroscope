package async

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/services"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

const (
	storagePrefix            = "async-queries/"
	defaultTTL               = 30 * time.Minute
	defaultHeartbeatInterval = 15 * time.Second
	defaultHeartbeatTimeout  = 45 * time.Second
	cleanupInterval          = 5 * time.Minute
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
}

// Result bundles a query's Metadata with its decoded Response. Response is only
// populated when Metadata.Status is StatusSuccess.
type Result struct {
	Metadata Metadata
	Response *querierv1.SelectMergeStacktracesResponse
}

// Store persists async query state and results in object storage. It also runs
// as a dskit service that periodically removes expired entries.
type Store struct {
	services.Service
	logger            log.Logger
	bucket            objstore.Bucket
	ttl               time.Duration
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
}

// NewStore returns a Store backed by the given bucket. The returned Store is a
// dskit service that, once started, periodically deletes entries older than the
// configured TTL; callers are responsible for starting and stopping it via its
// embedded Service.
func NewStore(logger log.Logger, bucket objstore.Bucket) *Store {
	s := &Store{
		logger:            logger,
		bucket:            bucket,
		ttl:               defaultTTL,
		heartbeatInterval: defaultHeartbeatInterval,
		heartbeatTimeout:  defaultHeartbeatTimeout,
	}
	s.Service = services.NewBasicService(s.starting, s.running, s.stopping)
	return s
}

func (s *Store) basePath(tenantID, requestID string) string {
	return storagePrefix + tenantID + "/" + requestID + "/"
}

func (s *Store) create(ctx context.Context, tenantID, requestID string) error {
	now := time.Now().UTC()
	meta := &Metadata{
		RequestID:     requestID,
		TenantID:      tenantID,
		Status:        StatusInProgress,
		CreatedAt:     now,
		LastHeartbeat: now,
	}
	return s.saveJSON(ctx, s.basePath(tenantID, requestID)+"metadata.json", meta)
}

func (s *Store) heartbeat(ctx context.Context, tenantID, requestID string) error {
	base := s.basePath(tenantID, requestID)
	var meta Metadata
	if err := s.readJSON(ctx, base+"metadata.json", &meta); err != nil {
		return err
	}
	meta.LastHeartbeat = time.Now().UTC()
	return s.saveJSON(ctx, base+"metadata.json", &meta)
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
	meta.Status = StatusSuccess
	meta.LastHeartbeat = time.Now().UTC()
	return s.saveJSON(ctx, base+"metadata.json", &meta)
}

func (s *Store) fail(ctx context.Context, tenantID, requestID string, queryErr error) error {
	base := s.basePath(tenantID, requestID)
	var meta Metadata
	if err := s.readJSON(ctx, base+"metadata.json", &meta); err != nil {
		return fmt.Errorf("failed to read metadata: %w", err)
	}
	meta.Status = StatusFailure
	meta.ErrorMessage = queryErr.Error()
	meta.LastHeartbeat = time.Now().UTC()
	return s.saveJSON(ctx, base+"metadata.json", &meta)
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

	if meta.Status == StatusInProgress && !meta.LastHeartbeat.IsZero() && time.Since(meta.LastHeartbeat) > s.heartbeatTimeout {
		meta.Status = StatusFailure
		meta.ErrorMessage = "query appears to have been orphaned (heartbeat timed out)"
	}

	result := &Result{Metadata: meta}

	if meta.Status == StatusSuccess {
		data, err := s.readRaw(ctx, base+"result.pb")
		if err != nil {
			return nil, fmt.Errorf("failed to read result: %w", err)
		}
		var resp querierv1.SelectMergeStacktracesResponse
		if err := proto.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal result: %w", err)
		}
		result.Response = &resp
	}

	return result, nil
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
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	s.runCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.runCleanup(ctx)
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
