package ingester

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	phlaremodel "github.com/grafana/phlare/pkg/model"
)

type fakeLimits struct {
	maxLocalSeriesPerTenant  int
	maxGlobalSeriesPerTenant int
}

func (f *fakeLimits) MaxLocalSeriesPerTenant(userID string) int {
	return f.maxLocalSeriesPerTenant
}

func (f *fakeLimits) MaxGlobalSeriesPerTenant(userID string) int {
	return f.maxGlobalSeriesPerTenant
}

type fakeRingCount struct {
	healthyInstancesCount int
}

func (f *fakeRingCount) HealthyInstancesCount() int {
	return f.healthyInstancesCount
}

func TestOutOfOrder(t *testing.T) {
	limiter := NewLimiter("foo", &fakeLimits{}, &fakeRingCount{1}, 1)
	defer limiter.Stop()

	// First push should be allowed.
	err := limiter.AllowProfile(1, phlaremodel.LabelsFromStrings("foo", "bar"), 5)
	require.NoError(t, err)

	// different stream should be allowed.
	err = limiter.AllowProfile(2, phlaremodel.LabelsFromStrings("foo", "baz"), 1)
	require.NoError(t, err)

	err = limiter.AllowProfile(1, phlaremodel.LabelsFromStrings("foo", "baz"), 1)
	require.Error(t, err)
}

func TestGlobalMaxSeries(t *testing.T) {
	// 5 series per user, 2 ingesters, replication factor 3.
	// We should be able to push 7.5 series. (5 / 2 * 3 = 7.5)
	activeSeriesTimeout = 200 * time.Millisecond
	activeSeriesCleanup = 100 * time.Millisecond

	limiter := NewLimiter("foo", &fakeLimits{maxGlobalSeriesPerTenant: 5}, &fakeRingCount{2}, 3)
	defer limiter.Stop()

	for i := 0; i < 7; i++ {
		err := limiter.AllowProfile(model.Fingerprint(i), phlaremodel.LabelsFromStrings("i", fmt.Sprintf("%d", i)), 0)
		require.NoError(t, err)
	}

	err := limiter.AllowProfile(8, phlaremodel.LabelsFromStrings("i", "8"), 0)
	require.Error(t, err)

	// Wait for cleanup to happen.
	time.Sleep(400 * time.Millisecond)

	// Now we should be able to push 5 series.
	for i := 0; i < 5; i++ {
		err := limiter.AllowProfile(model.Fingerprint(i), phlaremodel.LabelsFromStrings("i", fmt.Sprintf("%d", i)), 0)
		require.NoError(t, err)
	}
}

func TestLocalLimit(t *testing.T) {
	activeSeriesTimeout = 200 * time.Millisecond
	activeSeriesCleanup = 100 * time.Millisecond

	limiter := NewLimiter("foo", &fakeLimits{maxGlobalSeriesPerTenant: 5, maxLocalSeriesPerTenant: 1}, &fakeRingCount{2}, 3)
	defer limiter.Stop()

	// local limit of 1 series should take precedence over global limit of 5 series.

	err := limiter.AllowProfile(1, phlaremodel.LabelsFromStrings("i", "1"), 0)
	require.NoError(t, err)

	err = limiter.AllowProfile(2, phlaremodel.LabelsFromStrings("i", "2"), 0)
	require.Error(t, err)
}
