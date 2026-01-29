package diagnostics

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thanos-io/objstore"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

const (
	// storagePrefix is the prefix for diagnostics objects in the bucket.
	storagePrefix = "query-diagnostics/"

	// defaultTTL is how long diagnostics are kept before cleanup (7 days).
	defaultTTL = 7 * 24 * time.Hour

	// cleanupInterval is how often the cleanup routine runs.
	cleanupInterval = 1 * time.Hour
)

// StoredDiagnostics wraps the query diagnostics with metadata.
// Data is stored in separate files:
//   - <uuid>/metadata.json - ID, timestamps, tenant, response time
//   - <uuid>/request.json - query request
//   - <uuid>/plan.json - query plan
//   - <uuid>/execution.json - execution trace
type StoredDiagnostics struct {
	ID             string                 `json:"id"`
	CreatedAt      time.Time              `json:"created_at"`
	TenantID       string                 `json:"tenant_id"`
	ResponseTimeMs int64                  `json:"response_time_ms,omitempty"`
	Request        *queryv1.QueryRequest  `json:"request,omitempty"`
	Plan           *queryv1.QueryPlan     `json:"plan,omitempty"`
	Execution      *queryv1.ExecutionNode `json:"execution,omitempty"`
}

// storedMetadata is the structure saved in metadata.json
type storedMetadata struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	TenantID       string    `json:"tenant_id"`
	ResponseTimeMs int64     `json:"response_time_ms,omitempty"`
}

// Store manages query diagnostics storage and retrieval.
type Store struct {
	logger log.Logger
	bucket objstore.Bucket
	ttl    time.Duration

	// inflightDiagnostics holds diagnostics in memory before flushing to bucket.
	// Key is the diagnostics ID, value is *queryv1.Diagnostics.
	inflightDiagnostics sync.Map
}

// NewStore creates a new diagnostics store.
func NewStore(logger log.Logger, bucket objstore.Bucket) *Store {
	s := &Store{
		logger: logger,
		bucket: bucket,
		ttl:    defaultTTL,
	}
	go s.run(context.Background())
	return s
}

func (s *Store) run(ctx context.Context) error {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	// Run initial cleanup on startup
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
	deleted, err := s.Cleanup(ctx)
	if err != nil {
		level.Warn(s.logger).Log("msg", "diagnostics cleanup failed", "err", err)
		return
	}
	if deleted > 0 {
		level.Info(s.logger).Log("msg", "cleaned up old diagnostics", "deleted", deleted)
	}
}

func (s *Store) stopping(error) error { return nil }

// Add stores diagnostics in memory for later flushing.
// Called by Query() when diagnostics collection is enabled.
func (s *Store) Add(id string, diag *queryv1.Diagnostics) {
	if id == "" || diag == nil {
		return
	}
	s.inflightDiagnostics.Store(id, diag)
}

// Flush saves the in-memory diagnostics to the bucket and removes from memory.
// Called by the wrapper after the request completes.
func (s *Store) Flush(ctx context.Context, tenantID, id string, responseTimeMs int64) error {
	if id == "" || tenantID == "" {
		return nil
	}

	// Get and remove from in-memory store
	val, ok := s.inflightDiagnostics.LoadAndDelete(id)
	if !ok {
		level.Debug(s.logger).Log("msg", "no inflight diagnostics found", "id", id)
		return nil
	}

	diag, ok := val.(*queryv1.Diagnostics)
	if !ok || diag == nil {
		return nil
	}

	basePath := storagePrefix + tenantID + "/" + id + "/"

	// Save metadata
	metadata := &storedMetadata{
		ID:             id,
		CreatedAt:      time.Now().UTC(),
		TenantID:       tenantID,
		ResponseTimeMs: responseTimeMs,
	}
	if err := s.saveJSON(ctx, basePath+"metadata.json", metadata); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	// Save request if provided
	if diag.QueryRequest != nil {
		if err := s.saveJSON(ctx, basePath+"request.json", diag.QueryRequest); err != nil {
			return fmt.Errorf("failed to save query request: %w", err)
		}
	}

	// Save plan if provided and has content
	if diag.QueryPlan != nil && diag.QueryPlan.Root != nil {
		if err := s.saveJSON(ctx, basePath+"plan.json", diag.QueryPlan); err != nil {
			return fmt.Errorf("failed to save query plan: %w", err)
		}
	}

	// Save execution trace if provided
	if diag.ExecutionNode != nil {
		if err := s.saveJSON(ctx, basePath+"execution.json", diag.ExecutionNode); err != nil {
			return fmt.Errorf("failed to save execution trace: %w", err)
		}
	}

	level.Debug(s.logger).Log(
		"msg", "stored query diagnostics",
		"id", id,
		"tenant_id", tenantID,
		"response_time_ms", responseTimeMs,
	)

	return nil
}

// SaveDirect saves diagnostics directly to the store without going through the in-memory cache.
// This is used by the admin UI for re-running queries.
func (s *Store) SaveDirect(ctx context.Context, tenantID string, responseTimeMs int64, request *queryv1.QueryRequest, plan *queryv1.QueryPlan, execution *queryv1.ExecutionNode) (string, error) {
	id := generateUUID()
	basePath := storagePrefix + tenantID + "/" + id + "/"

	// Save metadata
	metadata := &storedMetadata{
		ID:             id,
		CreatedAt:      time.Now().UTC(),
		TenantID:       tenantID,
		ResponseTimeMs: responseTimeMs,
	}
	if err := s.saveJSON(ctx, basePath+"metadata.json", metadata); err != nil {
		return "", fmt.Errorf("failed to save metadata: %w", err)
	}

	// Save request if provided
	if request != nil {
		if err := s.saveJSON(ctx, basePath+"request.json", request); err != nil {
			return "", fmt.Errorf("failed to save query request: %w", err)
		}
	}

	// Save plan if provided and has content
	if plan != nil && plan.Root != nil {
		if err := s.saveJSON(ctx, basePath+"plan.json", plan); err != nil {
			return "", fmt.Errorf("failed to save query plan: %w", err)
		}
	}

	// Save execution trace if provided
	if execution != nil {
		if err := s.saveJSON(ctx, basePath+"execution.json", execution); err != nil {
			return "", fmt.Errorf("failed to save execution trace: %w", err)
		}
	}

	level.Debug(s.logger).Log(
		"msg", "stored query diagnostics (direct)",
		"id", id,
		"tenant_id", tenantID,
		"response_time_ms", responseTimeMs,
	)

	return id, nil
}

func (s *Store) saveJSON(ctx context.Context, path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.bucket.Upload(ctx, path, bytes.NewReader(data))
}

// Get retrieves diagnostics by tenant and ID.
func (s *Store) Get(ctx context.Context, tenantID, id string) (*StoredDiagnostics, error) {
	if !isValidUUID(id) {
		return nil, fmt.Errorf("invalid diagnostics ID: %s", id)
	}

	basePath := storagePrefix + tenantID + "/" + id + "/"

	// Read metadata (required)
	var metadata storedMetadata
	if err := s.readJSON(ctx, basePath+"metadata.json", &metadata); err != nil {
		if s.bucket.IsObjNotFoundErr(err) {
			return nil, fmt.Errorf("diagnostics not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get diagnostics: %w", err)
	}

	stored := &StoredDiagnostics{
		ID:             metadata.ID,
		CreatedAt:      metadata.CreatedAt,
		TenantID:       metadata.TenantID,
		ResponseTimeMs: metadata.ResponseTimeMs,
	}

	// Read request (optional)
	var request queryv1.QueryRequest
	if err := s.readJSON(ctx, basePath+"request.json", &request); err == nil {
		stored.Request = &request
	} else if !s.bucket.IsObjNotFoundErr(err) {
		level.Warn(s.logger).Log("msg", "failed to read query request", "id", id, "err", err)
	}

	// Read query plan (optional)
	var plan queryv1.QueryPlan
	if err := s.readJSON(ctx, basePath+"plan.json", &plan); err == nil {
		stored.Plan = &plan
	} else if !s.bucket.IsObjNotFoundErr(err) {
		level.Warn(s.logger).Log("msg", "failed to read query plan", "id", id, "err", err)
	}

	// Read execution trace (optional)
	var execution queryv1.ExecutionNode
	if err := s.readJSON(ctx, basePath+"execution.json", &execution); err == nil {
		stored.Execution = &execution
	} else if !s.bucket.IsObjNotFoundErr(err) {
		level.Warn(s.logger).Log("msg", "failed to read execution trace", "id", id, "err", err)
	}

	return stored, nil
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

// Delete removes diagnostics by tenant and ID.
func (s *Store) Delete(ctx context.Context, tenantID, id string) error {
	if !isValidUUID(id) {
		return fmt.Errorf("invalid diagnostics ID: %s", id)
	}

	basePath := storagePrefix + tenantID + "/" + id + "/"
	files := []string{"metadata.json", "request.json", "plan.json", "execution.json"}

	for _, file := range files {
		if err := s.bucket.Delete(ctx, basePath+file); err != nil {
			if !s.bucket.IsObjNotFoundErr(err) {
				return fmt.Errorf("failed to delete diagnostics file %s: %w", file, err)
			}
		}
	}

	return nil
}

// ListTenants returns a list of all tenants that have stored diagnostics.
func (s *Store) ListTenants(ctx context.Context) ([]string, error) {
	tenants := make(map[string]struct{})

	err := s.bucket.Iter(ctx, storagePrefix, func(name string) error {
		// Extract tenant from path: query-diagnostics/<tenant>/
		rel := strings.TrimPrefix(name, storagePrefix)
		if idx := strings.Index(rel, "/"); idx > 0 {
			tenant := rel[:idx]
			tenants[tenant] = struct{}{}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	result := make([]string, 0, len(tenants))
	for tenant := range tenants {
		result = append(result, tenant)
	}
	sort.Strings(result)
	return result, nil
}

// DiagnosticSummary contains minimal info for listing diagnostics.
type DiagnosticSummary struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	QueryType      string    `json:"query_type"`
	StartTime      int64     `json:"start_time"`
	EndTime        int64     `json:"end_time"`
	LabelSelector  string    `json:"label_selector"`
	ResponseTimeMs int64     `json:"response_time_ms"`
}

// ListByTenant returns all diagnostics for a given tenant.
func (s *Store) ListByTenant(ctx context.Context, tenant string) ([]*DiagnosticSummary, error) {
	var summaries []*DiagnosticSummary

	prefix := storagePrefix + tenant + "/"
	err := s.bucket.Iter(ctx, prefix, func(name string) error {
		// Only process metadata.json files
		if !strings.HasSuffix(name, "/metadata.json") {
			return nil
		}

		// Read metadata
		var metadata storedMetadata
		if err := s.readJSON(ctx, name, &metadata); err != nil {
			level.Warn(s.logger).Log("msg", "failed to read diagnostics metadata", "object", name, "err", err)
			return nil
		}

		summary := &DiagnosticSummary{
			ID:             metadata.ID,
			CreatedAt:      metadata.CreatedAt,
			ResponseTimeMs: metadata.ResponseTimeMs,
		}

		// Try to read request for additional info
		basePath := strings.TrimSuffix(name, "metadata.json")
		var request queryv1.QueryRequest
		if err := s.readJSON(ctx, basePath+"request.json", &request); err == nil {
			summary.StartTime = request.StartTime
			summary.EndTime = request.EndTime
			summary.LabelSelector = request.LabelSelector
			if len(request.Query) > 0 {
				summary.QueryType = request.Query[0].QueryType.String()
			}
		}

		summaries = append(summaries, summary)
		return nil
	}, objstore.WithRecursiveIter())

	if err != nil {
		return nil, fmt.Errorf("failed to list diagnostics: %w", err)
	}

	// Sort by CreatedAt descending (newest first)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].CreatedAt.After(summaries[j].CreatedAt)
	})

	return summaries, nil
}

// Cleanup removes diagnostics older than the TTL.
func (s *Store) Cleanup(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-s.ttl)
	deleted := 0

	err := s.bucket.Iter(ctx, storagePrefix, func(name string) error {
		if strings.HasSuffix(name, "/") {
			return nil
		}
		if !strings.HasSuffix(name, ".json") {
			return nil
		}

		attrs, err := s.bucket.Attributes(ctx, name)
		if err != nil {
			level.Warn(s.logger).Log("msg", "failed to get attributes", "object", name, "err", err)
			return nil
		}

		if attrs.LastModified.Before(cutoff) {
			if err := s.bucket.Delete(ctx, name); err != nil {
				level.Warn(s.logger).Log("msg", "failed to delete old diagnostics", "object", name, "err", err)
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

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func isValidUUID(uuid string) bool {
	if len(uuid) != 32 {
		return false
	}
	for _, c := range uuid {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
