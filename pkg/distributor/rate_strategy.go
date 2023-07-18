// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/distributor/ingestion_rate_strategy.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package distributor

import (
	"golang.org/x/time/rate"

	"github.com/grafana/dskit/limiter"
)

// ReadLifecycler represents the read interface to the lifecycler.
type ReadLifecycler interface {
	HealthyInstancesCount() int
}

type globalStrategy struct {
	baseStrategy limiter.RateLimiterStrategy
	ring         ReadLifecycler
}

// newGlobalRateStrategy represents a ingestion rate limiting strategy that enforces the rate
// limiting globally, configuring a per-distributor local rate limiter as "ingestion_rate / N",
// where N is the number of distributor replicas (it's automatically adjusted if the
// number of replicas change).
//
// The global strategy requires the distributors to form their own ring, which
// is used to keep track of the current number of healthy distributor replicas.
func newGlobalRateStrategy(baseStrategy limiter.RateLimiterStrategy, ring ReadLifecycler) limiter.RateLimiterStrategy {
	return &globalStrategy{
		baseStrategy: baseStrategy,
		ring:         ring,
	}
}

func (s *globalStrategy) Limit(tenantID string) float64 {
	numDistributors := s.ring.HealthyInstancesCount()

	limit := s.baseStrategy.Limit(tenantID)

	if numDistributors == 0 || limit == float64(rate.Inf) {
		return limit
	}
	return limit / float64(numDistributors)
}

func (s *globalStrategy) Burst(tenantID string) int {
	// The meaning of burst doesn't change for the global strategy, in order
	// to keep it easier to understand for users / operators.
	return s.baseStrategy.Burst(tenantID)
}

type ingestionRateStrategy struct {
	limits Limits
}

func newIngestionRateStrategy(limits Limits) limiter.RateLimiterStrategy {
	return &ingestionRateStrategy{
		limits: limits,
	}
}

func (s *ingestionRateStrategy) Limit(tenantID string) float64 {
	return s.limits.IngestionRateBytes(tenantID)
}

func (s *ingestionRateStrategy) Burst(tenantID string) int {
	return s.limits.IngestionBurstSizeBytes(tenantID)
}

type infiniteStrategy struct{}

func newInfiniteRateStrategy() limiter.RateLimiterStrategy {
	return &infiniteStrategy{}
}

func (s *infiniteStrategy) Limit(tenantID string) float64 {
	return float64(rate.Inf)
}

func (s *infiniteStrategy) Burst(tenantID string) int {
	// Burst is ignored when limit = rate.Inf
	return 0
}
