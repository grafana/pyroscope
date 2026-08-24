package querybackend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/grafana/dskit/tracing"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
)

const (
	resultCacheLabelNames   = "label_names"
	resultCacheLabelValues  = "label_values"
	resultCacheWorkers      = 2
	resultCacheQueueSize    = 128
	resultCacheWriteTimeout = 30 * time.Second
)

type ResultCacheOverrides interface {
	RejectOlderThan(tenantID string) time.Duration
	ResultCacheEnabled(tenantID string) bool
	ResultCacheGeneration(tenantID string) uint32
}

type resultCacheMetrics struct {
	lookups *prometheus.CounterVec
	writes  *prometheus.CounterVec
}

func newResultCacheMetrics(reg prometheus.Registerer) *resultCacheMetrics {
	m := &resultCacheMetrics{
		lookups: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope", Subsystem: "query_backend", Name: "result_cache_lookups_total",
		}, []string{"query_type", "outcome"}),
		writes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope", Subsystem: "query_backend", Name: "result_cache_writes_total",
		}, []string{"query_type", "outcome"}),
	}
	if reg != nil {
		reg.MustRegister(m.lookups, m.writes)
	}
	return m
}

type resultCacheWriteJob struct {
	queryType string
	key       string
	query     *queryv1.QueryRequest
	reports   []*queryv1.Report
}

type resultCacheFragment struct {
	start int64
	end   int64
	full  bool
}

func splitResultCacheFragments(start, end int64) []resultCacheFragment {
	if start > end {
		return nil
	}
	fragments := make([]resultCacheFragment, 0, 1)
	for current := start; current <= end; {
		dayStart := time.UnixMilli(current).UTC().Truncate(24 * time.Hour)
		dayEnd := dayStart.Add(24*time.Hour).UnixMilli() - 1
		fragmentEnd := min(end, dayEnd)
		fragments = append(fragments, resultCacheFragment{
			start: current,
			end:   fragmentEnd,
			full:  current == dayStart.UnixMilli() && fragmentEnd == dayEnd,
		})
		if fragmentEnd == end {
			break
		}
		current = fragmentEnd + 1
	}
	return fragments
}

func stableResultCacheDay(end int64, now time.Time, window time.Duration) bool {
	if window <= 0 {
		return false
	}
	followingMidnight := time.UnixMilli(end).UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	return !now.Before(followingMidnight.Add(2 * window))
}

func cacheQuery(req *queryv1.InvokeRequest, start, end int64) (*queryv1.QueryRequest, error) {
	selector, err := canonicalResultCacheSelector(req.LabelSelector)
	if err != nil {
		return nil, err
	}
	return &queryv1.QueryRequest{
		StartTime: start, EndTime: end, LabelSelector: selector, Query: req.Query,
	}, nil
}

func canonicalResultCacheSelector(selector string) (string, error) {
	matchers, err := phlaremodel.ParseMetricSelector(selector)
	if err != nil {
		return "", err
	}
	sort.Slice(matchers, func(i, j int) bool {
		return matchers[i].String() < matchers[j].String()
	})
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

func resultCacheKey(tenant string, generation uint32, query *queryv1.QueryRequest) (string, error) {
	hashQuery := query.CloneVT()
	hashQuery.StartTime = 0
	hashQuery.EndTime = 0
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(hashQuery)
	if err != nil {
		return "", fmt.Errorf("marshal cache key query: %w", err)
	}
	digest := sha256.Sum256(b)
	date := time.UnixMilli(query.StartTime).UTC().Format("2006-01-02")
	return fmt.Sprintf("result-cache/%s/%04d-%s-%s", tenant, generation, date, hex.EncodeToString(digest[:])), nil
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
	default:
		return "", false
	}
}

func (q *QueryBackend) coordinateResultCache(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "QueryBackend.ResultCache")
	defer span.Finish()
	queryType, validQuery := resultCacheQueryType(req.Query)
	if validQuery {
		span.SetTag("query_type", queryType)
	}
	if !q.resultCacheEligible(req) {
		span.SetTag("outcome", "bypass")
		return q.executeWithResultCacheBypassed(ctx, req)
	}
	span.SetTag("outcome", "enabled")

	tenant := req.Tenant[0]
	generation := q.resultCacheOverrides.ResultCacheGeneration(tenant)
	selector, err := canonicalResultCacheSelector(req.LabelSelector)
	if err != nil {
		return q.executeWithResultCacheBypassed(ctx, req)
	}
	aggregator := newAggregator(req)
	var bytesFetched uint64
	fragments := splitResultCacheFragments(req.StartTime, req.EndTime)
	span.SetTag("fragments", len(fragments))
	for _, fragment := range fragments {
		fragmentReq := req.CloneVT()
		fragmentReq.StartTime = fragment.start
		fragmentReq.EndTime = fragment.end
		fragmentReq.QueryPlan = queryplan.Build(q.filterBlocks(req.QueryPlan.GetRoot(), fragment.start, fragment.end), 4, 20)
		fragmentReq.Options = fragmentReq.GetOptions().CloneVT()
		if fragmentReq.Options == nil {
			fragmentReq.Options = &queryv1.InvokeOptions{}
		}
		fragmentReq.Options.BypassResultCache = true

		cacheable := fragment.full && stableResultCacheDay(fragment.end, q.now(), q.resultCacheOverrides.RejectOlderThan(tenant))
		query := &queryv1.QueryRequest{
			StartTime: fragment.start, EndTime: fragment.end, LabelSelector: selector, Query: fragmentReq.Query,
		}
		key := ""
		writeAllowed := false
		if cacheable {
			var hit bool
			var err error
			key, err = resultCacheKey(tenant, generation, query)
			if err == nil {
				hit, err = q.readResultCache(ctx, queryType, key, query, aggregator)
			}
			if err == nil && hit {
				continue
			}
			if err == nil {
				writeAllowed = true
			}
		}

		var resp *queryv1.InvokeResponse
		var err error
		if fragmentReq.QueryPlan.GetRoot() == nil {
			resp = &queryv1.InvokeResponse{}
		} else {
			resp, err = q.invokeUncached(ctx, fragmentReq)
		}
		if err != nil {
			return nil, err
		}
		bytesFetched += resp.GetDiagnostics().GetExecutionNode().GetStats().GetBytesFetched()
		if err := aggregator.aggregateResponse(resp, nil); err != nil {
			return nil, err
		}
		if cacheable && writeAllowed && ctx.Err() == nil {
			q.enqueueResultCacheWrite(resultCacheWriteJob{queryType: queryType, key: key, query: query.CloneVT(), reports: cloneReports(resp.Reports)})
		}
	}

	resp := aggregator.response()
	resp.Diagnostics = &queryv1.Diagnostics{ExecutionNode: &queryv1.ExecutionNode{Stats: &queryv1.ExecutionStats{BytesFetched: bytesFetched}}}
	return resp, nil
}

func (q *QueryBackend) executeWithResultCacheBypassed(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	clone := req.CloneVT()
	clone.Options = clone.GetOptions().CloneVT()
	if clone.Options == nil {
		clone.Options = &queryv1.InvokeOptions{}
	}
	clone.Options.BypassResultCache = true
	return q.invokeUncached(ctx, clone)
}

func (q *QueryBackend) resultCacheEligible(req *queryv1.InvokeRequest) bool {
	_, validQuery := resultCacheQueryType(req.Query)
	return q.resultCacheBucket != nil && q.resultCacheOverrides != nil && len(req.Tenant) == 1 &&
		validQuery &&
		!req.GetOptions().GetCollectDiagnostics() && q.resultCacheOverrides.ResultCacheEnabled(req.Tenant[0]) &&
		q.resultCacheOverrides.RejectOlderThan(req.Tenant[0]) > 0
}

// readResultCache returns a cache hit. Its error is non-nil only when a read's
// object state is unknown; corrupt entries are safe to replace after execution.
func (q *QueryBackend) readResultCache(ctx context.Context, queryType, key string, expected *queryv1.QueryRequest, aggregator *reportAggregator) (bool, error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "QueryBackend.ResultCacheLookup")
	defer span.Finish()
	span.SetTag("query_type", queryType)
	span.SetTag("utc_day", time.UnixMilli(expected.StartTime).UTC().Format("2006-01-02"))

	r, err := q.resultCacheBucket.Get(ctx, key)
	if err != nil {
		if q.resultCacheBucket.IsObjNotFoundErr(err) {
			span.SetTag("outcome", "miss")
			q.resultCacheMetrics.lookups.WithLabelValues(queryType, "miss").Inc()
			return false, nil
		}
		span.SetTag("outcome", "error")
		q.resultCacheMetrics.lookups.WithLabelValues(queryType, "error").Inc()
		return false, err
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		span.SetTag("outcome", "error")
		q.resultCacheMetrics.lookups.WithLabelValues(queryType, "error").Inc()
		return false, err
	}
	entry := new(queryv1.ResultCacheEntry)
	if err := proto.Unmarshal(b, entry); err != nil {
		span.SetTag("outcome", "error")
		q.resultCacheMetrics.lookups.WithLabelValues(queryType, "error").Inc()
		return false, nil
	}
	if !proto.Equal(entry.Query, expected) {
		span.SetTag("outcome", "collision")
		q.resultCacheMetrics.lookups.WithLabelValues(queryType, "collision").Inc()
		return false, fmt.Errorf("result cache collision")
	}
	if err := aggregator.aggregateResponse(&queryv1.InvokeResponse{Reports: entry.Reports}, nil); err != nil {
		span.SetTag("outcome", "error")
		return false, err
	}
	span.SetTag("outcome", "hit")
	q.resultCacheMetrics.lookups.WithLabelValues(queryType, "hit").Inc()
	return true, nil
}

func cloneReports(reports []*queryv1.Report) []*queryv1.Report {
	result := make([]*queryv1.Report, len(reports))
	for i, report := range reports {
		result[i] = report.CloneVT()
	}
	return result
}

func (q *QueryBackend) filterBlocks(root *queryv1.QueryNode, start, end int64) []*metastorev1.BlockMeta {
	blocks := make(map[string]*metastorev1.BlockMeta)
	var visit func(*queryv1.QueryNode)
	visit = func(node *queryv1.QueryNode) {
		if node == nil {
			return
		}
		for _, block := range node.Blocks {
			if block.MinTime > end || block.MaxTime < start {
				continue
			}
			clone := block.CloneVT()
			clone.Datasets = clone.Datasets[:0]
			for _, dataset := range block.Datasets {
				if dataset.MinTime <= end && dataset.MaxTime >= start {
					clone.Datasets = append(clone.Datasets, dataset.CloneVT())
				}
			}
			if len(clone.Datasets) > 0 {
				blocks[clone.Id] = clone
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)
	result := make([]*metastorev1.BlockMeta, 0, len(blocks))
	for _, block := range blocks {
		result = append(result, block)
	}
	return result
}

func (q *QueryBackend) enqueueResultCacheWrite(job resultCacheWriteJob) {
	select {
	case q.resultCacheWrites <- job:
	default:
		q.resultCacheMetrics.writes.WithLabelValues(job.queryType, "dropped").Inc()
	}
}

func (q *QueryBackend) runResultCacheWriter(ctx context.Context) {
	defer q.resultCacheWorkers.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-q.resultCacheWrites:
			span, writeCtx := tracing.StartSpanFromContext(ctx, "QueryBackend.ResultCacheWrite")
			span.SetTag("query_type", job.queryType)
			span.SetTag("utc_day", time.UnixMilli(job.query.StartTime).UTC().Format("2006-01-02"))
			entry := &queryv1.ResultCacheEntry{Query: job.query, Reports: job.reports}
			data, err := proto.Marshal(entry)
			if err == nil {
				writeCtx, cancel := context.WithTimeout(writeCtx, resultCacheWriteTimeout)
				err = q.resultCacheBucket.Upload(writeCtx, job.key, bytes.NewReader(data))
				cancel()
			}
			if err != nil {
				span.SetTag("outcome", "error")
				q.resultCacheMetrics.writes.WithLabelValues(job.queryType, "error").Inc()
			} else {
				span.SetTag("outcome", "success")
				q.resultCacheMetrics.writes.WithLabelValues(job.queryType, "success").Inc()
			}
			span.Finish()
		}
	}
}
