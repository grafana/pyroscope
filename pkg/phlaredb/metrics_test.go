package phlaredb

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestMultipleRegistrationMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m1 := newHeadMetrics(reg)
	m2 := newHeadMetrics(reg)

	m1.profilesCreated.WithLabelValues("test").Inc()
	m2.profilesCreated.WithLabelValues("test").Inc()

	// collect metrics and compare them
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP phlare_head_profiles_created_total Total number of profiles created in the head
# TYPE phlare_head_profiles_created_total counter
phlare_head_profiles_created_total{profile_name="test"} 2
`), "phlare_head_profiles_created_total"))
}
