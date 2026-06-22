package block_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/grafana/pyroscope/v2/pkg/block"
	blockmetadata "github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/grafana/pyroscope/v2/pkg/metrics"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/testutil"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetrics"
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
	require.Len(t, compactedBlocks[0].Datasets, 5)

	compactedJson, err := json.MarshalIndent(compactedBlocks, "", "  ")
	require.NoError(t, err)
	expectedJson, err := os.ReadFile("testdata/compacted.golden")
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedJson), string(compactedJson))
	assertSymbolBloomIndex(t, ctx, dst, compactedBlocks[0])
	assertSymbolBloomLookup(t, ctx, dst, compactedBlocks[0])

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
		require.Len(t, compactedBlocks[0].Datasets, 5)
	})
}

func assertSymbolBloomIndex(t *testing.T, ctx context.Context, bucket objstore.Bucket, md *metastorev1.BlockMeta) {
	t.Helper()

	matcher := labels.MustNewMatcher(labels.MatchEqual, blockmetadata.LabelNameTenantDataset, blockmetadata.LabelValueSymbolBloomIndex)
	var symbolBloomDataset *metastorev1.Dataset
	for ds := range blockmetadata.FindDatasets(md, matcher) {
		symbolBloomDataset = ds
		break
	}
	require.NotNil(t, symbolBloomDataset)
	require.Equal(t, uint32(block.DatasetFormatSymbolBloomIndex), symbolBloomDataset.Format)

	obj := block.NewObject(bucket, md)
	ds := block.NewDataset(symbolBloomDataset, obj)
	require.NoError(t, ds.Open(ctx, block.SectionSymbolBloomIndex))
	defer func() { require.NoError(t, ds.Close()) }()

	rows, err := ds.SymbolBloomIndex().ReadAll()
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	var found bool
	for _, row := range rows {
		if row.ServiceName != "pyroscope-test/ingester" {
			continue
		}
		contains, err := row.MightContain("github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push")
		require.NoError(t, err)
		if contains {
			found = true
			break
		}
	}
	require.True(t, found)
}

func assertSymbolBloomLookup(t *testing.T, ctx context.Context, bucket objstore.Bucket, md *metastorev1.BlockMeta) {
	t.Helper()

	result, err := block.LookupSymbolBloomCandidates(ctx, bucket, md, block.SymbolBloomLookupRequest{
		SymbolNames: []string{"github.com/grafana/pyroscope/pkg/does.not.exist", "github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push"},
		MinTime:     md.MinTime,
		MaxTime:     md.MaxTime,
		Matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "service_name", "pyroscope-test/ingester"),
			labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:alloc_space:bytes:space:bytes"),
		},
	})
	require.NoError(t, err)
	require.True(t, result.Complete)
	require.NotEmpty(t, result.Candidates)
	require.Equal(t, "pyroscope-test/ingester", result.Candidates[0].ServiceName)

	verified, err := block.VerifySymbolBloomCandidate(ctx, bucket, md, result.Candidates[0], "github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push")
	require.NoError(t, err)
	require.True(t, verified)
	verified, err = block.VerifySymbolBloomCandidate(ctx, bucket, md, result.Candidates[0], "github.com/grafana/pyroscope/pkg/does.not.exist")
	require.NoError(t, err)
	require.False(t, verified)
	found, err := block.VerifySymbolsInDataset(ctx, bucket, md, result.Candidates[0].DatasetIndex, []string{
		"github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push",
		"github.com/grafana/pyroscope/pkg/does.not.exist",
	}, nil)
	require.NoError(t, err)
	require.NotEmpty(t, found["github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push"])
	require.Empty(t, found["github.com/grafana/pyroscope/pkg/does.not.exist"])

	services, err := block.LookupSymbolBloomServices(ctx, bucket, md, block.SymbolBloomLookupRequest{
		SymbolNames: []string{"github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push"},
		MinTime:     md.MinTime,
		MaxTime:     md.MaxTime,
		Matchers: []*labels.Matcher{
			labels.MustNewMatcher(labels.MatchEqual, "service_name", "pyroscope-test/ingester"),
			labels.MustNewMatcher(labels.MatchEqual, "__profile_type__", "memory:alloc_space:bytes:space:bytes"),
		},
	})
	require.NoError(t, err)
	require.True(t, services.Complete)
	require.Equal(t, []block.SymbolBloomSymbolResult{{
		SymbolName: "github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push",
		Services: []block.SymbolBloomVerifiedService{{
			ServiceName:  "pyroscope-test/ingester",
			ProfileTypes: []string{"memory:alloc_space:bytes:space:bytes"},
		}},
	}}, services.Results)

	result, err = block.LookupSymbolBloomCandidates(ctx, bucket, md, block.SymbolBloomLookupRequest{
		SymbolName: "github.com/grafana/pyroscope/pkg/does.not.exist",
	})
	require.NoError(t, err)
	require.True(t, result.Complete)
	require.Empty(t, result.Candidates)

	_, err = block.LookupSymbolBloomCandidates(ctx, bucket, md, block.SymbolBloomLookupRequest{
		SymbolName:    "net/http.HandlerFunc.ServeHTTP",
		MaxCandidates: 1,
	})
	require.True(t, errors.Is(err, block.ErrSymbolBloomTooManyCandidates))

	withoutIndex := md.CloneVT()
	withoutIndex.Datasets = withoutIndex.Datasets[:len(withoutIndex.Datasets)-1]
	withoutIndex.MetadataOffset = 0
	result, err = block.LookupSymbolBloomCandidates(ctx, bucket, withoutIndex, block.SymbolBloomLookupRequest{
		SymbolName: "github.com/grafana/pyroscope/pkg/ingester.(*Ingester).Push",
	})
	require.NoError(t, err)
	require.False(t, result.Complete)
	require.Empty(t, result.Candidates)
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
	require.Len(t, compactedBlocks[0].Datasets, 5)

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
	assert.JSONEq(t, string(expectedJson), string(compactedJson))
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
	require.Len(t, compactedBlocks[0].Datasets, 5)

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
	assert.JSONEq(t, string(expectedJson), string(compactedJson))
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
