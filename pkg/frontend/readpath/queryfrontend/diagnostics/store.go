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
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"
	"github.com/thanos-io/objstore"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

const (
	// RequestHeader is the header clients send to request diagnostics collection.
	RequestHeader = "X-Pyroscope-Collect-Diagnostics"

	// ResponseIDHeader is the header containing the diagnostics ID in responses.
	ResponseIDHeader = "X-Pyroscope-Diagnostics-Id"

	// ResponseBlocksReadHeader contains the number of blocks read.
	ResponseBlocksReadHeader = "X-Pyroscope-Blocks-Read"

	// ResponseExecutionTimeHeader contains the total execution time in milliseconds.
	ResponseExecutionTimeHeader = "X-Pyroscope-Execution-Time-Ms"

	// storagePrefix is the prefix for diagnostics objects in the bucket.
	storagePrefix = "query-diagnostics/"

	// defaultTTL is how long diagnostics are kept before cleanup (7 days).
	defaultTTL = 7 * 24 * time.Hour

	// cleanupInterval is how often the cleanup routine runs.
	cleanupInterval = 1 * time.Hour
)

// StoredDiagnostics wraps the query diagnostics with metadata.
// Data is stored in separate files:
//   - <uuid>/query.json - metadata and query parameters
//   - <uuid>/plan.json - query plan
//   - <uuid>/execution.json - execution trace
type StoredDiagnostics struct {
	ID        string                 `json:"id"`
	CreatedAt time.Time              `json:"created_at"`
	TenantID  string                 `json:"tenant_id"`
	Query     *StoredQuery           `json:"query,omitempty"`
	Plan      *queryv1.QueryPlan     `json:"plan,omitempty"`
	Execution *queryv1.ExecutionNode `json:"execution,omitempty"`
}

// storedMetadata is the structure saved in query.json
type storedMetadata struct {
	ID        string       `json:"id"`
	CreatedAt time.Time    `json:"created_at"`
	TenantID  string       `json:"tenant_id"`
	Query     *StoredQuery `json:"query,omitempty"`
}

// StoredQuery contains the query parameters for reference.
type StoredQuery struct {
	StartTime     int64  `json:"start_time"`
	EndTime       int64  `json:"end_time"`
	LabelSelector string `json:"label_selector,omitempty"`
	QueryType     string `json:"query_type"`

	// Query-type specific parameters
	MaxNodes         int64    `json:"max_nodes,omitempty"`
	Step             float64  `json:"step,omitempty"`
	GroupBy          []string `json:"group_by,omitempty"`
	Limit            int64    `json:"limit,omitempty"`
	LabelName        string   `json:"label_name,omitempty"`
	SeriesLabelNames []string `json:"series_label_names,omitempty"`

	// Response metadata
	ResponseTimeMs int64 `json:"response_time_ms,omitempty"`
}

// Store manages query diagnostics storage and retrieval.
type Store struct {
	service services.Service
	logger  log.Logger
	bucket  objstore.Bucket
	ttl     time.Duration
}

// NewStore creates a new diagnostics store.
func NewStore(logger log.Logger, bucket objstore.Bucket) *Store {
	s := &Store{
		logger: logger,
		bucket: bucket,
		ttl:    defaultTTL,
	}
	s.service = services.NewBasicService(s.starting, s.running, s.stopping)
	return s
}

func (s *Store) Service() services.Service {
	return s.service
}

func (s *Store) starting(context.Context) error { return nil }

func (s *Store) running(ctx context.Context) error {
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

// Save stores diagnostics and returns the generated ID (UUID only).
// Data is stored in separate files:
//   - query-diagnostics/<tenant>/<uuid>/query.json
//   - query-diagnostics/<tenant>/<uuid>/plan.json
//   - query-diagnostics/<tenant>/<uuid>/execution.json
func (s *Store) Save(ctx context.Context, tenantID string, query *StoredQuery, plan *queryv1.QueryPlan, execution *queryv1.ExecutionNode) (string, error) {
	uuid := generateUUID()
	basePath := storagePrefix + tenantID + "/" + uuid + "/"

	// Save query metadata
	metadata := &storedMetadata{
		ID:        uuid,
		CreatedAt: time.Now().UTC(),
		TenantID:  tenantID,
		Query:     query,
	}
	if err := s.saveJSON(ctx, basePath+"query.json", metadata); err != nil {
		return "", fmt.Errorf("failed to save query metadata: %w", err)
	}

	// Save plan if provided and has content
	if plan != nil && plan.Root != nil {
		if err := s.saveJSON(ctx, basePath+"plan.json", plan); err != nil {
			return "", fmt.Errorf("failed to save query plan: %w", err)
		}
	} else {
		level.Debug(s.logger).Log("msg", "no query plan to save", "plan_nil", plan == nil, "root_nil", plan == nil || plan.Root == nil)
	}

	// Save execution trace if provided
	if execution != nil {
		if err := s.saveJSON(ctx, basePath+"execution.json", execution); err != nil {
			return "", fmt.Errorf("failed to save execution trace: %w", err)
		}
	}

	level.Debug(s.logger).Log(
		"msg", "stored query diagnostics",
		"id", uuid,
		"tenant_id", tenantID,
	)

	return uuid, nil
}

func (s *Store) saveJSON(ctx context.Context, path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.bucket.Upload(ctx, path, bytes.NewReader(data))
}

// Get retrieves diagnostics by tenant and ID.
// Data is read from separate files:
//   - <uuid>/query.json - metadata and query parameters
//   - <uuid>/plan.json - query plan (optional)
//   - <uuid>/execution.json - execution trace (optional)
func (s *Store) Get(ctx context.Context, tenantID, id string) (*StoredDiagnostics, error) {
	if !isValidUUID(id) {
		return nil, fmt.Errorf("invalid diagnostics ID: %s", id)
	}

	basePath := storagePrefix + tenantID + "/" + id + "/"

	// Read query metadata (required)
	var metadata storedMetadata
	if err := s.readJSON(ctx, basePath+"query.json", &metadata); err != nil {
		if s.bucket.IsObjNotFoundErr(err) {
			return nil, fmt.Errorf("diagnostics not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get diagnostics: %w", err)
	}

	stored := &StoredDiagnostics{
		ID:        metadata.ID,
		CreatedAt: metadata.CreatedAt,
		TenantID:  metadata.TenantID,
		Query:     metadata.Query,
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
// Deletes all files in the diagnostic directory.
func (s *Store) Delete(ctx context.Context, tenantID, id string) error {
	if !isValidUUID(id) {
		return fmt.Errorf("invalid diagnostics ID: %s", id)
	}

	basePath := storagePrefix + tenantID + "/" + id + "/"
	files := []string{"query.json", "plan.json", "execution.json"}

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
	// Sort for consistent ordering
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
// Searches for query.json files in subdirectories: <tenant>/<uuid>/query.json
func (s *Store) ListByTenant(ctx context.Context, tenant string) ([]*DiagnosticSummary, error) {
	var summaries []*DiagnosticSummary

	prefix := storagePrefix + tenant + "/"
	err := s.bucket.Iter(ctx, prefix, func(name string) error {
		// Only process query.json files (metadata files)
		if !strings.HasSuffix(name, "/query.json") {
			return nil
		}

		// Read and parse the metadata to get summary info
		var metadata storedMetadata
		if err := s.readJSON(ctx, name, &metadata); err != nil {
			level.Warn(s.logger).Log("msg", "failed to read diagnostics metadata", "object", name, "err", err)
			return nil
		}

		summary := &DiagnosticSummary{
			ID:        metadata.ID,
			CreatedAt: metadata.CreatedAt,
		}
		if metadata.Query != nil {
			summary.QueryType = metadata.Query.QueryType
			summary.StartTime = metadata.Query.StartTime
			summary.EndTime = metadata.Query.EndTime
			summary.LabelSelector = metadata.Query.LabelSelector
			summary.ResponseTimeMs = metadata.Query.ResponseTimeMs
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

	// Use recursive iteration to find all .json files under tenant subdirectories
	err := s.bucket.Iter(ctx, storagePrefix, func(name string) error {
		// Skip directories (they end with /)
		if strings.HasSuffix(name, "/") {
			return nil
		}
		// Only process .json files
		if !strings.HasSuffix(name, ".json") {
			return nil
		}

		// Get object attributes to check age
		attrs, err := s.bucket.Attributes(ctx, name)
		if err != nil {
			level.Warn(s.logger).Log("msg", "failed to get attributes", "object", name, "err", err)
			return nil // Continue iteration
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

	if deleted > 0 {
		level.Info(s.logger).Log("msg", "cleaned up old diagnostics", "deleted", deleted)
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
