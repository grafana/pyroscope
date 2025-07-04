package ingestlimits

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Mock ring for testing
type mockRing struct {
	instanceCount int
}

func (m *mockRing) InstancesCount() int {
	return m.instanceCount
}

func TestAllowRequest(t *testing.T) {
	sampler := NewSampler(&mockRing{instanceCount: 1})

	config := SamplingConfig{
		Period:      1 * time.Second,
		NumRequests: 1,
	}

	tenantID := "test-tenant"
	allowed := sampler.AllowRequest(tenantID, config)
	assert.True(t, allowed, "this request is brand new and should be allowed")

	allowed = sampler.AllowRequest(tenantID, config)
	assert.Falsef(t, allowed, "this request should be within a second of the first and not be allowed")
}

func TestTenantTrackerAllowRequest(t *testing.T) {
	testCases := []struct {
		name           string
		replicaCount   int
		windowDuration time.Duration
		maxRequests    int
		requestCount   int
		expectAllowed  int
	}{
		{
			name:           "Within limit",
			replicaCount:   2,
			windowDuration: 5 * time.Second,
			maxRequests:    5,
			requestCount:   3,
			expectAllowed:  3,
		},
		{
			name:           "Exceed limit",
			replicaCount:   2,
			windowDuration: 5 * time.Second,
			maxRequests:    3,
			requestCount:   5,
			expectAllowed:  3,
		},
		{
			name:           "Random probability",
			replicaCount:   100,
			windowDuration: 5 * time.Second,
			maxRequests:    5,
			requestCount:   100,
			expectAllowed:  5,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tracker := &tenantTracker{
				lastRequestTime:   time.Now(),
				remainingRequests: tc.maxRequests,
			}

			allowedCnt := 0
			for i := 0; i < tc.requestCount; i++ {
				if tracker.AllowRequest(tc.replicaCount, tc.windowDuration, tc.maxRequests) {
					allowedCnt++
				}
			}
			assert.LessOrEqualf(t, allowedCnt, tc.expectAllowed, "request %d should match expected")
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	sampler := NewSampler(&mockRing{instanceCount: 2})

	config := SamplingConfig{
		Period:      5 * time.Second,
		NumRequests: 10,
	}

	var wg sync.WaitGroup
	allowedCount := 0
	mu := sync.Mutex{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if sampler.AllowRequest("concurrent-tenant", config) {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	assert.LessOrEqual(t, allowedCount, 10, "Should not exceed max requests")
}
