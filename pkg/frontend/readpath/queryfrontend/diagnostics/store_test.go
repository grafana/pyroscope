package diagnostics

import (
	"context"
	"encoding/json"
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

func TestStore_Get(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	type testRequest struct {
		Query       string `json:"query"`
		LabelFilter string `json:"label_filter"`
	}

	request := &testRequest{
		Query:       "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		LabelFilter: `{service="test"}`,
	}

	id := generateUUID()
	store.AddRequest(id, "SelectMergeStacktraces", request)
	err := store.Flush(ctx, "tenant-1", id)
	require.NoError(t, err)

	stored, err := store.Get(ctx, "tenant-1", id)
	require.NoError(t, err)
	require.Equal(t, id, stored.ID)
	require.Equal(t, "tenant-1", stored.TenantID)
	require.Equal(t, "SelectMergeStacktraces", stored.Method)
	require.NotNil(t, stored.Request)

	var storedRequest testRequest
	require.NoError(t, json.Unmarshal(stored.Request, &storedRequest))
	require.Equal(t, request.LabelFilter, storedRequest.LabelFilter)
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
		"0000000000000000000000000000000g", // invalid hex char
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

	id := generateUUID()
	store.AddRequest(id, "ProfileTypes", nil)
	err := store.Flush(ctx, "tenant-1", id)
	require.NoError(t, err)

	_, err = store.Get(ctx, "tenant-1", id)
	require.NoError(t, err)

	err = store.Delete(ctx, "tenant-1", id)
	require.NoError(t, err)

	_, err = store.Get(ctx, "tenant-1", id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStore_Cleanup(t *testing.T) {
	ctx := context.Background()
	bucket := objstore.NewInMemBucket()
	store := NewStore(log.NewNopLogger(), bucket, WithTTL(1*time.Millisecond))

	// Save some diagnostics
	id1 := generateUUID()
	store.AddRequest(id1, "ProfileTypes", nil)
	err := store.Flush(ctx, "tenant-1", id1)
	require.NoError(t, err)

	id2 := generateUUID()
	store.AddRequest(id2, "ProfileTypes", nil)
	err = store.Flush(ctx, "tenant-2", id2)
	require.NoError(t, err)

	require.NoError(t, err)

	// Verify both exist
	_, err = store.Get(ctx, "tenant-1", id1)
	require.NoError(t, err)
	_, err = store.Get(ctx, "tenant-2", id2)
	require.NoError(t, err)

	// Wait for TTL to expire
	time.Sleep(10 * time.Millisecond)

	deleted, err := store.Cleanup(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, deleted)

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
	id := generateUUID()
	store.AddRequest(id, "ProfileTypes", nil)
	err := store.Flush(ctx, "tenant-1", id)
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

	type testRequest struct {
		Query       string `json:"query"`
		StartTime   int64  `json:"start_time"`
		EndTime     int64  `json:"end_time"`
		LabelFilter string `json:"label_filter"`
	}

	request := &testRequest{
		Query:       "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		StartTime:   1000,
		EndTime:     2000,
		LabelFilter: `{service="test"}`,
	}

	diag := &queryv1.Diagnostics{
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

	store.AddRequest(id, "SelectMergeStacktraces", request)
	store.Add(id, diag)
	err := store.Flush(ctx, "tenant-1", id)
	require.NoError(t, err)

	stored, err := store.Get(ctx, "tenant-1", id)
	require.NoError(t, err)
	assert.Equal(t, id, stored.ID)
	assert.Equal(t, "SelectMergeStacktraces", stored.Method)
	assert.NotNil(t, stored.Request)

	var storedRequest testRequest
	require.NoError(t, json.Unmarshal(stored.Request, &storedRequest))
	assert.Equal(t, int64(1000), storedRequest.StartTime)
	assert.NotNil(t, stored.Plan)
	assert.NotNil(t, stored.Execution)
}
