package querybackend

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/pkg/test"
)

func TestValidateExemplarType(t *testing.T) {
	tests := []struct {
		name             string
		exemplarType     typesv1.ExemplarType
		expectedInclude  bool
		expectedErrorMsg string
		expectedCode     codes.Code
	}{
		{
			name:            "UNSPECIFIED returns false, no error",
			exemplarType:    typesv1.ExemplarType_EXEMPLAR_TYPE_UNSPECIFIED,
			expectedInclude: false,
		},
		{
			name:            "NONE returns false, no error",
			exemplarType:    typesv1.ExemplarType_EXEMPLAR_TYPE_NONE,
			expectedInclude: false,
		},
		{
			name:            "INDIVIDUAL returns true, no error",
			exemplarType:    typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL,
			expectedInclude: true,
		},
		{
			name:             "SPAN returns error with Unimplemented code",
			exemplarType:     typesv1.ExemplarType_EXEMPLAR_TYPE_SPAN,
			expectedInclude:  false,
			expectedErrorMsg: "exemplar type span is not implemented",
			expectedCode:     codes.Unimplemented,
		},
		{
			name:             "Unknown type returns error with InvalidArgument code",
			exemplarType:     typesv1.ExemplarType(999),
			expectedInclude:  false,
			expectedErrorMsg: "unknown exemplar type",
			expectedCode:     codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			include, err := validateExemplarType(tt.exemplarType)
			if tt.expectedErrorMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				st, ok := status.FromError(err)
				require.True(t, ok)
				assert.Equal(t, tt.expectedCode, st.Code())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedInclude, include)
			}
		})
	}
}

type benchmarkFixture struct {
	ctx       context.Context
	reader    *BlockReader
	plan      *queryv1.QueryPlan
	tenant    []string
	startTime time.Time
}

// setupBenchmarkFixture creates a benchmark fixture with real block data.
func setupBenchmarkFixture(b *testing.B) *benchmarkFixture {
	b.Helper()

	bucket := memory.NewInMemBucket()
	var blocks []*metastorev1.BlockMeta

	err := filepath.WalkDir("testdata/samples", func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var md metastorev1.BlockMeta
		if err = metadata.Decode(data, &md); err != nil {
			return err
		}
		md.Size = uint64(len(data))
		blocks = append(blocks, &md)
		return bucket.Upload(context.Background(), block.ObjectPath(&md), bytes.NewReader(data))
	})
	if err != nil {
		b.Fatalf("failed to load test data: %v", err)
	}

	logger := test.NewTestingLogger(b)
	reader := NewBlockReader(logger, &objstore.ReaderAtBucket{Bucket: bucket}, nil)

	meta := make([]*metastorev1.BlockMeta, len(blocks))
	for i, block := range blocks {
		meta[i] = block.CloneVT()
	}
	sanitizeMetadata(meta)

	plan := queryplan.Build(meta, 10, 10)

	var tenant []string
	for _, b := range plan.Root.Blocks {
		for _, d := range b.Datasets {
			tenant = append(tenant, b.StringTable[d.Tenant])
		}
	}

	// Extract start time from blocks
	var minTime int64 = -1
	for _, block := range blocks {
		if minTime == -1 || block.MinTime < minTime {
			minTime = block.MinTime
		}
	}
	startTime := time.UnixMilli(minTime)

	return &benchmarkFixture{
		ctx:       context.Background(),
		reader:    reader,
		plan:      plan,
		tenant:    tenant,
		startTime: startTime,
	}
}

// sanitizeMetadata removes duplicate datasets (logic from testSuite.sanitizeMetadata)
func sanitizeMetadata(meta []*metastorev1.BlockMeta) {
	for _, m := range meta {
		for _, d := range m.Datasets {
			if block.DatasetFormat(d.Format) == block.DatasetFormat1 {
				m.Datasets = slices.DeleteFunc(m.Datasets, func(x *metastorev1.Dataset) bool {
					return x.Format == 0
				})
				break
			}
		}
	}
}

// runTimeSeriesQuery executes a timeseries query with the given parameters.
func (f *benchmarkFixture) runTimeSeriesQuery(b *testing.B, req *queryv1.InvokeRequest) {
	b.Helper()
	resp, err := f.reader.Invoke(f.ctx, req)
	if err != nil {
		b.Fatalf("query failed: %v", err)
	}
	for _, r := range resp.Reports {
		if r.ReportType != queryv1.ReportType_REPORT_TIME_SERIES {
			continue
		}
		for _, s := range r.TimeSeries.TimeSeries {
			for _, p := range s.Points {
				if p.Value > 0 {
					return
				}
			}
		}
	}
	panic("no data found")
}

// makeTimeSeriesRequest creates a timeseries query request with the given parameters.
func (f *benchmarkFixture) makeTimeSeriesRequest(
	startTime, endTime time.Time,
	labelSelector string,
	groupBy []string,
	exemplarType typesv1.ExemplarType,
) *queryv1.InvokeRequest {
	return &queryv1.InvokeRequest{
		StartTime:     startTime.UnixMilli(),
		EndTime:       endTime.UnixMilli(),
		LabelSelector: labelSelector,
		QueryPlan:     f.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TIME_SERIES,
			TimeSeries: &queryv1.TimeSeriesQuery{
				Step:         60.0, // 1 minute resolution
				GroupBy:      groupBy,
				ExemplarType: exemplarType,
			},
		}},
		Tenant: f.tenant,
	}
}

// BenchmarkTimeSeriesQuery measures the performance impact of exemplar collection.
//
//	go test -bench=BenchmarkTimeSeriesQuery$ -benchmem ./pkg/querybackend/
//
// Expected results: Exemplar overhead should be < 30% for typical queries.
func BenchmarkTimeSeriesQuery(b *testing.B) {
	fixture := setupBenchmarkFixture(b)

	benchmarks := []struct {
		name         string
		exemplarType typesv1.ExemplarType
	}{
		{"NoExemplars", typesv1.ExemplarType_EXEMPLAR_TYPE_NONE},
		{"WithExemplars", typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			req := fixture.makeTimeSeriesRequest(
				fixture.startTime, fixture.startTime.Add(time.Hour),
				"{}",
				[]string{"service_name"},
				bm.exemplarType,
			)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				fixture.runTimeSeriesQuery(b, req)
			}
		})
	}
}

// BenchmarkTimeSeriesQuery_TimeRange measures how performance scales with time range.
//
// This tests whether exemplar overhead grows linearly or non-linearly with data size.
// Run with:
//
//	go test -bench=BenchmarkTimeSeriesQuery_TimeRange -benchmem ./pkg/querybackend/
//
// Expected results: Overhead ratio should remain constant across time ranges.
func BenchmarkTimeSeriesQuery_TimeRange(b *testing.B) {
	fixture := setupBenchmarkFixture(b)

	timeRanges := []struct {
		name     string
		duration time.Duration
	}{
		{"1Minute", 1 * time.Minute},
		{"5Minutes", 5 * time.Minute},
		{"15Minutes", 15 * time.Minute},
		{"1Hour", 1 * time.Hour},
	}

	exemplarTypes := []struct {
		name string
		typ  typesv1.ExemplarType
	}{
		{"NoExemplars", typesv1.ExemplarType_EXEMPLAR_TYPE_NONE},
		{"WithExemplars", typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL},
	}

	for _, tr := range timeRanges {
		b.Run(tr.name, func(b *testing.B) {
			for _, et := range exemplarTypes {
				b.Run(et.name, func(b *testing.B) {
					req := fixture.makeTimeSeriesRequest(
						fixture.startTime, fixture.startTime.Add(tr.duration),
						"{}",
						[]string{"service_name"},
						et.typ,
					)

					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						fixture.runTimeSeriesQuery(b, req)
					}
				})
			}
		})
	}
}

func TestTimeSeriesCompactAggregator(t *testing.T) {
	query := &queryv1.Query{
		TimeSeriesCompact: &queryv1.TimeSeriesQuery{
			GroupBy: []string{"service_name"},
			Step:    1.0,
		},
	}

	req := &queryv1.InvokeRequest{
		StartTime: 1000,
		EndTime:   2000,
		Query:     []*queryv1.Query{query},
	}

	agg := newTimeSeriesCompactAggregator(req).(*timeSeriesCompactAggregator)

	// Report 1: 3 attributes (pod, version, region) at timestamp 1000
	table1 := &queryv1.AttributeTable{
		Keys:   []string{"service_name", "pod", "version", "region"},
		Values: []string{"api", "a", "1.0", "us-east-1"},
	}
	report1 := &queryv1.Report{
		TimeSeriesCompact: &queryv1.TimeSeriesCompactReport{
			Query: query.TimeSeriesCompact,
			TimeSeries: []*queryv1.Series{{
				AttributeRefs: []int64{0},
				Points: []*queryv1.Point{{
					Timestamp: 1000,
					Value:     100,
					Exemplars: []*queryv1.Exemplar{{
						ProfileId:     "prof-1",
						Value:         100,
						Timestamp:     1000,
						AttributeRefs: []int64{1, 2, 3},
					}},
				}},
			}},
			AttributeTable: table1,
		},
	}

	// Report 2: series label + 2 exemplar attributes at timestamp 2000 - different structure!
	table2 := &queryv1.AttributeTable{
		Keys:   []string{"service_name", "pod", "version"},
		Values: []string{"api", "b", "1.0"},
	}
	report2 := &queryv1.Report{
		TimeSeriesCompact: &queryv1.TimeSeriesCompactReport{
			Query: query.TimeSeriesCompact,
			TimeSeries: []*queryv1.Series{{
				AttributeRefs: []int64{0},
				Points: []*queryv1.Point{{
					Timestamp: 2000,
					Value:     200,
					Exemplars: []*queryv1.Exemplar{{
						ProfileId:     "prof-2",
						Value:         200,
						Timestamp:     2000,
						AttributeRefs: []int64{1, 2},
					}},
				}},
			}},
			AttributeTable: table2,
		},
	}

	// Report 3: Same as report1 to test string interning - duplicate labels!
	table3 := &queryv1.AttributeTable{
		Keys:   []string{"service_name", "pod", "version", "region"},
		Values: []string{"api", "a", "1.0", "us-east-1"},
	}
	report3 := &queryv1.Report{
		TimeSeriesCompact: &queryv1.TimeSeriesCompactReport{
			Query: query.TimeSeriesCompact,
			TimeSeries: []*queryv1.Series{{
				AttributeRefs: []int64{0},
				Points: []*queryv1.Point{{
					Timestamp: 1000,
					Value:     50,
					Exemplars: []*queryv1.Exemplar{{
						ProfileId:     "prof-3",
						Value:         50,
						Timestamp:     1000,
						AttributeRefs: []int64{1, 2, 3},
					}},
				}},
			}},
			AttributeTable: table3,
		},
	}

	err := agg.aggregate(report1)
	require.NoError(t, err)
	err = agg.aggregate(report2)
	require.NoError(t, err)
	err = agg.aggregate(report3)
	require.NoError(t, err)

	result := agg.build()
	require.NotNil(t, result.TimeSeriesCompact)
	require.NotNil(t, result.TimeSeriesCompact.AttributeTable)
	require.Len(t, result.TimeSeriesCompact.TimeSeries, 1)

	series := result.TimeSeriesCompact.TimeSeries[0]
	require.Len(t, series.Points, 2, "Should have 2 points (different timestamps)")

	attrTable := result.TimeSeriesCompact.AttributeTable

	attrMap := make(map[int64]struct {
		key   string
		value string
	})
	for i := range attrTable.Keys {
		attrMap[int64(i)] = struct {
			key   string
			value string
		}{attrTable.Keys[i], attrTable.Values[i]}
	}

	// Point 1: timestamp 1000 - should keep prof-1 (value=100) not prof-3 (value=50)
	point1 := series.Points[0]
	require.Equal(t, int64(1000), point1.Timestamp)
	require.Len(t, point1.Exemplars, 1, "RangeSeries limits to 1 exemplar per point")
	ex1 := point1.Exemplars[0]
	assert.Equal(t, "prof-1", ex1.ProfileId, "Should keep highest value exemplar at timestamp 1000")
	assert.Equal(t, int64(100), ex1.Value)
	require.Len(t, ex1.AttributeRefs, 3)

	// Verify prof-1 attributes: pod=a, version=1.0, region=us-east-1
	ex1Labels := make(map[string]string)
	for _, ref := range ex1.AttributeRefs {
		attr := attrMap[ref]
		ex1Labels[attr.key] = attr.value
	}
	assert.Equal(t, "a", ex1Labels["pod"])
	assert.Equal(t, "1.0", ex1Labels["version"])
	assert.Equal(t, "us-east-1", ex1Labels["region"])

	// Point 2: timestamp 2000 - should have prof-2
	point2 := series.Points[1]
	require.Equal(t, int64(2000), point2.Timestamp)
	require.Len(t, point2.Exemplars, 1)
	ex2 := point2.Exemplars[0]
	assert.Equal(t, "prof-2", ex2.ProfileId)
	assert.Equal(t, int64(200), ex2.Value)
	require.Len(t, ex2.AttributeRefs, 2)

	// Verify prof-2 attributes: pod=b, version=1.0 (no region)
	ex2Labels := make(map[string]string)
	for _, ref := range ex2.AttributeRefs {
		attr := attrMap[ref]
		ex2Labels[attr.key] = attr.value
	}
	assert.Equal(t, "b", ex2Labels["pod"])
	assert.Equal(t, "1.0", ex2Labels["version"])
	_, hasRegion := ex2Labels["region"]
	assert.False(t, hasRegion, "prof-2 should not have region label")

	assert.Len(t, attrTable.Keys, 5)
	assert.Len(t, attrTable.Values, 5)

	allKeys := make(map[string][]string)
	for i := range attrTable.Keys {
		key := attrTable.Keys[i]
		value := attrTable.Values[i]
		allKeys[key] = append(allKeys[key], value)
	}
	assert.ElementsMatch(t, []string{"a", "b"}, allKeys["pod"])
	assert.ElementsMatch(t, []string{"1.0"}, allKeys["version"])
	assert.ElementsMatch(t, []string{"us-east-1"}, allKeys["region"])
}

func TestTimeSeriesCompactAnnotations(t *testing.T) {
	query := &queryv1.Query{
		TimeSeriesCompact: &queryv1.TimeSeriesQuery{
			GroupBy: []string{"service_name"},
			Step:    1.0,
		},
	}

	req := &queryv1.InvokeRequest{
		StartTime: 1000,
		EndTime:   2000,
		Query:     []*queryv1.Query{query},
	}

	agg := newTimeSeriesCompactAggregator(req).(*timeSeriesCompactAggregator)

	table1 := &queryv1.AttributeTable{
		Keys:   []string{"service_name", "error", "host"},
		Values: []string{"api", "true", "server-1"},
	}
	report1 := &queryv1.Report{
		TimeSeriesCompact: &queryv1.TimeSeriesCompactReport{
			Query: query.TimeSeriesCompact,
			TimeSeries: []*queryv1.Series{{
				AttributeRefs: []int64{0},
				Points: []*queryv1.Point{{
					Timestamp:      1000,
					Value:          100,
					AnnotationRefs: []int64{1, 2},
				}},
			}},
			AttributeTable: table1,
		},
	}

	// Report 2: different annotations at timestamp 1000 (should merge)
	table2 := &queryv1.AttributeTable{
		Keys:   []string{"service_name", "error"},
		Values: []string{"api", "false"},
	}
	report2 := &queryv1.Report{
		TimeSeriesCompact: &queryv1.TimeSeriesCompactReport{
			Query: query.TimeSeriesCompact,
			TimeSeries: []*queryv1.Series{{
				AttributeRefs: []int64{0},
				Points: []*queryv1.Point{{
					Timestamp:      1000,
					Value:          50,
					AnnotationRefs: []int64{1}, // error=false
				}},
			}},
			AttributeTable: table2,
		},
	}

	err := agg.aggregate(report1)
	require.NoError(t, err)
	err = agg.aggregate(report2)
	require.NoError(t, err)

	result := agg.build()
	require.NotNil(t, result.TimeSeriesCompact)
	require.Len(t, result.TimeSeriesCompact.TimeSeries, 1)

	series := result.TimeSeriesCompact.TimeSeries[0]
	require.Len(t, series.Points, 1)

	point := series.Points[0]
	require.Equal(t, int64(1000), point.Timestamp)
	require.Equal(t, 150.0, point.Value)
	require.Len(t, point.AnnotationRefs, 3)

	attrTable := result.TimeSeriesCompact.AttributeTable
	annotations := make([]*typesv1.ProfileAnnotation, len(point.AnnotationRefs))
	for i, ref := range point.AnnotationRefs {
		annotations[i] = &typesv1.ProfileAnnotation{
			Key:   attrTable.Keys[ref],
			Value: attrTable.Values[ref],
		}
	}

	keys := make([]string, len(annotations))
	for i, a := range annotations {
		keys[i] = a.Key + "=" + a.Value
	}
	assert.ElementsMatch(t, []string{"error=true", "host=server-1", "error=false"}, keys)
}
