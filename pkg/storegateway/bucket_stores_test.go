package storegateway

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/validation"
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
	}, 0)
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

	p := expfmt.NewTextParser(model.UTF8Validation)
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

func TestBucketStores_CloseIdleBlocks(t *testing.T) {
	ctx := context.Background()

	bucketDir := filepath.Join(t.TempDir(), "bucket")
	tenantDir := filepath.Join(bucketDir, "tenant-1")
	require.NoError(t, os.MkdirAll(tenantDir, 0755))
	test.Copy(t, "../phlaredb/testdata", filepath.Join(tenantDir, "phlaredb"))

	bucket, err := filesystem.NewBucket(bucketDir)
	require.NoError(t, err)
	limits, err := validation.NewOverrides(*validation.MockDefaultLimits(), nil)
	require.NoError(t, err)
	reg := prometheus.NewRegistry()
	config := BucketStoreConfig{
		SyncDir:               filepath.Join(t.TempDir(), "sync-dir"),
		TenantSyncConcurrency: 1,
		MetaSyncConcurrency:   1,
	}
	stores, err := NewBucketStores(config, new(mockShardingStrategy), bucket, limits, log.NewNopLogger(), reg)
	require.NoError(t, err)
	require.NoError(t, stores.SyncBlocks(ctx))
	t.Cleanup(func() { assert.NoError(t, stores.closeBucketStore("tenant-1")) })

	store := stores.getStore("tenant-1")
	require.NotNil(t, store)
	require.Len(t, store.blockSet.blocks, 3)

	openBlocks := func(n int) string {
		return fmt.Sprintf(`
# HELP pyroscopedb_blocks_currently_open Number of blocks opened
# TYPE pyroscopedb_blocks_currently_open gauge
pyroscopedb_blocks_currently_open{tenant="tenant-1"} %d
`, n)
	}
	query := func(getBlocks phlaredb.BlockGetter) error {
		_, err := phlaredb.LabelNames(ctx, connect.NewRequest(&typesv1.LabelNamesRequest{
			Start: 0,
			End:   time.Now().UnixMilli(),
		}), getBlocks)
		return err
	}
	future := time.Now().Add(time.Hour)

	// Blocks used by an ongoing request are never closed.
	require.NoError(t, store.withBlocks(func(getBlocks phlaredb.BlockGetter) error {
		require.NoError(t, query(getBlocks))
		assertMetricsGathered(t, reg, true, []string{openBlocks(3)})
		stores.CloseIdleBlocks(future)
		assertMetricsGathered(t, reg, true, []string{openBlocks(3)})
		return query(getBlocks)
	}))

	// Released but used after the cutoff: kept open.
	stores.CloseIdleBlocks(time.Now().Add(-time.Hour))
	assertMetricsGathered(t, reg, true, []string{openBlocks(3)})

	// Idle and released: closed.
	stores.CloseIdleBlocks(future)
	assertMetricsGathered(t, reg, true, []string{openBlocks(0)})

	// The next request opens them again.
	require.NoError(t, store.withBlocks(query))
	assertMetricsGathered(t, reg, true, []string{openBlocks(3)})
}
