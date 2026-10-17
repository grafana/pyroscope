package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
)

func TestCompactionStateCache(t *testing.T) {
	now := time.Now()
	state := new(metastorev1.GetCompactionStateResponse)
	c := compactionStateCache{ttl: 5 * time.Second}

	_, _, ok := c.get("-", now)
	assert.False(t, ok)

	c.put("-", state, now)
	got, fetched, ok := c.get("-", now.Add(time.Second))
	require.True(t, ok)
	assert.Same(t, state, got)
	assert.Equal(t, now, fetched)

	// The entry expires exactly at the TTL.
	_, _, ok = c.get("-", now.Add(5*time.Second))
	assert.False(t, ok)
}

func TestCompactionStateCache_disabled(t *testing.T) {
	now := time.Now()
	var c compactionStateCache // the zero value caches nothing
	c.put("-", new(metastorev1.GetCompactionStateResponse), now)
	_, _, ok := c.get("-", now)
	assert.False(t, ok)
}

// The overview and the segment drill-down are distinct views, and the segment
// drill-down is keyed by an empty tenant: they must not collide.
func TestCompactionStateCacheKey(t *testing.T) {
	assert.NotEqual(t,
		compactionStateCacheKey(false, ""),
		compactionStateCacheKey(true, ""))
	assert.NotEqual(t,
		compactionStateCacheKey(true, "tenant-a"),
		compactionStateCacheKey(true, "tenant-b"))
	assert.Equal(t,
		compactionStateCacheKey(true, "tenant-a"),
		compactionStateCacheKey(true, "tenant-a"))
	// Keys are unambiguous for tenants that contain the separator.
	assert.NotEqual(t,
		compactionStateCacheKey(true, "a:b"),
		compactionStateCacheKey(true, "a:b:"))
}

func TestCompactionStateCache_eviction(t *testing.T) {
	now := time.Now()
	c := compactionStateCache{ttl: time.Minute}
	for i := 0; i < compactionStateCacheSize+2; i++ {
		c.put(compactionStateCacheKey(true, string(rune('a'+i))), new(metastorev1.GetCompactionStateResponse),
			now.Add(time.Duration(i)*time.Millisecond))
	}
	assert.LessOrEqual(t, len(c.entries), compactionStateCacheSize)
	// The oldest entries are the ones dropped.
	_, _, ok := c.get(compactionStateCacheKey(true, "a"), now)
	assert.False(t, ok)
	_, _, ok = c.get(compactionStateCacheKey(true, string(rune('a'+compactionStateCacheSize+1))), now)
	assert.True(t, ok)
}

// A reload must not repeat the scan, and the page must say the state is not
// freshly read.
func TestCompactionHandler_cached(t *testing.T) {
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(testCompactionState(time.Now()), nil).
		Once() // exactly one read for two requests

	a := &Admin{compactionClient: client}
	a.compactionCache.ttl = time.Minute

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metastore-compaction", nil))
		require.Equal(t, http.StatusOK, w.Code)
	}
}

// Distinct views are cached separately: walking from the overview into a
// tenant must still read the tenant state.
func TestCompactionHandler_cachedPerView(t *testing.T) {
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(testCompactionState(time.Now()), nil).
		Twice()

	a := &Admin{compactionClient: client}
	a.compactionCache.ttl = time.Minute

	for _, url := range []string{
		"/metastore-compaction",
		"/metastore-compaction?tenant=tenant-a",
		"/metastore-compaction", // back to the overview, served from the cache
	} {
		w := httptest.NewRecorder()
		a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		require.Equal(t, http.StatusOK, w.Code)
	}
}

// A failed read is not cached: the next request must try again.
func TestCompactionHandler_errorNotCached(t *testing.T) {
	client := mockmetastorev1.NewMockCompactionServiceClient(t)
	client.EXPECT().
		GetCompactionState(mock.Anything, mock.Anything).
		Return(nil, assert.AnError).
		Twice()

	a := &Admin{compactionClient: client}
	a.compactionCache.ttl = time.Minute

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		a.CompactionHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metastore-compaction", nil))
		assert.NotEqual(t, http.StatusOK, w.Code)
	}
}

func TestFormatStateAge(t *testing.T) {
	now := time.Now()
	// A state read for this request is not reported as stale.
	assert.Empty(t, formatStateAge(time.Time{}, now))
	assert.Empty(t, formatStateAge(now, now))
	assert.Empty(t, formatStateAge(now.Add(-500*time.Millisecond), now))
	assert.Equal(t, "3s", formatStateAge(now.Add(-3*time.Second), now))
}
