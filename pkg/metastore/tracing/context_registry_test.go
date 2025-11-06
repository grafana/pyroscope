package tracing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contextKey string

func TestContextRegistry_StoreAndRetrieve(t *testing.T) {
	r := newContextRegistry(1*time.Second, 5*time.Second, nil)
	defer r.Shutdown()

	ctx := context.WithValue(context.Background(), contextKey("key"), "value")
	r.Store("id-1", ctx)

	retrieved, found := r.Retrieve("id-1")
	require.True(t, found)
	assert.Equal(t, "value", retrieved.Value(contextKey("key")))
}

func TestContextRegistry_RetrieveNotFound(t *testing.T) {
	r := newContextRegistry(1*time.Second, 5*time.Second, nil)
	defer r.Shutdown()

	retrieved, found := r.Retrieve("nonexistent-id")
	require.False(t, found)
	assert.Equal(t, context.Background(), retrieved)
}

func TestContextRegistry_Delete(t *testing.T) {
	r := newContextRegistry(1*time.Second, 5*time.Second, nil)
	defer r.Shutdown()

	ctx := context.WithValue(context.Background(), contextKey("key"), "value")
	r.Store("id-1", ctx)

	_, found := r.Retrieve("id-1")
	require.True(t, found)

	r.Delete("id-1")

	_, found = r.Retrieve("id-1")
	require.False(t, found)
}

func TestContextRegistry_Cleanup(t *testing.T) {
	// Use short TTL for a faster test
	r := newContextRegistry(100*time.Millisecond, 200*time.Millisecond, nil)
	defer r.Shutdown()

	ctx := context.WithValue(context.Background(), contextKey("key"), "value")
	r.Store("id-1", ctx)

	_, found := r.Retrieve("id-1")
	require.True(t, found)
	assert.Equal(t, 1, r.Size())

	time.Sleep(400 * time.Millisecond)

	_, found = r.Retrieve("id-1")
	require.False(t, found)
	assert.Equal(t, 0, r.Size())
}

func TestContextRegistry_Size(t *testing.T) {
	r := newContextRegistry(1*time.Second, 5*time.Second, nil)
	defer r.Shutdown()

	assert.Equal(t, 0, r.Size())

	ctx := context.Background()
	r.Store("id-1", ctx)
	r.Store("id-2", ctx)
	r.Store("id-3", ctx)

	assert.Equal(t, 3, r.Size())

	r.Delete("id-2")
	assert.Equal(t, 2, r.Size())
}

func TestContextRegistry_ConcurrentAccess(t *testing.T) {
	r := newContextRegistry(1*time.Second, 5*time.Second, nil)
	defer r.Shutdown()

	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(id int) {
			ctx := context.WithValue(context.Background(), contextKey("id"), id)
			for j := 0; j < 100; j++ {
				r.Store(fmt.Sprintf("%d-%d", id, j), ctx)
				time.Sleep(1 * time.Millisecond)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				r.Retrieve(fmt.Sprintf("%d-%d", id, j))
				time.Sleep(1 * time.Millisecond)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	assert.True(t, r.Size() > 0)
}
