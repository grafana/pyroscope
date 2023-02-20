// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/distributor/ingestion_rate_strategy_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package distributor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/grafana/phlare/pkg/validation"
)

func TestIngestionRateStrategy(t *testing.T) {
	t.Run("rate limiter should share the limit across the number of distributors", func(t *testing.T) {
		// Init limits overrides
		overrides, err := validation.NewOverrides(validation.Limits{
			IngestionRateMB:      float64(1000),
			IngestionBurstSizeMB: 10000,
		}, nil)
		require.NoError(t, err)

		mockRing := newReadLifecyclerMock()
		mockRing.On("HealthyInstancesCount").Return(2)

		strategy := newGlobalRateStrategy(newIngestionRateStrategy(overrides), mockRing)
		assert.Equal(t, strategy.Limit("test"), float64(1000*1024*1024/2))
		assert.Equal(t, strategy.Burst("test"), 10000*1024*1024)
	})

	t.Run("infinite rate limiter should return unlimited settings", func(t *testing.T) {
		strategy := newInfiniteRateStrategy()

		assert.Equal(t, strategy.Limit("test"), float64(rate.Inf))
		assert.Equal(t, strategy.Burst("test"), 0)
	})
}

type readLifecyclerMock struct {
	mock.Mock
}

func newReadLifecyclerMock() *readLifecyclerMock {
	return &readLifecyclerMock{}
}

func (m *readLifecyclerMock) HealthyInstancesCount() int {
	args := m.Called()
	return args.Int(0)
}
