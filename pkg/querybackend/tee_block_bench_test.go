package querybackend

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	thanosobjstore "github.com/thanos-io/objstore"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// benchmarkLatencyBucket delays each remote-style request, not each reader.Read.
// ReaderAtBucket routes ReadAt through GetRange, so range reads are counted once.
type benchmarkLatencyBucket struct {
	thanosobjstore.Bucket
	latency  time.Duration
	requests atomic.Int64
}

func (b *benchmarkLatencyBucket) wait(ctx context.Context) error {
	b.requests.Add(1)
	if b.latency == 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(b.latency)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (b *benchmarkLatencyBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	return b.Bucket.GetRange(ctx, name, off, length)
}

func (b *benchmarkLatencyBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	if err := b.wait(ctx); err != nil {
		return nil, err
	}
	return b.Bucket.Get(ctx, name)
}

func (b *benchmarkLatencyBucket) Attributes(ctx context.Context, name string) (thanosobjstore.ObjectAttributes, error) {
	if err := b.wait(ctx); err != nil {
		return thanosobjstore.ObjectAttributes{}, err
	}
	return b.Bucket.Attributes(ctx, name)
}

// BenchmarkTeeRealBlock runs full local block queries, including decoding and
// aggregation. Set PYROSCOPE_BENCH_BLOCK to a downloaded block.bin, and optionally
// PYROSCOPE_BENCH_REQUEST_LATENCY to an object-store request delay (e.g. 3ms).
//
// Production block used for this benchmark (tenant 27821, shard 25, ~1 GiB):
// s3://ops-eu-south-0-profiles-ops-002-data/blocks/25/27821/01M8EDZE96Q4XBXG6SMRBTVTME/block.bin
// Access requires workloads-ops credentials. The object is subject to retention;
// keep a local copy outside the repository rather than checking production data in.
//
// Download and run from the repository root:
//
//	mkdir -p ../blocks/01M8EDZE96Q4XBXG6SMRBTVTME
//	aws s3 cp --profile workloads-ops \
//	  s3://ops-eu-south-0-profiles-ops-002-data/blocks/25/27821/01M8EDZE96Q4XBXG6SMRBTVTME/block.bin \
//	  ../blocks/01M8EDZE96Q4XBXG6SMRBTVTME/block.bin
//	PYROSCOPE_BENCH_BLOCK="$PWD/../blocks/01M8EDZE96Q4XBXG6SMRBTVTME/block.bin" \
//	PYROSCOPE_BENCH_REQUEST_LATENCY=3ms \
//	  go test ./pkg/querybackend -run '^$' -bench '^BenchmarkTeeRealBlock$' -benchtime=1x -count=10
//
// Unset PYROSCOPE_BENCH_REQUEST_LATENCY for local filesystem latency. Each timed
// operation invokes the complete block reader query after one untimed warmup.
func BenchmarkTeeRealBlock(b *testing.B) {
	path := os.Getenv("PYROSCOPE_BENCH_BLOCK")
	if path == "" {
		b.Skip("set PYROSCOPE_BENCH_BLOCK to block.bin")
	}
	path, err := filepath.Abs(path)
	require.NoError(b, err)
	var md metastorev1.BlockMeta
	{
		data, err := os.ReadFile(path)
		require.NoError(b, err)
		require.NoError(b, metadata.Decode(data, &md))
		md.Size = uint64(len(data))
	}
	root := b.TempDir()
	object := filepath.Join(root, block.ObjectPath(&md))
	require.NoError(b, os.MkdirAll(filepath.Dir(object), 0755))
	require.NoError(b, os.Symlink(path, object))
	bucket, err := filesystem.NewBucket(root)
	require.NoError(b, err)
	b.Cleanup(func() { _ = bucket.Close() })
	var latency time.Duration
	if value := os.Getenv("PYROSCOPE_BENCH_REQUEST_LATENCY"); value != "" {
		latency, err = time.ParseDuration(value)
		require.NoError(b, err)
		require.GreaterOrEqual(b, latency, time.Duration(0))
	}
	storage := &benchmarkLatencyBucket{Bucket: bucket, latency: latency}
	reader := NewBlockReader(log.NewNopLogger(), &objstore.ReaderAtBucket{Bucket: storage}, nil, validation.MockDefaultOverrides())
	b.Logf("object-store request latency=%s", latency)
	meta := []*metastorev1.BlockMeta{md.CloneVT()}
	sanitizeMetadata(meta)
	plan := queryplan.Build(meta, 10, 10)
	var tenants []string
	for _, ds := range meta[0].Datasets {
		tenants = append(tenants, meta[0].StringTable[ds.Tenant])
	}
	slices.Sort(tenants)
	tenants = slices.Compact(tenants)
	b.Logf("block=%s bytes=%d datasets=%d time=[%d,%d] tenants=%v", md.Id, md.Size, len(meta[0].Datasets), md.MinTime, md.MaxTime, tenants)

	// Pick the Format0 dataset used in single-service TimeSeries + Individual Exemplar benchmarks
	// (mimir-ops-03/warpstream-agent-write if present, else the largest Format0 dataset).
	var (
		targetDS  *metastorev1.Dataset
		singleSvc string
	)
	for _, ds := range md.Datasets {
		if block.DatasetFormat(ds.Format) != block.DatasetFormat0 {
			continue
		}
		if md.StringTable[ds.Name] == "mimir-ops-03/warpstream-agent-write" {
			targetDS = ds
			break
		}
		if targetDS == nil || ds.Size > targetDS.Size {
			targetDS = ds
		}
	}
	var singlePlan *queryv1.QueryPlan
	var exemplarProfileID string
	var exemplarTS int64
	var exemplarLabelSelector string
	if targetDS != nil {
		singleSvc = md.StringTable[targetDS.Name]
		singleMeta := md.CloneVT()
		singleMeta.Datasets = []*metastorev1.Dataset{targetDS.CloneVT()}
		singlePlan = queryplan.Build([]*metastorev1.BlockMeta{singleMeta}, 10, 10)
		exemplarLabelSelector = `{service_name="` + singleSvc + `",__profile_type__="memory:alloc_objects:count:space:bytes"}`

		tsResp, err := reader.Invoke(context.Background(), &queryv1.InvokeRequest{
			StartTime:     md.MinTime,
			EndTime:       md.MaxTime + 1,
			LabelSelector: exemplarLabelSelector,
			QueryPlan:     singlePlan,
			Tenant:        tenants,
			Query: []*queryv1.Query{{
				QueryType: queryv1.QueryType_QUERY_TIME_SERIES_COMPACT,
				TimeSeriesCompact: &queryv1.TimeSeriesQuery{
					Step:         15,
					GroupBy:      []string{"pod"},
					ExemplarType: typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL,
				},
			}},
		})
		require.NoError(b, err)
		for _, s := range tsResp.Reports[0].GetTimeSeriesCompact().GetTimeSeries() {
			for _, pt := range s.GetPoints() {
				if len(pt.GetExemplars()) > 0 {
					exemplarProfileID = pt.Exemplars[0].ProfileId
					exemplarTS = pt.Exemplars[0].Timestamp
					break
				}
			}
			if exemplarProfileID != "" {
				break
			}
		}
		b.Logf("single_service=%q size=%d exemplar_profile_id=%s exemplar_ts=%d", singleSvc, targetDS.Size, exemplarProfileID, exemplarTS)
	}

	type benchCase struct {
		name string
		req  *queryv1.InvokeRequest
	}
	cases := []benchCase{
		{
			name: "Tree",
			req: &queryv1.InvokeRequest{
				StartTime:     md.MinTime,
				EndTime:       md.MaxTime + 1,
				LabelSelector: "{}",
				QueryPlan:     plan,
				Tenant:        tenants,
				Query:         []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: 1024}}},
			},
		},
		{
			name: "TimeSeries",
			req: &queryv1.InvokeRequest{
				StartTime:     md.MinTime,
				EndTime:       md.MaxTime + 1,
				LabelSelector: "{}",
				QueryPlan:     plan,
				Tenant:        tenants,
				Query:         []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_TIME_SERIES, TimeSeries: &queryv1.TimeSeriesQuery{Step: 60, GroupBy: []string{"service_name"}}}},
			},
		},
	}
	if singlePlan != nil {
		cases = append(cases, benchCase{
			name: "ServiceTimeSeriesExemplars",
			req: &queryv1.InvokeRequest{
				StartTime:     md.MinTime,
				EndTime:       md.MaxTime + 1,
				LabelSelector: exemplarLabelSelector,
				QueryPlan:     singlePlan,
				Tenant:        tenants,
				Query: []*queryv1.Query{{
					QueryType: queryv1.QueryType_QUERY_TIME_SERIES_COMPACT,
					TimeSeriesCompact: &queryv1.TimeSeriesQuery{
						Step:         15,
						GroupBy:      []string{"pod"},
						ExemplarType: typesv1.ExemplarType_EXEMPLAR_TYPE_INDIVIDUAL,
					},
				}},
			},
		})
	}
	if singlePlan != nil && exemplarProfileID != "" {
		cases = append(cases, benchCase{
			name: "IndividualExemplarTree",
			req: &queryv1.InvokeRequest{
				StartTime:     exemplarTS - 1000,
				EndTime:       exemplarTS + 1000,
				LabelSelector: exemplarLabelSelector,
				QueryPlan:     singlePlan,
				Tenant:        tenants,
				Query: []*queryv1.Query{{
					QueryType: queryv1.QueryType_QUERY_TREE,
					Tree: &queryv1.TreeQuery{
						MaxNodes:          1024,
						ProfileIdSelector: []string{exemplarProfileID},
					},
				}},
			},
		})
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			req := tc.req
			// Warm the filesystem cache and record a response digest outside timing.
			resp, err := reader.Invoke(context.Background(), req)
			require.NoError(b, err)
			require.NotEmpty(b, resp.Reports)
			encoded, err := json.Marshal(resp.Reports)
			require.NoError(b, err)
			b.Logf("reports=%d response_bytes=%d sha256=%x", len(resp.Reports), len(encoded), sha256.Sum256(encoded))
			b.ReportAllocs()
			storage.requests.Store(0)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				resp, err := reader.Invoke(context.Background(), req)
				if err != nil {
					b.Fatal(err)
				}
				if len(resp.Reports) == 0 {
					b.Fatal("empty reports")
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(storage.requests.Load())/float64(b.N), "requests/op")
		})
	}
}
