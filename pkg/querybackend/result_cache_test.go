package querybackend

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	thanobjstore "github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlareobjstore "github.com/grafana/pyroscope/v2/pkg/objstore"
)

type resultCacheOverrides struct {
	enabled    bool
	generation uint32
	window     time.Duration
}

func (o resultCacheOverrides) ResultCacheEnabled(string) bool       { return o.enabled }
func (o resultCacheOverrides) ResultCacheGeneration(string) uint32  { return o.generation }
func (o resultCacheOverrides) RejectOlderThan(string) time.Duration { return o.window }

type queryHandlerFunc func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)

func (f queryHandlerFunc) Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	return f(ctx, req)
}

func TestSplitResultCacheFragments(t *testing.T) {
	start := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC).UnixMilli()

	fragments := splitResultCacheFragments(start, end)
	require.Len(t, fragments, 3)
	require.False(t, fragments[0].full)
	require.True(t, fragments[1].full)
	require.False(t, fragments[2].full)
	require.Equal(t, time.Date(2026, 8, 20, 23, 59, 59, 999000000, time.UTC).UnixMilli(), fragments[0].end)
	require.Equal(t, time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC).UnixMilli(), fragments[1].start)
	require.Equal(t, time.Date(2026, 8, 21, 23, 59, 59, 999000000, time.UTC).UnixMilli(), fragments[1].end)
}

func TestStableResultCacheDay(t *testing.T) {
	end := time.Date(2026, 8, 20, 23, 59, 59, 999000000, time.UTC).UnixMilli()
	boundary := time.Date(2026, 8, 21, 2, 0, 0, 0, time.UTC)
	require.False(t, stableResultCacheDay(end, boundary.Add(-time.Nanosecond), time.Hour))
	require.True(t, stableResultCacheDay(end, boundary, time.Hour))
	require.False(t, stableResultCacheDay(end, boundary, 0))
}

func TestResultCacheKeyIgnoresTime(t *testing.T) {
	query := &queryv1.QueryRequest{
		StartTime:     time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC).UnixMilli(),
		EndTime:       time.Date(2026, 8, 21, 23, 59, 59, 999000000, time.UTC).UnixMilli(),
		LabelSelector: `{service_name="api"}`,
		Query:         []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_LABEL_NAMES, LabelNames: &queryv1.LabelNamesQuery{}}},
	}
	key, err := resultCacheKey("tenant-a", 1, query)
	require.NoError(t, err)
	query.StartTime += 24 * 60 * 60 * 1000
	query.EndTime += 24 * 60 * 60 * 1000
	otherDayKey, err := resultCacheKey("tenant-a", 1, query)
	require.NoError(t, err)
	require.Contains(t, key, "result-cache/tenant-a/0001-2026-08-21-")
	require.NotEqual(t, key, otherDayKey)
	require.Equal(t, key[len(key)-64:], otherDayKey[len(otherDayKey)-64:])

	labelValues := query.CloneVT()
	labelValues.Query[0] = &queryv1.Query{
		QueryType:   queryv1.QueryType_QUERY_LABEL_VALUES,
		LabelValues: &queryv1.LabelValuesQuery{LabelName: "service_name"},
	}
	labelValuesKey, err := resultCacheKey("tenant-a", 1, labelValues)
	require.NoError(t, err)
	labelValues.Query[0].LabelValues.LabelName = "cluster"
	otherLabelValuesKey, err := resultCacheKey("tenant-a", 1, labelValues)
	require.NoError(t, err)
	require.NotEqual(t, key, labelValuesKey)
	require.NotEqual(t, labelValuesKey, otherLabelValuesKey)

	seriesLabels := &queryv1.InvokeRequest{
		LabelSelector: `{service_name="api"}`,
		Query: []*queryv1.Query{{
			QueryType:    queryv1.QueryType_QUERY_SERIES_LABELS,
			SeriesLabels: &queryv1.SeriesLabelsQuery{LabelNames: []string{"service_name", "cluster"}},
		}},
	}
	seriesLabelsQuery, err := cacheQuery(seriesLabels, query.StartTime, query.EndTime)
	require.NoError(t, err)
	require.Equal(t, []string{"cluster", "service_name"}, seriesLabelsQuery.Query[0].SeriesLabels.LabelNames)
	seriesLabels.Query[0].SeriesLabels.LabelNames = []string{"cluster", "service_name"}
	otherSeriesLabelsQuery, err := cacheQuery(seriesLabels, query.StartTime, query.EndTime)
	require.NoError(t, err)
	seriesLabelsKey, err := resultCacheKey("tenant-a", 1, seriesLabelsQuery)
	require.NoError(t, err)
	otherSeriesLabelsKey, err := resultCacheKey("tenant-a", 1, otherSeriesLabelsQuery)
	require.NoError(t, err)
	require.Equal(t, seriesLabelsKey, otherSeriesLabelsKey)
}

func TestCanonicalResultCacheSelector(t *testing.T) {
	first, err := canonicalResultCacheSelector(`{service_name="api",environment="prod"}`)
	require.NoError(t, err)
	second, err := canonicalResultCacheSelector(`{ environment = "prod", service_name = "api" }`)
	require.NoError(t, err)
	require.Equal(t, `{environment="prod",service_name="api"}`, first)
	require.Equal(t, first, second)
}

func TestReadResultCache(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanobjstore.NewInMemBucket())
	q := &QueryBackend{resultCacheBucket: bucket, resultCacheMetrics: newResultCacheMetrics(prometheus.NewRegistry())}
	request := &queryv1.InvokeRequest{Query: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_LABEL_NAMES, LabelNames: &queryv1.LabelNamesQuery{}}}}
	expected := &queryv1.QueryRequest{StartTime: 1, EndTime: 2, Query: request.Query}
	entry := &queryv1.ResultCacheEntry{Query: expected.CloneVT(), Reports: []*queryv1.Report{{
		ReportType: queryv1.ReportType_REPORT_LABEL_NAMES,
		LabelNames: &queryv1.LabelNamesReport{Query: &queryv1.LabelNamesQuery{}, LabelNames: []string{"cluster", "service_name"}},
	}}}
	data, err := proto.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, bucket.Upload(context.Background(), "entry", bytes.NewReader(data)))

	aggregator := newAggregator(request)
	hit, err := q.readResultCache(context.Background(), resultCacheLabelNames, "entry", expected, aggregator)
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, []string{"cluster", "service_name"}, aggregator.response().Reports[0].LabelNames.LabelNames)

	entry.Query.EndTime = 3
	data, err = proto.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, bucket.Upload(context.Background(), "collision", bytes.NewReader(data)))
	hit, err = q.readResultCache(context.Background(), resultCacheLabelNames, "collision", expected, newAggregator(request))
	require.Error(t, err)
	require.False(t, hit)
}

func TestCoordinateResultCacheHitDoesNotExecutePlan(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanobjstore.NewInMemBucket())
	q := &QueryBackend{
		resultCacheBucket:    bucket,
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 7, window: time.Hour},
		resultCacheMetrics:   newResultCacheMetrics(prometheus.NewRegistry()),
		now:                  func() time.Time { return time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC) },
	}
	start := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2026, 8, 20, 23, 59, 59, 999000000, time.UTC).UnixMilli()
	req := &queryv1.InvokeRequest{
		Tenant:        []string{"tenant-a"},
		StartTime:     start,
		EndTime:       end,
		LabelSelector: "{}",
		Query:         []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_LABEL_NAMES, LabelNames: &queryv1.LabelNamesQuery{}}},
	}
	query, err := cacheQuery(req, start, end)
	require.NoError(t, err)
	key, err := resultCacheKey("tenant-a", 7, query)
	require.NoError(t, err)
	data, err := proto.Marshal(&queryv1.ResultCacheEntry{Query: query, Reports: []*queryv1.Report{{
		ReportType: queryv1.ReportType_REPORT_LABEL_NAMES,
		LabelNames: &queryv1.LabelNamesReport{Query: &queryv1.LabelNamesQuery{}, LabelNames: []string{"cluster"}},
	}}})
	require.NoError(t, err)
	require.NoError(t, bucket.Upload(context.Background(), key, bytes.NewReader(data)))

	resp, err := q.Invoke(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"cluster"}, resp.Reports[0].LabelNames.LabelNames)
	require.Zero(t, resp.Diagnostics.ExecutionNode.Stats.BytesFetched)
	require.Equal(t, float64(1), promtest.ToFloat64(q.resultCacheMetrics.lookups.WithLabelValues(resultCacheLabelNames, "hit")))
}

func TestCoordinateResultCacheLabelValuesHitDoesNotExecutePlan(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanobjstore.NewInMemBucket())
	q := &QueryBackend{
		resultCacheBucket:    bucket,
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 7, window: time.Hour},
		resultCacheMetrics:   newResultCacheMetrics(prometheus.NewRegistry()),
		now:                  func() time.Time { return time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC) },
	}
	start := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2026, 8, 20, 23, 59, 59, 999000000, time.UTC).UnixMilli()
	req := &queryv1.InvokeRequest{
		Tenant:        []string{"tenant-a"},
		StartTime:     start,
		EndTime:       end,
		LabelSelector: "{}",
		Query: []*queryv1.Query{{
			QueryType:   queryv1.QueryType_QUERY_LABEL_VALUES,
			LabelValues: &queryv1.LabelValuesQuery{LabelName: "service_name"},
		}},
	}
	query, err := cacheQuery(req, start, end)
	require.NoError(t, err)
	key, err := resultCacheKey("tenant-a", 7, query)
	require.NoError(t, err)
	data, err := proto.Marshal(&queryv1.ResultCacheEntry{Query: query, Reports: []*queryv1.Report{{
		ReportType: queryv1.ReportType_REPORT_LABEL_VALUES,
		LabelValues: &queryv1.LabelValuesReport{
			Query:       &queryv1.LabelValuesQuery{LabelName: "service_name"},
			LabelValues: []string{"api", "worker"},
		},
	}}})
	require.NoError(t, err)
	require.NoError(t, bucket.Upload(context.Background(), key, bytes.NewReader(data)))

	resp, err := q.Invoke(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"api", "worker"}, resp.Reports[0].LabelValues.LabelValues)
	require.Zero(t, resp.Diagnostics.ExecutionNode.Stats.BytesFetched)
	require.Equal(t, float64(1), promtest.ToFloat64(q.resultCacheMetrics.lookups.WithLabelValues(resultCacheLabelValues, "hit")))
}

func TestCoordinateResultCacheSeriesLabelsHitDoesNotExecutePlan(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanobjstore.NewInMemBucket())
	q := &QueryBackend{
		resultCacheBucket:    bucket,
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 7, window: time.Hour},
		resultCacheMetrics:   newResultCacheMetrics(prometheus.NewRegistry()),
		now:                  func() time.Time { return time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC) },
	}
	start := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2026, 8, 20, 23, 59, 59, 999000000, time.UTC).UnixMilli()
	req := &queryv1.InvokeRequest{
		Tenant:        []string{"tenant-a"},
		StartTime:     start,
		EndTime:       end,
		LabelSelector: "{}",
		Query: []*queryv1.Query{{
			QueryType:    queryv1.QueryType_QUERY_SERIES_LABELS,
			SeriesLabels: &queryv1.SeriesLabelsQuery{LabelNames: []string{"service_name", "cluster"}},
		}},
	}
	query, err := cacheQuery(req, start, end)
	require.NoError(t, err)
	key, err := resultCacheKey("tenant-a", 7, query)
	require.NoError(t, err)
	data, err := proto.Marshal(&queryv1.ResultCacheEntry{Query: query, Reports: []*queryv1.Report{{
		ReportType: queryv1.ReportType_REPORT_SERIES_LABELS,
		SeriesLabels: &queryv1.SeriesLabelsReport{
			Query: query.Query[0].SeriesLabels,
			SeriesLabels: []*typesv1.Labels{{Labels: []*typesv1.LabelPair{
				{Name: "cluster", Value: "prod"},
				{Name: "service_name", Value: "api"},
			}}},
		},
	}}})
	require.NoError(t, err)
	require.NoError(t, bucket.Upload(context.Background(), key, bytes.NewReader(data)))

	resp, err := q.Invoke(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, resp.Reports[0].SeriesLabels.SeriesLabels, 1)
	require.Zero(t, resp.Diagnostics.ExecutionNode.Stats.BytesFetched)
	require.Equal(t, float64(1), promtest.ToFloat64(q.resultCacheMetrics.lookups.WithLabelValues(resultCacheSeriesLabels, "hit")))
}

func TestCoordinateResultCacheLimitsConcurrentColdFragments(t *testing.T) {
	const fragments = resultCacheFragmentConcurrency + 1

	started := make(chan struct{}, fragments)
	release := make(chan struct{})
	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	calls := 0

	blockReader := queryHandlerFunc(func(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		mu.Lock()
		inFlight++
		calls++
		maxInFlight = max(maxInFlight, inFlight)
		mu.Unlock()
		started <- struct{}{}

		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		mu.Lock()
		inFlight--
		mu.Unlock()
		return &queryv1.InvokeResponse{
			Reports: []*queryv1.Report{{
				ReportType: queryv1.ReportType_REPORT_LABEL_NAMES,
				LabelNames: &queryv1.LabelNamesReport{
					Query:      req.Query[0].LabelNames.CloneVT(),
					LabelNames: []string{"service_name"},
				},
			}},
			Diagnostics: &queryv1.Diagnostics{ExecutionNode: &queryv1.ExecutionNode{Stats: &queryv1.ExecutionStats{}}},
		}, nil
	})

	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, fragments).Add(-time.Millisecond)
	q := &QueryBackend{
		blockReader:          blockReader,
		resultCacheBucket:    phlareobjstore.NewBucket(thanobjstore.NewInMemBucket()),
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 1, window: time.Hour},
		resultCacheMetrics:   newResultCacheMetrics(prometheus.NewRegistry()),
		now:                  func() time.Time { return end.Add(48 * time.Hour) },
	}
	req := &queryv1.InvokeRequest{
		Tenant:        []string{"tenant-a"},
		StartTime:     start.UnixMilli(),
		EndTime:       end.UnixMilli(),
		LabelSelector: "{}",
		Query:         []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_LABEL_NAMES, LabelNames: &queryv1.LabelNamesQuery{}}},
		QueryPlan: &queryv1.QueryPlan{Root: &queryv1.QueryNode{Blocks: []*metastorev1.BlockMeta{{
			Id: "block", MinTime: start.UnixMilli(), MaxTime: end.UnixMilli(),
			Datasets: []*metastorev1.Dataset{{MinTime: start.UnixMilli(), MaxTime: end.UnixMilli()}},
		}}}},
	}

	result := make(chan error, 1)
	go func() {
		_, err := q.Invoke(context.Background(), req)
		result <- err
	}()

	for range resultCacheFragmentConcurrency {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for concurrent fragment execution")
		}
	}
	mu.Lock()
	require.Equal(t, resultCacheFragmentConcurrency, maxInFlight)
	mu.Unlock()
	select {
	case <-started:
		t.Fatal("fragment execution exceeded the concurrency limit")
	default:
	}

	close(release)
	require.NoError(t, <-result)
	mu.Lock()
	require.Equal(t, fragments, calls)
	mu.Unlock()
}

func TestResultCacheEligibility(t *testing.T) {
	q := &QueryBackend{
		resultCacheBucket:    phlareobjstore.NewBucket(thanobjstore.NewInMemBucket()),
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 1, window: time.Hour},
	}
	req := &queryv1.InvokeRequest{
		Tenant: []string{"tenant-a"},
		Query:  []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_LABEL_NAMES, LabelNames: &queryv1.LabelNamesQuery{}}},
	}
	require.True(t, q.resultCacheEligible(req))
	req.Query[0] = &queryv1.Query{QueryType: queryv1.QueryType_QUERY_LABEL_VALUES, LabelValues: &queryv1.LabelValuesQuery{LabelName: "service_name"}}
	require.True(t, q.resultCacheEligible(req))
	req.Query[0].LabelValues = nil
	require.False(t, q.resultCacheEligible(req))
	req.Query[0] = &queryv1.Query{QueryType: queryv1.QueryType_QUERY_SERIES_LABELS, SeriesLabels: &queryv1.SeriesLabelsQuery{}}
	require.True(t, q.resultCacheEligible(req))
	req.Query[0].SeriesLabels = nil
	require.False(t, q.resultCacheEligible(req))
	req.Query[0] = &queryv1.Query{QueryType: queryv1.QueryType_QUERY_LABEL_NAMES, LabelNames: &queryv1.LabelNamesQuery{}}
	req.Options = &queryv1.InvokeOptions{CollectDiagnostics: true}
	require.False(t, q.resultCacheEligible(req))
	req.Options = nil
	req.Tenant = append(req.Tenant, "tenant-b")
	require.False(t, q.resultCacheEligible(req))
}

func TestInvokeUncachedEmptyPlanMetadataQuery(t *testing.T) {
	q := &QueryBackend{}
	for _, tc := range []struct {
		name    string
		query   *queryv1.Query
		wantErr bool
	}{
		{
			name:  "label names",
			query: &queryv1.Query{QueryType: queryv1.QueryType_QUERY_LABEL_NAMES, LabelNames: &queryv1.LabelNamesQuery{}},
		},
		{
			name:  "label values",
			query: &queryv1.Query{QueryType: queryv1.QueryType_QUERY_LABEL_VALUES, LabelValues: &queryv1.LabelValuesQuery{LabelName: "service_name"}},
		},
		{
			name:    "time series",
			query:   &queryv1.Query{QueryType: queryv1.QueryType_QUERY_TIME_SERIES, TimeSeries: &queryv1.TimeSeriesQuery{}},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := q.invokeUncached(context.Background(), &queryv1.InvokeRequest{Query: []*queryv1.Query{tc.query}})
			if !tc.wantErr {
				require.NoError(t, err)
				require.Empty(t, resp.Reports)
				return
			}
			require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

func TestFilterResultCacheBlocks(t *testing.T) {
	root := &queryv1.QueryNode{Blocks: []*metastorev1.BlockMeta{{
		Id: "block", MinTime: 0, MaxTime: 100,
		Datasets: []*metastorev1.Dataset{{MinTime: 0, MaxTime: 49}, {MinTime: 50, MaxTime: 100}},
	}}}
	q := &QueryBackend{}
	blocks := q.filterBlocks(root, 50, 75)
	require.Len(t, blocks, 1)
	require.Len(t, blocks[0].Datasets, 1)
	require.Equal(t, int64(50), blocks[0].Datasets[0].MinTime)
	require.Len(t, root.Blocks[0].Datasets, 2)
}
