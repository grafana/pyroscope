package phlaredb

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

func TestMultipleRegistrationMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m1 := newHeadMetrics(reg)
	m2 := newHeadMetrics(reg)

	m1.profilesCreated.WithLabelValues("test").Inc()
	m2.profilesCreated.WithLabelValues("test").Inc()

	// collect metrics and compare them
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP pyroscope_head_profiles_created_total Total number of profiles created in the head
# TYPE pyroscope_head_profiles_created_total counter
pyroscope_head_profiles_created_total{profile_name="test"} 2
`), "pyroscope_head_profiles_created_total"))
}

func TestHeadMetrics(t *testing.T) {
	head := newTestHead(t)
	require.NoError(t, head.Ingest(context.Background(), newProfileFoo(), uuid.New()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBar(), uuid.New()))
	require.NoError(t, head.Ingest(context.Background(), newProfileBaz(), uuid.New()))
	head.updateSymbolsMemUsage(new(symdb.MemoryStats))
	time.Sleep(time.Second)
	require.NoError(t, testutil.GatherAndCompare(head.reg,
		strings.NewReader(`
# HELP pyroscope_head_ingested_sample_values_total Number of sample values ingested into the head per profile type.
# TYPE pyroscope_head_ingested_sample_values_total counter
pyroscope_head_ingested_sample_values_total{profile_name=""} 3
# HELP pyroscope_head_profiles_created_total Total number of profiles created in the head
# TYPE pyroscope_head_profiles_created_total counter
pyroscope_head_profiles_created_total{profile_name=""} 2
# HELP pyroscope_head_received_sample_values_total Number of sample values received into the head per profile type.
# TYPE pyroscope_head_received_sample_values_total counter
pyroscope_head_received_sample_values_total{profile_name=""} 3

# HELP pyroscope_head_size_bytes Size of a particular in memory store within the head phlaredb block.
# TYPE pyroscope_head_size_bytes gauge
pyroscope_head_size_bytes{type="functions"} 96
pyroscope_head_size_bytes{type="locations"} 152
pyroscope_head_size_bytes{type="mappings"} 96
pyroscope_head_size_bytes{type="profiles"} 420
pyroscope_head_size_bytes{type="stacktraces"} 96
pyroscope_head_size_bytes{type="strings"} 66

`),
		"pyroscope_head_received_sample_values_total",
		"pyroscope_head_profiles_created_total",
		"pyroscope_head_ingested_sample_values_total",
		"pyroscope_head_size_bytes",
	))
}
