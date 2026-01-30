package diagnostics

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
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
//   - <uuid>/metadata.json - ID, timestamps, tenant, response time, method
//   - <uuid>/request.json - the original request (JSON serialized)
//   - <uuid>/plan.json - query plan
//   - <uuid>/execution.json - execution trace
type StoredDiagnostics struct {
	ID                string                 `json:"id"`
	CreatedAt         time.Time              `json:"created_at"`
	TenantID          string                 `json:"tenant_id"`
	ResponseTimeMs    int64                  `json:"response_time_ms,omitempty"`
	ResponseSizeBytes int64                  `json:"response_size_bytes,omitempty"`
	Method            string                 `json:"method,omitempty"`
	Request           json.RawMessage        `json:"request,omitempty"`
	Response          json.RawMessage        `json:"response,omitempty"`
	Plan              *queryv1.QueryPlan     `json:"plan,omitempty"`
	Execution         *queryv1.ExecutionNode `json:"execution,omitempty"`
}

// storedMetadata is the structure saved in metadata.json
type storedMetadata struct {
	ID                string    `json:"id"`
	CreatedAt         time.Time `json:"created_at"`
	TenantID          string    `json:"tenant_id"`
	ResponseTimeMs    int64     `json:"response_time_ms,omitempty"`
	ResponseSizeBytes int64     `json:"response_size_bytes,omitempty"`
	Method            string    `json:"method,omitempty"`
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

// StoreOption is a functional option for configuring a Store.
type StoreOption func(*Store)

// WithTTL sets the TTL for stored diagnostics.
func WithTTL(ttl time.Duration) StoreOption {
	return func(s *Store) {
		s.ttl = ttl
	}
}

// NewStore creates a new diagnostics store.
func NewStore(logger log.Logger, bucket objstore.Bucket, opts ...StoreOption) *Store {
	s := &Store{
		logger: logger,
		bucket: bucket,
		ttl:    defaultTTL,
	}
	for _, opt := range opts {
		opt(s)
	}
	go s.run(context.Background())
	return s
}

func (s *Store) run(ctx context.Context) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	// Run initial cleanup on startup
	s.runCleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return
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

// inflightData holds request info and diagnostics for an in-progress request.
type inflightData struct {
	Method            string
	Request           any // The original request
	Response          any // The original response
	ResponseSizeBytes int64
	ResponseTimeMs    int64
	Diagnostics       *queryv1.Diagnostics
}

// Add stores diagnostics in memory for later flushing.
// Called by Query() when diagnostics collection is enabled.
func (s *Store) Add(id string, diag *queryv1.Diagnostics) {
	if id == "" || diag == nil {
		return
	}
	// Load existing inflight data or create new
	val, _ := s.inflightDiagnostics.Load(id)
	data, ok := val.(*inflightData)
	if !ok {
		data = &inflightData{}
	}
	data.Diagnostics = diag
	s.inflightDiagnostics.Store(id, data)
}

// AddRequest stores the method name, request, and response size in memory for later flushing.
// Called by the wrapper to capture the API method and its parameters.
func (s *Store) AddRequest(id string, method string, request any) {
	if id == "" {
		return
	}
	// Load existing inflight data or create new
	val, _ := s.inflightDiagnostics.Load(id)
	data, ok := val.(*inflightData)
	if !ok {
		data = &inflightData{}
	}
	data.Method = method
	data.Request = request
	s.inflightDiagnostics.Store(id, data)
}

func (s *Store) AddResponse(id string, response any, sizeBytes int64, responseTimeMs int64) {
	if id == "" {
		return
	}
	// Load existing inflight data or create new
	val, _ := s.inflightDiagnostics.Load(id)
	data, ok := val.(*inflightData)
	if !ok {
		data = &inflightData{}
	}
	data.Response = response
	data.ResponseSizeBytes = sizeBytes
	data.ResponseTimeMs = responseTimeMs
}

// Flush saves the in-memory diagnostics to the bucket and removes from memory.
// Called by the wrapper after the request completes.
func (s *Store) Flush(ctx context.Context, tenantID, id string) error {
	if id == "" || tenantID == "" {
		return nil
	}

	// Get and remove from in-memory store
	val, ok := s.inflightDiagnostics.LoadAndDelete(id)
	if !ok {
		level.Debug(s.logger).Log("msg", "no inflight diagnostics found", "id", id)
		return nil
	}

	data, ok := val.(*inflightData)
	if !ok || data == nil {
		return nil
	}

	basePath := storagePrefix + tenantID + "/" + id + "/"

	// Save metadata (including method name and response size)
	metadata := &storedMetadata{
		ID:                id,
		CreatedAt:         time.Now().UTC(),
		TenantID:          tenantID,
		ResponseTimeMs:    data.ResponseTimeMs,
		ResponseSizeBytes: data.ResponseSizeBytes,
		Method:            data.Method,
	}
	if err := s.saveJSON(ctx, basePath+"metadata.json", metadata); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	// Save request if provided (serialized as JSON)
	if data.Request != nil {
		if err := s.saveJSON(ctx, basePath+"request.json", data.Request); err != nil {
			return fmt.Errorf("failed to save request: %w", err)
		}
	}

	// Save response if provided
	if data.Response != nil {
		if err := s.saveJSON(ctx, basePath+"response.json", data.Response); err != nil {
			return fmt.Errorf("failed to save response: %w", err)
		}
	}

	// Save plan if provided and has content
	if data.Diagnostics != nil && data.Diagnostics.QueryPlan != nil && data.Diagnostics.QueryPlan.Root != nil {
		if err := s.saveJSON(ctx, basePath+"plan.json", data.Diagnostics.QueryPlan); err != nil {
			return fmt.Errorf("failed to save query plan: %w", err)
		}
	}

	// Save execution trace if provided
	if data.Diagnostics != nil && data.Diagnostics.ExecutionNode != nil {
		if err := s.saveJSON(ctx, basePath+"execution.json", data.Diagnostics.ExecutionNode); err != nil {
			return fmt.Errorf("failed to save execution trace: %w", err)
		}
	}

	level.Debug(s.logger).Log(
		"msg", "stored query diagnostics",
		"id", id,
		"tenant_id", tenantID,
		"method", data.Method,
		"response_time_ms", data.ResponseTimeMs,
	)

	return nil
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
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("invalid diagnostics ID: %s", err)
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
		ID:                metadata.ID,
		CreatedAt:         metadata.CreatedAt,
		TenantID:          metadata.TenantID,
		ResponseTimeMs:    metadata.ResponseTimeMs,
		ResponseSizeBytes: metadata.ResponseSizeBytes,
		Method:            metadata.Method,
	}

	// Read request as raw JSON (optional)
	if data, err := s.readRaw(ctx, basePath+"request.json"); err == nil {
		stored.Request = data
	} else if !s.bucket.IsObjNotFoundErr(err) {
		level.Warn(s.logger).Log("msg", "failed to read request", "id", id, "err", err)
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

func (s *Store) readRaw(ctx context.Context, path string) (json.RawMessage, error) {
	reader, err := s.bucket.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %w", err)
	}

	return data, nil
}

// Delete removes diagnostics by tenant and ID.
func (s *Store) Delete(ctx context.Context, tenantID, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("invalid diagnostics ID: %s", err)
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
	ID                string          `json:"id"`
	CreatedAt         time.Time       `json:"created_at"`
	Method            string          `json:"method"`
	ResponseTimeMs    int64           `json:"response_time_ms"`
	ResponseSizeBytes int64           `json:"response_size_bytes"`
	Request           json.RawMessage `json:"request,omitempty"`
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
			ID:                metadata.ID,
			CreatedAt:         metadata.CreatedAt,
			Method:            metadata.Method,
			ResponseTimeMs:    metadata.ResponseTimeMs,
			ResponseSizeBytes: metadata.ResponseSizeBytes,
		}

		// Try to read request payload
		requestPath := strings.TrimSuffix(name, "metadata.json") + "request.json"
		if data, err := s.readRaw(ctx, requestPath); err == nil {
			summary.Request = data
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

// Export creates a zip archive containing all files for a diagnostic.
func (s *Store) Export(ctx context.Context, tenantID, id string) ([]byte, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("invalid diagnostics ID: %s", err)
	}

	basePath := storagePrefix + tenantID + "/" + id + "/"
	files := []string{"metadata.json", "request.json", "response.json", "plan.json", "execution.json"}

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	for _, file := range files {
		data, err := s.readRaw(ctx, basePath+file)
		if err != nil {
			if s.bucket.IsObjNotFoundErr(err) {
				continue
			}
			return nil, fmt.Errorf("failed to read %s: %w", file, err)
		}

		w, err := zipWriter.Create(file)
		if err != nil {
			return nil, fmt.Errorf("failed to create zip entry for %s: %w", file, err)
		}
		if _, err := w.Write(data); err != nil {
			return nil, fmt.Errorf("failed to write %s to zip: %w", file, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip: %w", err)
	}

	return buf.Bytes(), nil
}

// Import extracts a zip archive and stores the diagnostic files.
// If newID is empty, generates a new UUID. Returns the ID used.
func (s *Store) Import(ctx context.Context, tenantID string, newID string, zipData []byte) (string, error) {
	if newID == "" {
		newID = generateUUID()
	} else if _, err := uuid.Parse(newID); err != nil {
		return "", fmt.Errorf("invalid diagnostics ID: %s", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return "", fmt.Errorf("failed to read zip: %w", err)
	}

	basePath := storagePrefix + tenantID + "/" + newID + "/"
	allowedFiles := map[string]bool{
		"metadata.json":  true,
		"request.json":   true,
		"response.json":  true,
		"plan.json":      true,
		"execution.json": true,
	}

	var hasMetadata bool
	for _, file := range zipReader.File {
		if !allowedFiles[file.Name] {
			continue
		}
		if file.Name == "metadata.json" {
			hasMetadata = true
		}

		rc, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("failed to open %s from zip: %w", file.Name, err)
		}

		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", fmt.Errorf("failed to read %s from zip: %w", file.Name, err)
		}

		// For metadata, update the ID and tenant to match the import
		if file.Name == "metadata.json" {
			var metadata storedMetadata
			if err := json.Unmarshal(data, &metadata); err != nil {
				return "", fmt.Errorf("failed to parse metadata: %w", err)
			}
			metadata.ID = newID
			metadata.TenantID = tenantID
			metadata.CreatedAt = time.Now().UTC()
			data, err = json.Marshal(metadata)
			if err != nil {
				return "", fmt.Errorf("failed to marshal updated metadata: %w", err)
			}
		}

		if err := s.bucket.Upload(ctx, basePath+file.Name, bytes.NewReader(data)); err != nil {
			return "", fmt.Errorf("failed to upload %s: %w", file.Name, err)
		}
	}

	if !hasMetadata {
		return "", fmt.Errorf("zip archive must contain metadata.json")
	}

	level.Info(s.logger).Log("msg", "imported diagnostics", "id", newID, "tenant_id", tenantID)
	return newID, nil
}

func generateUUID() string {
	return uuid.New().String()
}
