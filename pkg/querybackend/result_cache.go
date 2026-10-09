package querybackend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"google.golang.org/protobuf/proto"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
)

const (
	resultCacheLabelNames      = "label_names"
	resultCacheLabelValues     = "label_values"
	resultCacheSeriesLabels    = "series_labels"
	resultCacheWorkers         = 2
	resultCacheQueueSize       = 128
	resultCacheWriteTimeout    = 30 * time.Second
	resultCacheShutdownTimeout = 30 * time.Second
)

type ResultCacheOverrides interface {
	ResultCacheEnabled(tenantID string) bool
	ResultCacheGeneration(tenantID string) uint
}

type resultCache struct {
	resultCacheExecutionDelay time.Duration
	resultCacheBucket         objstore.Bucket
	resultCacheOverrides      ResultCacheOverrides
	resultCacheMetrics        *resultCacheMetrics
	resultCacheWrites         chan resultCacheWriteJob
	resultCacheWorkers        sync.WaitGroup
}

type resultCacheMetrics struct{ lookups, writes *prometheus.CounterVec }

func newResultCacheMetrics(reg prometheus.Registerer) *resultCacheMetrics {
	m := &resultCacheMetrics{
		lookups: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "pyroscope", Subsystem: "query_backend", Name: "result_cache_lookups_total"}, []string{"query_type", "block_level", "outcome"}),
		writes:  prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "pyroscope", Subsystem: "query_backend", Name: "result_cache_writes_total"}, []string{"query_type", "block_level", "outcome"}),
	}
	if reg != nil {
		reg.MustRegister(m.lookups, m.writes)
	}
	return m
}

type resultCacheWriteJob struct {
	queryType, key string
	identity       *queryv1.ResultCacheKey
	reports        []*queryv1.Report
}

func cacheQuery(req *queryv1.InvokeRequest, start, end int64) (*queryv1.QueryRequest, error) {
	selector, err := canonicalResultCacheSelector(req.LabelSelector)
	if err != nil {
		return nil, err
	}
	queries := make([]*queryv1.Query, len(req.Query))
	for i, query := range req.Query {
		if query == nil {
			// Nil queries are kept in place: they marshal deterministically and
			// compare equal via proto.Equal, so the identity stays stable.
			continue
		}
		queries[i] = query.CloneVT()
		if queries[i].SeriesLabels != nil {
			sort.Strings(queries[i].SeriesLabels.LabelNames)
		}
	}
	return &queryv1.QueryRequest{StartTime: start, EndTime: end, LabelSelector: selector, Query: queries}, nil
}
func canonicalResultCacheSelector(selector string) (string, error) {
	matchers, err := phlaremodel.ParseMetricSelector(selector)
	if err != nil {
		return "", err
	}
	sort.Slice(matchers, func(i, j int) bool { return matchers[i].String() < matchers[j].String() })
	var canonical strings.Builder
	canonical.WriteByte('{')
	for i, matcher := range matchers {
		if i > 0 {
			canonical.WriteByte(',')
		}
		canonical.WriteString(matcher.String())
	}
	canonical.WriteByte('}')
	return canonical.String(), nil
}
func resultCacheQueryType(queries []*queryv1.Query) (string, bool) {
	if len(queries) != 1 || queries[0] == nil {
		return "", false
	}
	switch query := queries[0]; query.QueryType {
	case queryv1.QueryType_QUERY_LABEL_NAMES:
		return resultCacheLabelNames, query.LabelNames != nil
	case queryv1.QueryType_QUERY_LABEL_VALUES:
		return resultCacheLabelValues, query.LabelValues != nil
	case queryv1.QueryType_QUERY_SERIES_LABELS:
		return resultCacheSeriesLabels, query.SeriesLabels != nil
	default:
		return "", false
	}
}
func (q *resultCache) resultCacheEligible(req *queryv1.InvokeRequest) bool {
	_, valid := resultCacheQueryType(req.Query)
	if q == nil || q.resultCacheBucket == nil || q.resultCacheOverrides == nil || len(req.Tenant) != 1 || !valid ||
		req.GetOptions().GetCollectDiagnostics() || !q.resultCacheOverrides.ResultCacheEnabled(req.Tenant[0]) {
		return false
	}
	matchers, err := phlaremodel.ParseMetricSelector(req.LabelSelector)
	if err != nil {
		return false
	}
	for _, matcher := range matchers {
		if matcher.Name == phlaremodel.LabelNameServiceName &&
			(matcher.Type == labels.MatchEqual || (matcher.Type == labels.MatchRegexp && len(matcher.SetMatches()) == 1)) {
			// Single-service metadata queries are cheap enough to execute directly.
			return false
		}
	}
	return true
}
func (q *resultCache) readResultCache(ctx context.Context, queryType, key string, expected *queryv1.ResultCacheKey, aggregator *reportAggregator) (bool, error) {
	outcome := "error"
	defer func() { q.resultCacheMetrics.lookups.WithLabelValues(queryType, "L2", outcome).Inc() }()
	r, err := q.resultCacheBucket.Get(ctx, key)
	if err != nil {
		if q.resultCacheBucket.IsObjNotFoundErr(err) {
			outcome = "miss"
			return false, nil
		}
		// Cancellation is expected whenever block execution wins the race:
		// it must not pollute the error rate.
		if errors.Is(err, context.Canceled) {
			outcome = "canceled"
			return false, err
		}
		return false, err
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			outcome = "canceled"
		}
		return false, err
	}
	entry := new(queryv1.ResultCacheEntry)
	if err := proto.Unmarshal(b, entry); err != nil {
		return false, err
	}
	if !proto.Equal(entry.Key, expected) {
		outcome = "collision"
		return false, fmt.Errorf("result cache collision")
	}
	if err := aggregator.aggregateResponse(&queryv1.InvokeResponse{Reports: entry.Reports}); err != nil {
		return false, err
	}
	outcome = "hit"
	return true, nil
}
func cloneReports(reports []*queryv1.Report) []*queryv1.Report {
	result := make([]*queryv1.Report, len(reports))
	for i, report := range reports {
		result[i] = report.CloneVT()
	}
	return result
}
func (q *resultCache) enqueueResultCacheWrite(job resultCacheWriteJob) {
	select {
	case q.resultCacheWrites <- job:
	default:
		q.resultCacheMetrics.writes.WithLabelValues(job.queryType, "L2", "dropped").Inc()
	}
}
func (q *resultCache) runResultCacheWriter(ctx, writeCtx context.Context) {
	defer q.resultCacheWorkers.Done()
	for writeCtx.Err() == nil {
		select {
		case <-ctx.Done():
			// Keep draining until the queue is empty or the shared shutdown
			// budget expires. writeCtx also cancels in-flight uploads.
			for writeCtx.Err() == nil {
				select {
				case job := <-q.resultCacheWrites:
					q.uploadResultCacheEntry(writeCtx, job)
				default:
					return
				}
			}
			return
		case <-writeCtx.Done():
			return
		case job := <-q.resultCacheWrites:
			q.uploadResultCacheEntry(writeCtx, job)
		}
	}
}

func (q *resultCache) dropResultCacheWrites() {
	for {
		select {
		case job := <-q.resultCacheWrites:
			q.resultCacheMetrics.writes.WithLabelValues(job.queryType, "L2", "dropped").Inc()
		default:
			return
		}
	}
}

func (q *resultCache) uploadResultCacheEntry(ctx context.Context, job resultCacheWriteJob) {
	entry := &queryv1.ResultCacheEntry{Key: job.identity, Reports: job.reports}
	data, err := proto.Marshal(entry)
	if err == nil {
		writeCtx, cancel := context.WithTimeout(ctx, resultCacheWriteTimeout)
		err = q.resultCacheBucket.Upload(writeCtx, job.key, bytes.NewReader(data))
		cancel()
	}
	outcome := "success"
	if err != nil {
		outcome = "error"
	}
	q.resultCacheMetrics.writes.WithLabelValues(job.queryType, "L2", outcome).Inc()
}
