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
	durations  []time.Duration
}

func (o resultCacheOverrides) ResultCacheEnabled(string) bool      { return o.enabled }
func (o resultCacheOverrides) ResultCacheGeneration(string) uint32 { return o.generation }
func (o resultCacheOverrides) ResultCacheFragmentDurations(string) []time.Duration {
	if o.durations == nil {
		return []time.Duration{24 * time.Hour, 2 * time.Hour, 15 * time.Minute}
	}
	return o.durations
}

type queryHandlerFunc func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)

func (f queryHandlerFunc) Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	return f(ctx, req)
}

func TestSplitResultCacheFragments(t *testing.T) {
	start := time.Date(2026, 8, 20, 1, 45, 0, 0, time.UTC)
	end := time.Date(2026, 8, 20, 4, 14, 59, 999000000, time.UTC)

	fragments := splitResultCacheFragments(start.UnixMilli(), end.UnixMilli(), []time.Duration{2 * time.Hour, 15 * time.Minute})
	require.Len(t, fragments, 3)
	require.Equal(t, 15*time.Minute, fragments[0].duration)
	require.Equal(t, 2*time.Hour, fragments[1].duration)
	require.Equal(t, 15*time.Minute, fragments[2].duration)
	require.Equal(t, start.UnixMilli(), fragments[0].start)
	require.Equal(t, time.Date(2026, 8, 20, 2, 0, 0, 0, time.UTC).UnixMilli(), fragments[1].start)
	require.Equal(t, end.UnixMilli(), fragments[2].end)

	unaligned := splitResultCacheFragments(start.Add(2*time.Minute).UnixMilli(), end.Add(-2*time.Minute).UnixMilli(), []time.Duration{2 * time.Hour, 15 * time.Minute})
	require.Len(t, unaligned, 3)
	require.Zero(t, unaligned[0].duration)
	require.Equal(t, 2*time.Hour, unaligned[1].duration)
	require.Zero(t, unaligned[2].duration)

	beforeEpoch := splitResultCacheFragments(-time.Millisecond.Milliseconds(), (15*time.Minute - time.Millisecond).Milliseconds(), []time.Duration{15 * time.Minute})
	require.Len(t, beforeEpoch, 2)
	require.Equal(t, int64(-1), beforeEpoch[0].start)
	require.Equal(t, int64(-1), beforeEpoch[0].end)
	require.Zero(t, beforeEpoch[0].duration)
	require.Equal(t, int64(0), beforeEpoch[1].start)
	require.Equal(t, 15*time.Minute, beforeEpoch[1].duration)
}

func TestResultCacheKeyIncludesTimeAndBlocks(t *testing.T) {
	query := &queryv1.QueryRequest{
		StartTime:     time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC).UnixMilli(),
		EndTime:       time.Date(2026, 8, 21, 23, 59, 59, 999000000, time.UTC).UnixMilli(),
		LabelSelector: `{service_name="api"}`,
		Query:         []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_LABEL_NAMES, LabelNames: &queryv1.LabelNamesQuery{}}},
	}
	identity := resultCacheIdentity(query, []*metastorev1.BlockMeta{{Id: "block-b"}, {Id: "block-a"}})
	key, err := resultCacheKey("tenant-a", 1, 24*time.Hour, identity)
	require.NoError(t, err)
	require.Equal(t, []string{"block-a", "block-b"}, identity.BlockIds)
	require.Contains(t, key, "result-cache/tenant-a/24h/0001-")

	otherBlocksKey, err := resultCacheKey("tenant-a", 1, 24*time.Hour, resultCacheIdentity(query, []*metastorev1.BlockMeta{{Id: "block-a"}, {Id: "block-c"}}))
	require.NoError(t, err)
	require.NotEqual(t, key, otherBlocksKey)

	otherDurationKey, err := resultCacheKey("tenant-a", 1, 2*time.Hour, identity)
	require.NoError(t, err)
	require.Contains(t, otherDurationKey, "result-cache/tenant-a/2h/0001-")
	require.NotEqual(t, key, otherDurationKey)

	otherDay := query.CloneVT()
	otherDay.StartTime += 24 * 60 * 60 * 1000
	otherDay.EndTime += 24 * 60 * 60 * 1000
	otherDayKey, err := resultCacheKey("tenant-a", 1, 24*time.Hour, resultCacheIdentity(otherDay, []*metastorev1.BlockMeta{{Id: "block-b"}, {Id: "block-a"}}))
	require.NoError(t, err)
	require.NotEqual(t, key, otherDayKey)

	query.StartTime += 24 * 60 * 60 * 1000
	query.EndTime += 24 * 60 * 60 * 1000

	labelValues := query.CloneVT()
	labelValues.Query[0] = &queryv1.Query{
		QueryType:   queryv1.QueryType_QUERY_LABEL_VALUES,
		LabelValues: &queryv1.LabelValuesQuery{LabelName: "service_name"},
	}
	labelValuesKey, err := resultCacheKey("tenant-a", 1, 24*time.Hour, resultCacheIdentity(labelValues, nil))
	require.NoError(t, err)
	labelValues.Query[0].LabelValues.LabelName = "cluster"
	otherLabelValuesKey, err := resultCacheKey("tenant-a", 1, 24*time.Hour, resultCacheIdentity(labelValues, nil))
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
	seriesLabelsKey, err := resultCacheKey("tenant-a", 1, 24*time.Hour, resultCacheIdentity(seriesLabelsQuery, nil))
	require.NoError(t, err)
	otherSeriesLabelsKey, err := resultCacheKey("tenant-a", 1, 24*time.Hour, resultCacheIdentity(otherSeriesLabelsQuery, nil))
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
	expected := &queryv1.ResultCacheKey{Query: &queryv1.QueryRequest{StartTime: 1, EndTime: 2, Query: request.Query}, BlockIds: []string{"block-a"}}
	entry := &queryv1.ResultCacheEntry{Key: expected.CloneVT(), Reports: []*queryv1.Report{{
		ReportType: queryv1.ReportType_REPORT_LABEL_NAMES,
		LabelNames: &queryv1.LabelNamesReport{Query: &queryv1.LabelNamesQuery{}, LabelNames: []string{"cluster", "service_name"}},
	}}}
	data, err := proto.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, bucket.Upload(context.Background(), "entry", bytes.NewReader(data)))

	aggregator := newAggregator(request)
	hit, err := q.readResultCache(context.Background(), resultCacheLabelNames, "24h", "entry", expected, aggregator)
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, []string{"cluster", "service_name"}, aggregator.response().Reports[0].LabelNames.LabelNames)

	entry.Key.BlockIds[0] = "block-b"
	data, err = proto.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, bucket.Upload(context.Background(), "collision", bytes.NewReader(data)))
	hit, err = q.readResultCache(context.Background(), resultCacheLabelNames, "24h", "collision", expected, newAggregator(request))
	require.Error(t, err)
	require.False(t, hit)
}

func TestCoordinateResultCacheHitDoesNotExecutePlan(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanobjstore.NewInMemBucket())
	q := &QueryBackend{
		resultCacheBucket:    bucket,
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 7},
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
	identity := resultCacheIdentity(query, nil)
	key, err := resultCacheKey("tenant-a", 7, 24*time.Hour, identity)
	require.NoError(t, err)
	data, err := proto.Marshal(&queryv1.ResultCacheEntry{Key: identity, Reports: []*queryv1.Report{{
		ReportType: queryv1.ReportType_REPORT_LABEL_NAMES,
		LabelNames: &queryv1.LabelNamesReport{Query: &queryv1.LabelNamesQuery{}, LabelNames: []string{"cluster"}},
	}}})
	require.NoError(t, err)
	require.NoError(t, bucket.Upload(context.Background(), key, bytes.NewReader(data)))

	resp, err := q.Invoke(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"cluster"}, resp.Reports[0].LabelNames.LabelNames)
	require.Zero(t, resp.Diagnostics.ExecutionNode.Stats.BytesFetched)
	require.Equal(t, float64(1), promtest.ToFloat64(q.resultCacheMetrics.lookups.WithLabelValues(resultCacheLabelNames, "24h", "hit")))
}

func TestCoordinateResultCacheLabelValuesHitDoesNotExecutePlan(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanobjstore.NewInMemBucket())
	q := &QueryBackend{
		resultCacheBucket:    bucket,
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 7},
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
	identity := resultCacheIdentity(query, nil)
	key, err := resultCacheKey("tenant-a", 7, 24*time.Hour, identity)
	require.NoError(t, err)
	data, err := proto.Marshal(&queryv1.ResultCacheEntry{Key: identity, Reports: []*queryv1.Report{{
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
	require.Equal(t, float64(1), promtest.ToFloat64(q.resultCacheMetrics.lookups.WithLabelValues(resultCacheLabelValues, "24h", "hit")))
}

func TestCoordinateResultCacheSeriesLabelsHitDoesNotExecutePlan(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanobjstore.NewInMemBucket())
	q := &QueryBackend{
		resultCacheBucket:    bucket,
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 7},
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
	identity := resultCacheIdentity(query, nil)
	key, err := resultCacheKey("tenant-a", 7, 24*time.Hour, identity)
	require.NoError(t, err)
	data, err := proto.Marshal(&queryv1.ResultCacheEntry{Key: identity, Reports: []*queryv1.Report{{
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
	require.Equal(t, float64(1), promtest.ToFloat64(q.resultCacheMetrics.lookups.WithLabelValues(resultCacheSeriesLabels, "24h", "hit")))
}

func TestCoordinateResultCacheDoesNotCacheSmallestRecentFragment(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanobjstore.NewInMemBucket())
	start := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	end := start.Add(30*time.Minute - time.Millisecond)
	blockReader := queryHandlerFunc(func(_ context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		return &queryv1.InvokeResponse{
			Reports: []*queryv1.Report{{
				ReportType: queryv1.ReportType_REPORT_LABEL_NAMES,
				LabelNames: &queryv1.LabelNamesReport{Query: req.Query[0].LabelNames.CloneVT(), LabelNames: []string{"service_name"}},
			}},
			Diagnostics: &queryv1.Diagnostics{ExecutionNode: &queryv1.ExecutionNode{Stats: &queryv1.ExecutionStats{}}},
		}, nil
	})
	q := &QueryBackend{
		blockReader:          blockReader,
		resultCacheBucket:    bucket,
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 1, durations: []time.Duration{15 * time.Minute}},
		resultCacheMetrics:   newResultCacheMetrics(prometheus.NewRegistry()),
		now:                  func() time.Time { return end.Add(time.Millisecond) },
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

	resp, err := q.Invoke(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"service_name"}, resp.Reports[0].LabelNames.LabelNames)
	require.Equal(t, float64(1), promtest.ToFloat64(q.resultCacheMetrics.lookups.WithLabelValues(resultCacheLabelNames, "15m", "miss")))
	require.Equal(t, float64(1), promtest.ToFloat64(q.resultCacheMetrics.writes.WithLabelValues(resultCacheLabelNames, "15m", "dropped")))
}

func TestCoordinateResultCacheBlockSetInvalidatesEntry(t *testing.T) {
	bucket := phlareobjstore.NewBucket(thanobjstore.NewInMemBucket())
	start := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	end := start.Add(15*time.Minute - time.Millisecond)
	block := func(id string) *metastorev1.BlockMeta {
		return &metastorev1.BlockMeta{
			Id: id, MinTime: start.UnixMilli(), MaxTime: end.UnixMilli(),
			Datasets: []*metastorev1.Dataset{{MinTime: start.UnixMilli(), MaxTime: end.UnixMilli()}},
		}
	}
	query := &queryv1.QueryRequest{
		StartTime: start.UnixMilli(), EndTime: end.UnixMilli(), LabelSelector: "{}",
		Query: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_LABEL_NAMES, LabelNames: &queryv1.LabelNamesQuery{}}},
	}
	identity := resultCacheIdentity(query, []*metastorev1.BlockMeta{block("block-a")})
	key, err := resultCacheKey("tenant-a", 1, 15*time.Minute, identity)
	require.NoError(t, err)
	data, err := proto.Marshal(&queryv1.ResultCacheEntry{Key: identity, Reports: []*queryv1.Report{{
		ReportType: queryv1.ReportType_REPORT_LABEL_NAMES,
		LabelNames: &queryv1.LabelNamesReport{Query: &queryv1.LabelNamesQuery{}, LabelNames: []string{"cached"}},
	}}})
	require.NoError(t, err)
	require.NoError(t, bucket.Upload(context.Background(), key, bytes.NewReader(data)))

	calls := 0
	q := &QueryBackend{
		blockReader: queryHandlerFunc(func(_ context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
			calls++
			return &queryv1.InvokeResponse{
				Reports: []*queryv1.Report{{
					ReportType: queryv1.ReportType_REPORT_LABEL_NAMES,
					LabelNames: &queryv1.LabelNamesReport{Query: req.Query[0].LabelNames.CloneVT(), LabelNames: []string{"fresh"}},
				}},
				Diagnostics: &queryv1.Diagnostics{ExecutionNode: &queryv1.ExecutionNode{Stats: &queryv1.ExecutionStats{}}},
			}, nil
		}),
		resultCacheBucket:    bucket,
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 1, durations: []time.Duration{15 * time.Minute}},
		resultCacheMetrics:   newResultCacheMetrics(prometheus.NewRegistry()),
		now:                  func() time.Time { return end.Add(30 * time.Minute) },
	}
	req := &queryv1.InvokeRequest{
		Tenant: []string{"tenant-a"}, StartTime: start.UnixMilli(), EndTime: end.UnixMilli(), LabelSelector: "{}",
		Query:     query.Query,
		QueryPlan: &queryv1.QueryPlan{Root: &queryv1.QueryNode{Blocks: []*metastorev1.BlockMeta{block("block-a")}}},
	}

	resp, err := q.Invoke(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"cached"}, resp.Reports[0].LabelNames.LabelNames)
	require.Zero(t, calls)

	req.QueryPlan.Root.Blocks = append(req.QueryPlan.Root.Blocks, block("block-b"))
	resp, err = q.Invoke(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"fresh"}, resp.Reports[0].LabelNames.LabelNames)
	require.Equal(t, 1, calls)
	require.Equal(t, float64(1), promtest.ToFloat64(q.resultCacheMetrics.lookups.WithLabelValues(resultCacheLabelNames, "15m", "hit")))
	require.Equal(t, float64(1), promtest.ToFloat64(q.resultCacheMetrics.lookups.WithLabelValues(resultCacheLabelNames, "15m", "miss")))
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
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 1, durations: []time.Duration{24 * time.Hour}},
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
		resultCacheOverrides: resultCacheOverrides{enabled: true, generation: 1},
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
