package ingestlimits

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/grafana/dskit/services"
)

type tenantTracker struct {
	mu                sync.Mutex
	lastRequestTime   time.Time
	remainingRequests int
}

// Sampler provides a very simple time-based probabilistic sampling,
// intended to be used when a tenant limit has been reached.
//
// The sampler will allow a number of requests in a time interval.
// Once the interval is over, the number of allowed requests resets.
//
// We introduce a probability function for a request to be allowed defined as 1 / num_replicas,
// to account for the size of the cluster and because tracking is done in memory.
type Sampler struct {
	*services.BasicService

	mu      sync.RWMutex
	tenants map[string]*tenantTracker

	// needed for adjusting the probability function with the number of replicas
	instanceCountProvider InstanceCountProvider

	// cleanup of the tenants map to prevent build-up
	cleanupInterval time.Duration
	maxAge          time.Duration
	closeOnce       sync.Once
	stop            chan struct{}
	done            chan struct{}
}

type InstanceCountProvider interface {
	InstancesCount() int
}

func NewSampler(instanceCount InstanceCountProvider) *Sampler {
	s := &Sampler{
		tenants:               make(map[string]*tenantTracker),
		instanceCountProvider: instanceCount,
		cleanupInterval:       1 * time.Hour,
		maxAge:                24 * time.Hour,
		stop:                  make(chan struct{}),
		done:                  make(chan struct{}),
	}
	s.BasicService = services.NewBasicService(
		s.starting,
		s.running,
		s.stopping,
	)

	return s
}

func (s *Sampler) starting(_ context.Context) error { return nil }

func (s *Sampler) stopping(_ error) error {
	s.closeOnce.Do(func() {
		close(s.stop)
		<-s.done
	})
	return nil
}

func (s *Sampler) running(ctx context.Context) error {
	t := time.NewTicker(s.cleanupInterval)
	defer func() {
		t.Stop()
		close(s.done)
	}()
	for {
		select {
		case <-t.C:
			s.removeStaleTenants()
		case <-s.stop:
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

func (s *Sampler) AllowRequest(tenantID string, config SamplingConfig) bool {
	s.mu.Lock()
	tracker, exists := s.tenants[tenantID]
	if !exists {
		tracker = &tenantTracker{
			lastRequestTime:   time.Now(),
			remainingRequests: config.NumRequests,
		}
		s.tenants[tenantID] = tracker
	}
	s.mu.Unlock()

	return tracker.AllowRequest(s.instanceCountProvider.InstancesCount(), config.Period, config.NumRequests)
}

func (b *tenantTracker) AllowRequest(replicaCount int, windowDuration time.Duration, maxRequests int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// reset tracking data if enough time has passed
	if now.Sub(b.lastRequestTime) >= windowDuration {
		b.lastRequestTime = now
		b.remainingRequests = maxRequests
	}

	if b.remainingRequests > 0 {
		// random chance of allowing request, adjusting for the number of replicas
		shouldAllow := rand.Float64() < float64(maxRequests)/float64(replicaCount)

		if shouldAllow {
			b.remainingRequests--
			return true
		}
	}

	return false
}

func (s *Sampler) removeStaleTenants() {
	s.mu.Lock()
	cutoff := time.Now().Add(-s.maxAge)
	for tenantID, tracker := range s.tenants {
		if tracker.lastRequestTime.Before(cutoff) {
			delete(s.tenants, tenantID)
		}
	}
	s.mu.Unlock()
}
