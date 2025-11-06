package block_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/metrics"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore/testutil"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetrics"
)

func Test_CompactBlocks(t *testing.T) {
	ctx := context.Background()
	bucket, _ := testutil.NewFilesystemBucket(t, ctx, "testdata")

	var resp metastorev1.GetBlockMetadataResponse
	raw, err := os.ReadFile("testdata/block-metas.json")
	require.NoError(t, err)
	err = protojson.Unmarshal(raw, &resp)
	require.NoError(t, err)

	dst, tempdir := testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	compactedBlocks, err := block.Compact(ctx, resp.Blocks, bucket,
		block.WithCompactionDestination(dst),
		block.WithCompactionTempDir(tempdir),
		block.WithCompactionObjectOptions(
			block.WithObjectDownload(filepath.Join(tempdir, "source")),
			block.WithObjectMaxSizeLoadInMemory(0)), // Force download.
	)

	require.NoError(t, err)
	require.Len(t, compactedBlocks, 1)
	require.NotZero(t, compactedBlocks[0].Size)
	require.Len(t, compactedBlocks[0].Datasets, 4)

	compactedJson, err := json.MarshalIndent(compactedBlocks, "", "  ")
	require.NoError(t, err)
	expectedJson, err := os.ReadFile("testdata/compacted.golden")
	require.NoError(t, err)
	assert.Equal(t, string(expectedJson), string(compactedJson))

	t.Run("Compact compacted blocks", func(t *testing.T) {
		compactedBlocks, err = block.Compact(ctx, compactedBlocks, dst,
			block.WithCompactionDestination(dst),
			block.WithCompactionTempDir(tempdir),
			block.WithCompactionObjectOptions(
				block.WithObjectDownload(filepath.Join(tempdir, "source")),
				block.WithObjectMaxSizeLoadInMemory(0)), // Force download.
		)

		require.NoError(t, err)
		require.Len(t, compactedBlocks, 1)
		require.NotZero(t, compactedBlocks[0].Size)
		require.Len(t, compactedBlocks[0].Datasets, 4)
	})
}

func Test_CompactBlocks_recordingRules(t *testing.T) {
	ctx := context.Background()
	bucket, _ := testutil.NewFilesystemBucket(t, ctx, "testdata")

	var resp metastorev1.GetBlockMetadataResponse
	raw, err := os.ReadFile("testdata/block-metas.json")
	require.NoError(t, err)
	err = protojson.Unmarshal(raw, &resp)
	require.NoError(t, err)

	exporter := &stringExporter{}
	ruler := new(mockmetrics.MockRuler)
	ruler.On("RecordingRules", mock.Anything).Return([]*phlaremodel.RecordingRule{
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "goroutine:goroutine:count:goroutine:count"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_goroutines_total_count"}),
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:alloc_objects:count:space:bytes"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_alloc_total_count"}),
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:alloc_space:bytes:space:bytes"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_alloc_total_bytes"}),
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:inuse_objects:count:space:bytes"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_inuse_total_count"}),
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:inuse_space:bytes:space:bytes"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_inuse_total_bytes"}),
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_cpu_usage_total_nanoseconds"}),
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "process_cpu:samples:count:cpu:nanoseconds"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_cpu_usage_total_samples"}),
		},
		// functions
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "goroutine:goroutine:count:goroutine:count"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_goroutines_function_total_servehttp_count"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:alloc_objects:count:space:bytes"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_alloc_function_total_servehttp_count"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:alloc_space:bytes:space:bytes"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_alloc_function_total_servehttp_bytes"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:inuse_objects:count:space:bytes"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_inuse_function_total_servehttp_count"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:inuse_space:bytes:space:bytes"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_inuse_function_total_servehttp_bytes"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "process_cpu:cpu:nanoseconds:cpu:nanoseconds"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_cpu_usage_function_total_servehttp_nanoseconds"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "process_cpu:samples:count:cpu:nanoseconds"),
			},
			GroupBy:        []string{"service_name"},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_cpu_usage_function_total_servehttp_samples"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
	})
	sampleObserver := metrics.NewSampleObserver(0, exporter, ruler, labels.EmptyLabels())

	compactedBlocks, err := block.Compact(ctx, resp.Blocks, bucket,
		block.WithSampleObserver(sampleObserver),
	)
	// Close observer to flush export
	sampleObserver.Close()

	require.NoError(t, err)
	require.Len(t, compactedBlocks, 1)
	require.NotZero(t, compactedBlocks[0].Size)
	require.Len(t, compactedBlocks[0].Datasets, 4)

	expectedMetrics, err := os.ReadFile("testdata/profiles_recorded.txt")
	require.NoError(t, err)
	expectedMetricsArray := strings.Split(string(expectedMetrics), "\n")
	sort.Strings(expectedMetricsArray)
	actualMetricsArray := strings.Split(exporter.String(), "\n")
	sort.Strings(actualMetricsArray)
	assert.Equal(t, expectedMetricsArray, actualMetricsArray)

	compactedJson, err := json.MarshalIndent(compactedBlocks, "", "  ")
	require.NoError(t, err)
	expectedJson, err := os.ReadFile("testdata/compacted.golden")
	require.NoError(t, err)
	assert.Equal(t, string(expectedJson), string(compactedJson))
}

func Test_CompactBlocks_recordingRules_shadowedSymbols(t *testing.T) {
	ctx := context.Background()
	bucket, _ := testutil.NewFilesystemBucket(t, ctx, "testdata")

	var resp metastorev1.GetBlockMetadataResponse
	raw, err := os.ReadFile("testdata/block-metas.json")
	require.NoError(t, err)
	err = protojson.Unmarshal(raw, &resp)
	require.NoError(t, err)

	exporter := &stringExporter{}
	ruler := new(mockmetrics.MockRuler)
	ruler.On("RecordingRules", mock.Anything).Return([]*phlaremodel.RecordingRule{
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "pyroscope-test/ingester"),
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:alloc_space:bytes:space:bytes"),
			},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_alloc_total_pyroscope_ingester_Push_bytes"}),
			FunctionName:   "github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "pyroscope-test/ingester"),
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:inuse_space:bytes:space:bytes"),
			},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_inuse_total_pyroscope_ingester_Push_bytes"}),
			FunctionName:   "github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "pyroscope-test/ingester"),
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "process_cpu:samples:count:cpu:nanoseconds"),
			},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_cpu_usage_total_pyroscope_ingester_Push_samples"}),
			FunctionName:   "github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "pyroscope-test/ingester"),
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:alloc_space:bytes:space:bytes"),
			},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_alloc_total_pyroscope_ingester_Serve_bytes"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "pyroscope-test/ingester"),
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:inuse_space:bytes:space:bytes"),
			},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_inuse_total_pyroscope_ingester_Serve_bytes"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "pyroscope-test/ingester"),
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "process_cpu:samples:count:cpu:nanoseconds"),
			},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_cpu_usage_total_pyroscope_ingester_Serve_samples"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "pyroscope-test/query-frontend"),
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:inuse_space:bytes:space:bytes"),
			},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_inuse_total_query_Serve_bytes"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
		{
			Matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "service_name", "pyroscope-test/query-frontend"),
				labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:alloc_space:bytes:space:bytes"),
			},
			ExternalLabels: labels.New(labels.Label{Name: "__name__", Value: "profiles_recorded_mem_alloc_total_query_Serve_bytes"}),
			FunctionName:   "net/http.HandlerFunc.ServeHTTP",
		},
	})
	sampleObserver := metrics.NewSampleObserver(0, exporter, ruler, labels.EmptyLabels())

	compactedBlocks, err := block.Compact(ctx, resp.Blocks, bucket,
		block.WithSampleObserver(sampleObserver),
	)
	// Close observer to flush export
	sampleObserver.Close()

	require.NoError(t, err)
	require.Len(t, compactedBlocks, 1)
	require.NotZero(t, compactedBlocks[0].Size)
	require.Len(t, compactedBlocks[0].Datasets, 4)

	expectedMetrics, err := os.ReadFile("testdata/profiles_recorded_shadowed.txt")
	require.NoError(t, err)
	expectedMetricsArray := strings.Split(string(expectedMetrics), "\n")
	sort.Strings(expectedMetricsArray)
	actualMetricsArray := strings.Split(exporter.String(), "\n")
	sort.Strings(actualMetricsArray)
	assert.Equal(t, expectedMetricsArray, actualMetricsArray)

	compactedJson, err := json.MarshalIndent(compactedBlocks, "", "  ")
	require.NoError(t, err)
	expectedJson, err := os.ReadFile("testdata/compacted.golden")
	require.NoError(t, err)
	assert.Equal(t, string(expectedJson), string(compactedJson))
}

type stringExporter struct {
	output string
}

func (e *stringExporter) Send(tenant string, series []prompb.TimeSeries) error {
	for _, s := range series {
		e.output += fmt.Sprintf("%s: %s\n", tenant, s.String())
	}
	return nil
}

func (*stringExporter) Flush() {}

func (e *stringExporter) String() string {
	return e.output
}
