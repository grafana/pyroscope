package storegateway

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/validation"
)

func TestBucketStores_BlockMetricsRegistration(t *testing.T) {
	ctx := context.Background()

	bucketDir := filepath.Join(t.TempDir(), "bucket")
	tenantDir := filepath.Join(bucketDir, "tenant-1")
	phlaredbDir := filepath.Join(tenantDir, "phlaredb")
	require.NoError(t, os.MkdirAll(tenantDir, 0755))
	test.Copy(t, "../phlaredb/testdata", phlaredbDir)

	bucket, err := filesystem.NewBucket(bucketDir)
	require.NoError(t, err)

	sharding := new(mockShardingStrategy)
	mockLimits := validation.MockDefaultLimits()
	limits, err := validation.NewOverrides(*mockLimits, nil)
	require.NoError(t, err)
	logger := log.NewNopLogger()
	reg := prometheus.NewRegistry()
	config := BucketStoreConfig{
		SyncDir:               filepath.Join(t.TempDir(), "sync-dir"),
		TenantSyncConcurrency: 1,
		MetaSyncConcurrency:   1,
	}

	stores, err := NewBucketStores(config, sharding, bucket, limits, logger, reg)
	require.NoError(t, err)
	require.NoError(t, stores.SyncBlocks(ctx))

	userStore := stores.getStore("tenant-1")
	require.NotNil(t, userStore)
	require.Len(t, userStore.blockSet.blocks, 3)
	r, err := userStore.blockSet.blocks[0].SelectMergeByStacktraces(ctx, &ingestv1.SelectProfilesRequest{
		LabelSelector: "{}",
		Type: &typesv1.ProfileType{
			Name:       "process_cpu",
			SampleType: "cpu",
			SampleUnit: "nanoseconds",
			PeriodType: "cpu",
			PeriodUnit: "nanoseconds",
		},
		Start: 0,
		End:   time.Now().UnixMilli(),
	})
	require.NoError(t, err)
	require.NotNil(t, r)

	tenantBlockMetrics := []string{`
# HELP pyroscopedb_block_profile_table_accesses_total Number of times a profile table was accessed
# TYPE pyroscopedb_block_profile_table_accesses_total counter
pyroscopedb_block_profile_table_accesses_total{table="profiles.parquet",tenant="tenant-1"} 1
`, `
# HELP pyroscopedb_page_reads_total Total number of pages read while querying
# TYPE pyroscopedb_page_reads_total counter
pyroscopedb_page_reads_total{column="Samples.list.element.StacktraceID",table="profiles",tenant="tenant-1"} 2
pyroscopedb_page_reads_total{column="Samples.list.element.Value",table="profiles",tenant="tenant-1"} 2
pyroscopedb_page_reads_total{column="SeriesIndex",table="profiles",tenant="tenant-1"} 2
pyroscopedb_page_reads_total{column="StacktracePartition",table="profiles",tenant="tenant-1"} 2
pyroscopedb_page_reads_total{column="TimeNanos",table="profiles",tenant="tenant-1"} 2
`}
	assertMetricsGathered(t, reg, true, tenantBlockMetrics)

	assert.NoError(t, stores.closeBucketStore("tenant-1"))
	assertMetricsGathered(t, reg, false, tenantBlockMetrics)
}

type mockShardingStrategy struct{}

func (m *mockShardingStrategy) FilterUsers(_ context.Context, userIDs []string) ([]string, error) {
	return userIDs, nil
}

func (m *mockShardingStrategy) FilterBlocks(_ context.Context, _ string, _ map[ulid.ULID]*block.Meta, _ map[ulid.ULID]struct{}, _ block.GaugeVec) error {
	return nil
}

func assertMetricsGathered(tb testing.TB, reg prometheus.Gatherer, gathered bool, expected []string) {
	m, err := reg.Gather()
	require.NoError(tb, err)
	actual := make([]string, 0, len(m))
	var buf bytes.Buffer
	for _, metric := range m {
		buf.Reset()
		_, err = expfmt.MetricFamilyToText(&buf, metric)
		require.NoError(tb, err)
		if s := strings.TrimSpace(buf.String()); len(s) > 0 {
			actual = append(actual, s)
		}
	}

	var p expfmt.TextParser
	for _, e := range expected {
		_, err = p.TextToMetricFamilies(strings.NewReader(e))
		require.NoError(tb, err, "expected metric is not valid:\n%s", e)
		e = strings.TrimSpace(e)
		var found bool
		for _, g := range actual {
			if found = g == e; found {
				break
			}
		}
		if gathered {
			assert.True(tb, found, "expected metric not found:\n%s", e)
		} else {
			assert.False(tb, found, "found unexpected metric:\n%s", e)
		}
	}

	if tb.Failed() {
		tb.Logf("gathered metrics:\n%s\n", strings.Join(actual, "\n\n"))
	}
}
