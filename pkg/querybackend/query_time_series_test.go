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
	ctx    context.Context
	reader *BlockReader
	plan   *queryv1.QueryPlan
	tenant []string
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

	return &benchmarkFixture{
		ctx:    context.Background(),
		reader: reader,
		plan:   plan,
		tenant: tenant,
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
	_, err := f.reader.Invoke(f.ctx, req)
	if err != nil {
		b.Fatalf("query failed: %v", err)
	}
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

	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)

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
				oneHourAgo, now,
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
	now := time.Now()

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
						now.Add(-tr.duration), now,
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
