package diagnostics

import (
	"context"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	bucket := objstore.NewInMemBucket()
	return NewStore(log.NewNopLogger(), bucket)
}

func TestStore_SaveDirectAndGet(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	request := &queryv1.QueryRequest{
		StartTime:     1000,
		EndTime:       2000,
		LabelSelector: `{service="test"}`,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_PPROF,
		}},
	}

	plan := &queryv1.QueryPlan{
		Root: &queryv1.QueryNode{
			Type: queryv1.QueryNode_READ,
		},
	}

	execution := &queryv1.ExecutionNode{
		Type:        queryv1.QueryNode_READ,
		Executor:    "test-host",
		StartTimeNs: 1000000000,
		EndTimeNs:   1500000000,
		Stats: &queryv1.ExecutionStats{
			BlocksRead:        5,
			DatasetsProcessed: 10,
		},
	}

	// Save
	id, err := store.SaveDirect(ctx, "tenant-1", 500, request, plan, execution)
	require.NoError(t, err)
	assert.Len(t, id, 32) // hex-encoded 16 bytes UUID

	// Get
	stored, err := store.Get(ctx, "tenant-1", id)
	require.NoError(t, err)
	assert.Equal(t, id, stored.ID)
	assert.Equal(t, "tenant-1", stored.TenantID)
	assert.Equal(t, int64(500), stored.ResponseTimeMs)
	assert.NotNil(t, stored.Request)
	assert.Equal(t, request.LabelSelector, stored.Request.LabelSelector)
	assert.NotNil(t, stored.Plan)
	assert.Equal(t, queryv1.QueryNode_READ, stored.Plan.Root.Type)
	assert.NotNil(t, stored.Execution)
	assert.Equal(t, "test-host", stored.Execution.Executor)
	assert.Equal(t, int64(5), stored.Execution.Stats.BlocksRead)
}

func TestStore_GetNotFound(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	_, err := store.Get(ctx, "tenant-1", "00000000000000000000000000000000")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStore_GetInvalidID(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	tests := []string{
		"",
		"short",
		"0000000000000000000000000000000g",     // invalid hex char
		"00000000-0000-0000-0000-000000000000", // UUID format not allowed (dashes)
	}

	for _, id := range tests {
		t.Run(id, func(t *testing.T) {
			_, err := store.Get(ctx, "tenant-1", id)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid")
		})
	}
}

func TestStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// Save first
	id, err := store.SaveDirect(ctx, "tenant-1", 100, nil, nil, nil)
	require.NoError(t, err)

	// Verify it exists
	_, err = store.Get(ctx, "tenant-1", id)
	require.NoError(t, err)

	// Delete
	err = store.Delete(ctx, "tenant-1", id)
	require.NoError(t, err)

	// Verify it's gone
	_, err = store.Get(ctx, "tenant-1", id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		uuid     string
		expected bool
	}{
		{"00000000000000000000000000000000", true},
		{"abcdef0123456789abcdef0123456789", true},
		{"", false},
		{"short", false},
		{"0000000000000000000000000000000", false},   // 31 chars
		{"000000000000000000000000000000000", false}, // 33 chars
		{"0000000000000000000000000000000g", false},  // invalid char
		{"0000000000000000000000000000000G", false},  // uppercase not allowed
	}

	for _, tt := range tests {
		t.Run(tt.uuid, func(t *testing.T) {
			assert.Equal(t, tt.expected, isValidUUID(tt.uuid))
		})
	}
}

func TestGenerateUUID(t *testing.T) {
	uuid1 := generateUUID()
	uuid2 := generateUUID()

	assert.Len(t, uuid1, 32)
	assert.Len(t, uuid2, 32)
	assert.NotEqual(t, uuid1, uuid2)
	assert.True(t, isValidUUID(uuid1))
	assert.True(t, isValidUUID(uuid2))
}

func TestStore_Cleanup(t *testing.T) {
	ctx := context.Background()
	bucket := objstore.NewInMemBucket()
	store := NewStore(log.NewNopLogger(), bucket)

	// Override TTL to a very short duration for testing
	store.ttl = 1 * time.Millisecond

	// Save some diagnostics
	id1, err := store.SaveDirect(ctx, "tenant-1", 100, nil, nil, nil)
	require.NoError(t, err)

	id2, err := store.SaveDirect(ctx, "tenant-2", 200, nil, nil, nil)
	require.NoError(t, err)

	// Verify both exist
	_, err = store.Get(ctx, "tenant-1", id1)
	require.NoError(t, err)
	_, err = store.Get(ctx, "tenant-2", id2)
	require.NoError(t, err)

	// Wait for TTL to expire
	time.Sleep(10 * time.Millisecond)

	// Run cleanup
	deleted, err := store.Cleanup(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)

	// Verify both are gone
	_, err = store.Get(ctx, "tenant-1", id1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	_, err = store.Get(ctx, "tenant-2", id2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStore_CleanupPreservesRecent(t *testing.T) {
	ctx := context.Background()
	bucket := objstore.NewInMemBucket()
	store := NewStore(log.NewNopLogger(), bucket)

	// Use default TTL (7 days) - recent items should not be cleaned up
	// Save some diagnostics
	id, err := store.SaveDirect(ctx, "tenant-1", 100, nil, nil, nil)
	require.NoError(t, err)

	// Run cleanup immediately
	deleted, err := store.Cleanup(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, deleted)

	// Verify it still exists
	_, err = store.Get(ctx, "tenant-1", id)
	require.NoError(t, err)
}

func TestStore_AddAndFlush(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	diag := &queryv1.Diagnostics{
		QueryRequest: &queryv1.QueryRequest{
			StartTime:     1000,
			EndTime:       2000,
			LabelSelector: `{service="test"}`,
		},
		QueryPlan: &queryv1.QueryPlan{
			Root: &queryv1.QueryNode{
				Type: queryv1.QueryNode_READ,
			},
		},
		ExecutionNode: &queryv1.ExecutionNode{
			Type:     queryv1.QueryNode_READ,
			Executor: "test-host",
		},
	}

	id := "abcdef0123456789abcdef0123456789"
	store.Add(id, diag)

	// Flush
	err := store.Flush(ctx, "tenant-1", id, 500)
	require.NoError(t, err)

	// Get and verify
	stored, err := store.Get(ctx, "tenant-1", id)
	require.NoError(t, err)
	assert.Equal(t, id, stored.ID)
	assert.Equal(t, int64(500), stored.ResponseTimeMs)
	assert.NotNil(t, stored.Request)
	assert.Equal(t, int64(1000), stored.Request.StartTime)
	assert.NotNil(t, stored.Plan)
	assert.NotNil(t, stored.Execution)
}
