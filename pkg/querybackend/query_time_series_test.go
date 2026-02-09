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

func newTestCompactAggregator(startTime, endTime int64) (*timeSeriesCompactAggregator, *queryv1.TimeSeriesQuery) {
	query := &queryv1.TimeSeriesQuery{
		GroupBy: []string{"service_name"},
		Step:    1.0,
	}
	req := &queryv1.InvokeRequest{
		StartTime: startTime,
		EndTime:   endTime,
		Query: []*queryv1.Query{{
			TimeSeriesCompact: query,
		}},
	}
	return newTimeSeriesCompactAggregator(req).(*timeSeriesCompactAggregator), query
}

func makeCompactReport(query *queryv1.TimeSeriesQuery, table *queryv1.AttributeTable, series []*queryv1.Series) *queryv1.Report {
	return &queryv1.Report{
		TimeSeriesCompact: &queryv1.TimeSeriesCompactReport{
			Query:          query,
			TimeSeries:     series,
			AttributeTable: table,
		},
	}
}

func resolveRefs(refs []int64, table *queryv1.AttributeTable) []string {
	result := make([]string, len(refs))
	for i, ref := range refs {
		result[i] = table.Keys[ref] + "=" + table.Values[ref]
	}
	return result
}

func TestTimeSeriesCompactAggregator(t *testing.T) {
	agg, query := newTestCompactAggregator(1000, 2000)

	// Report 1: exemplar with 3 attributes (pod, version, region) at timestamp 1000
	report1 := makeCompactReport(query, &queryv1.AttributeTable{
		Keys:   []string{"service_name", "pod", "version", "region"},
		Values: []string{"api", "a", "1.0", "us-east-1"},
	}, []*queryv1.Series{{
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
	}})

	// Report 2: exemplar with 2 attributes at timestamp 2000
	report2 := makeCompactReport(query, &queryv1.AttributeTable{
		Keys:   []string{"service_name", "pod", "version"},
		Values: []string{"api", "b", "1.0"},
	}, []*queryv1.Series{{
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
	}})

	// Report 3: same timestamp as report1, lower value exemplar (tests highest-value selection)
	report3 := makeCompactReport(query, &queryv1.AttributeTable{
		Keys:   []string{"service_name", "pod", "version", "region"},
		Values: []string{"api", "a", "1.0", "us-east-1"},
	}, []*queryv1.Series{{
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
	}})

	require.NoError(t, agg.aggregate(report1))
	require.NoError(t, agg.aggregate(report2))
	require.NoError(t, agg.aggregate(report3))

	result := agg.build()
	require.NotNil(t, result.TimeSeriesCompact)
	require.NotNil(t, result.TimeSeriesCompact.AttributeTable)
	require.Len(t, result.TimeSeriesCompact.TimeSeries, 1)

	series := result.TimeSeriesCompact.TimeSeries[0]
	attrTable := result.TimeSeriesCompact.AttributeTable
	require.Len(t, series.Points, 2, "Should have 2 points (different timestamps)")

	// Point 1: timestamp 1000 - should keep prof-1 (value=100) not prof-3 (value=50)
	point1 := series.Points[0]
	require.Equal(t, int64(1000), point1.Timestamp)
	require.Len(t, point1.Exemplars, 1)
	assert.Equal(t, "prof-1", point1.Exemplars[0].ProfileId)
	assert.Equal(t, int64(100), point1.Exemplars[0].Value)
	assert.ElementsMatch(t, []string{"pod=a", "version=1.0", "region=us-east-1"},
		resolveRefs(point1.Exemplars[0].AttributeRefs, attrTable))

	// Point 2: timestamp 2000 - should have prof-2
	point2 := series.Points[1]
	require.Equal(t, int64(2000), point2.Timestamp)
	require.Len(t, point2.Exemplars, 1)
	assert.Equal(t, "prof-2", point2.Exemplars[0].ProfileId)
	assert.ElementsMatch(t, []string{"pod=b", "version=1.0"},
		resolveRefs(point2.Exemplars[0].AttributeRefs, attrTable))

	// Verify attribute table has deduplicated entries
	assert.Len(t, attrTable.Keys, 5)
}

func TestTimeSeriesCompactAnnotations(t *testing.T) {
	t.Run("same_timestamp_merges_annotations", func(t *testing.T) {
		agg, query := newTestCompactAggregator(1000, 2000)

		// Report 1: annotations error=true, host=server-1
		report1 := makeCompactReport(query, &queryv1.AttributeTable{
			Keys:   []string{"service_name", "error", "host"},
			Values: []string{"api", "true", "server-1"},
		}, []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{{
				Timestamp:      1000,
				Value:          100,
				AnnotationRefs: []int64{1, 2},
			}},
		}})

		// Report 2: annotation error=false at same timestamp
		report2 := makeCompactReport(query, &queryv1.AttributeTable{
			Keys:   []string{"service_name", "error"},
			Values: []string{"api", "false"},
		}, []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{{
				Timestamp:      1000,
				Value:          50,
				AnnotationRefs: []int64{1},
			}},
		}})

		require.NoError(t, agg.aggregate(report1))
		require.NoError(t, agg.aggregate(report2))

		result := agg.build()
		series := result.TimeSeriesCompact.TimeSeries[0]
		require.Len(t, series.Points, 1)

		point := series.Points[0]
		assert.Equal(t, int64(1000), point.Timestamp)
		assert.Equal(t, 150.0, point.Value) // 100 + 50
		assert.ElementsMatch(t, []string{"error=true", "host=server-1", "error=false"},
			resolveRefs(point.AnnotationRefs, result.TimeSeriesCompact.AttributeTable))
	})

	t.Run("different_timestamps_no_corruption", func(t *testing.T) {
		// annotations at different timestamps should not be corrupted by later points.
		agg, query := newTestCompactAggregator(1000, 3000)

		report := makeCompactReport(query, &queryv1.AttributeTable{
			Keys:   []string{"service_name", "error", "host", "region"},
			Values: []string{"api", "true", "server-1", "us-east"},
		}, []*queryv1.Series{{
			AttributeRefs: []int64{0},
			Points: []*queryv1.Point{
				{Timestamp: 1000, Value: 100, AnnotationRefs: []int64{1}},
				{Timestamp: 2000, Value: 200, AnnotationRefs: []int64{2}},
				{Timestamp: 3000, Value: 300, AnnotationRefs: []int64{3}},
			},
		}})

		require.NoError(t, agg.aggregate(report))

		result := agg.build()
		series := result.TimeSeriesCompact.TimeSeries[0]
		attrTable := result.TimeSeriesCompact.AttributeTable
		require.Len(t, series.Points, 3)

		// Each point should have its own annotations, not corrupted by later points
		assert.ElementsMatch(t, []string{"error=true"}, resolveRefs(series.Points[0].AnnotationRefs, attrTable))
		assert.ElementsMatch(t, []string{"host=server-1"}, resolveRefs(series.Points[1].AnnotationRefs, attrTable))
		assert.ElementsMatch(t, []string{"region=us-east"}, resolveRefs(series.Points[2].AnnotationRefs, attrTable))
	})
}

type benchmarkFixture struct {
	ctx       context.Context
	reader    *BlockReader
	plan      *queryv1.QueryPlan
	tenant    []string
	startTime time.Time
}

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

	var minTime int64 = -1
	for _, block := range blocks {
		if minTime == -1 || block.MinTime < minTime {
			minTime = block.MinTime
		}
	}

	return &benchmarkFixture{
		ctx:       context.Background(),
		reader:    reader,
		plan:      plan,
		tenant:    tenant,
		startTime: time.UnixMilli(minTime),
	}
}

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
				Step:         60.0,
				GroupBy:      groupBy,
				ExemplarType: exemplarType,
			},
		}},
		Tenant: f.tenant,
	}
}

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
