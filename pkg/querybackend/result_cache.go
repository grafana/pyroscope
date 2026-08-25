package querybackend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/dskit/tracing"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
)

const (
	resultCacheLabelNames          = "label_names"
	resultCacheLabelValues         = "label_values"
	resultCacheSeriesLabels        = "series_labels"
	resultCacheWorkers             = 2
	resultCacheQueueSize           = 128
	resultCacheWriteTimeout        = 30 * time.Second
	resultCacheFragmentConcurrency = 64
)

type ResultCacheOverrides interface {
	ResultCacheEnabled(tenantID string) bool
	ResultCacheGeneration(tenantID string) uint32
	ResultCacheFragmentDurations(tenantID string) []time.Duration
	ResultCacheMetadataServiceNameMinQueryDuration(tenantID string) time.Duration
}

type resultCacheMetrics struct {
	lookups *prometheus.CounterVec
	writes  *prometheus.CounterVec
}

func newResultCacheMetrics(reg prometheus.Registerer) *resultCacheMetrics {
	m := &resultCacheMetrics{
		lookups: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope", Subsystem: "query_backend", Name: "result_cache_lookups_total",
		}, []string{"query_type", "fragment_duration", "outcome"}),
		writes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope", Subsystem: "query_backend", Name: "result_cache_writes_total",
		}, []string{"query_type", "fragment_duration", "outcome"}),
	}
	if reg != nil {
		reg.MustRegister(m.lookups, m.writes)
	}
	return m
}

type resultCacheWriteJob struct {
	queryType string
	duration  string
	key       string
	identity  *queryv1.ResultCacheKey
	reports   []*queryv1.Report
}

type resultCacheFragment struct {
	start    int64
	end      int64
	duration time.Duration
}

func splitResultCacheFragments(start, end int64, durations []time.Duration) []resultCacheFragment {
	if start > end {
		return nil
	}
	durations = append([]time.Duration(nil), durations...)
	sort.Slice(durations, func(i, j int) bool { return durations[i] > durations[j] })
	fragments := make([]resultCacheFragment, 0, 1)
	endExclusive := end + 1
	for current := start; current < endExclusive; {
		var selected time.Duration
		for _, duration := range durations {
			milliseconds := duration.Milliseconds()
			if current%milliseconds == 0 && current+milliseconds <= endExclusive {
				selected = duration
				break
			}
		}

		fragmentEnd := end
		if selected > 0 {
			fragmentEnd = current + selected.Milliseconds() - 1
		} else if len(durations) > 0 {
			smallest := durations[len(durations)-1].Milliseconds()
			remainder := current % smallest
			if remainder < 0 {
				remainder += smallest
			}
			nextBoundary := current + smallest - remainder
			fragmentEnd = min(end, nextBoundary-1)
		}
		fragments = append(fragments, resultCacheFragment{start: current, end: fragmentEnd, duration: selected})
		if fragmentEnd == end {
			break
		}
		current = fragmentEnd + 1
	}
	return fragments
}

func resultCacheDurationName(duration time.Duration) string {
	if duration <= 0 {
		return ""
	}
	if duration%time.Hour == 0 {
		return strconv.FormatInt(int64(duration/time.Hour), 10) + "h"
	}
	if duration%time.Minute == 0 {
		return strconv.FormatInt(int64(duration/time.Minute), 10) + "m"
	}
	if duration%time.Second == 0 {
		return strconv.FormatInt(int64(duration/time.Second), 10) + "s"
	}
	return duration.String()
}

func resultCacheIdentity(query *queryv1.QueryRequest, blocks []*metastorev1.BlockMeta) *queryv1.ResultCacheKey {
	blockIDs := make([]string, len(blocks))
	for i, block := range blocks {
		blockIDs[i] = block.Id
	}
	sort.Strings(blockIDs)
	return &queryv1.ResultCacheKey{Query: query, BlockIds: blockIDs}
}

func cacheQuery(req *queryv1.InvokeRequest, start, end int64) (*queryv1.QueryRequest, error) {
	selector, err := canonicalResultCacheSelector(req.LabelSelector)
	if err != nil {
		return nil, err
	}
	queries := cloneQueries(req.Query)
	normalizeResultCacheQueries(queries)
	return &queryv1.QueryRequest{
		StartTime: start, EndTime: end, LabelSelector: selector, Query: queries,
	}, nil
}

func cloneQueries(queries []*queryv1.Query) []*queryv1.Query {
	clones := make([]*queryv1.Query, len(queries))
	for i, query := range queries {
		if query != nil {
			clones[i] = query.CloneVT()
		}
	}
	return clones
}

func normalizeResultCacheQueries(queries []*queryv1.Query) {
	for _, query := range queries {
		if query != nil && query.SeriesLabels != nil {
			sort.Strings(query.SeriesLabels.LabelNames)
		}
	}
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

func resultCacheKey(tenant string, generation uint32, duration time.Duration, identity *queryv1.ResultCacheKey) (string, error) {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("marshal result cache identity: %w", err)
	}
	digest := sha256.Sum256(b)
	return fmt.Sprintf("results-cache/%s/%s/%04d-%s", resultCacheDurationName(duration), tenant, generation, hex.EncodeToString(digest[:])), nil
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
	durations := q.resultCacheOverrides.ResultCacheFragmentDurations(tenant)
	selector, err := canonicalResultCacheSelector(req.LabelSelector)
	if err != nil {
		return q.executeWithResultCacheBypassed(ctx, req)
	}
	aggregator := newAggregator(req)
	fragments := splitResultCacheFragments(req.StartTime, req.EndTime, durations)
	span.SetTag("fragments", len(fragments))
	fragmentResponses := make([]*queryv1.InvokeResponse, len(fragments))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(resultCacheFragmentConcurrency)
	for i, fragment := range fragments {
		idx := i
		fragment := fragment
		g.Go(func() error {
			fragmentReq := req.CloneVT()
			fragmentReq.StartTime = fragment.start
			fragmentReq.EndTime = fragment.end
			normalizeResultCacheQueries(fragmentReq.Query)
			blocks := q.filterBlocks(req.QueryPlan.GetRoot(), fragment.start, fragment.end)
			fragmentReq.QueryPlan = queryplan.Build(blocks, 4, 20)
			fragmentReq.Options = fragmentReq.GetOptions().CloneVT()
			if fragmentReq.Options == nil {
				fragmentReq.Options = &queryv1.InvokeOptions{}
			}
			fragmentReq.Options.BypassResultCache = true

			cacheable := fragment.duration > 0 && fragment.end <= q.now().Add(-durations[len(durations)-1]).UnixMilli()
			query := &queryv1.QueryRequest{
				StartTime: fragment.start, EndTime: fragment.end, LabelSelector: selector, Query: fragmentReq.Query,
			}
			identity := resultCacheIdentity(query, blocks)
			durationName := resultCacheDurationName(fragment.duration)
			key := ""
			writeAllowed := false
			if cacheable {
				fragmentAggregator := newAggregator(fragmentReq)
				var hit bool
				var err error
				key, err = resultCacheKey(tenant, generation, fragment.duration, identity)
				if err == nil {
					hit, err = q.readResultCache(ctx, queryType, durationName, key, identity, fragmentAggregator)
				}
				if err == nil && hit {
					fragmentResponses[idx] = fragmentAggregator.response()
					return nil
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
				return err
			}
			if cacheable && writeAllowed && ctx.Err() == nil {
				q.enqueueResultCacheWrite(resultCacheWriteJob{queryType: queryType, duration: durationName, key: key, identity: identity.CloneVT(), reports: cloneReports(resp.Reports)})
			}
			fragmentResponses[idx] = resp
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var bytesFetched uint64
	for _, resp := range fragmentResponses {
		bytesFetched += resp.GetDiagnostics().GetExecutionNode().GetStats().GetBytesFetched()
		if err := aggregator.aggregateResponse(resp); err != nil {
			return nil, err
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
		len(q.resultCacheOverrides.ResultCacheFragmentDurations(req.Tenant[0])) > 0 &&
		!resultCacheMetadataServiceNameShortRange(req, q.resultCacheOverrides.ResultCacheMetadataServiceNameMinQueryDuration(req.Tenant[0]))
}

func resultCacheMetadataServiceNameShortRange(req *queryv1.InvokeRequest, minQueryDuration time.Duration) bool {
	if minQueryDuration <= 0 || req.EndTime-req.StartTime >= minQueryDuration.Milliseconds() {
		return false
	}
	matchers, err := phlaremodel.ParseMetricSelector(req.LabelSelector)
	if err != nil {
		return false
	}
	for _, matcher := range matchers {
		if matcher.Name == "service_name" {
			return true
		}
	}
	return false
}

// readResultCache returns a cache hit. Its error is non-nil only when a read's
// object state is unknown; corrupt entries are safe to replace after execution.
func (q *QueryBackend) readResultCache(ctx context.Context, queryType, duration, key string, expected *queryv1.ResultCacheKey, aggregator *reportAggregator) (bool, error) {
	span, ctx := tracing.StartSpanFromContext(ctx, "QueryBackend.ResultCacheLookup")
	defer span.Finish()
	span.SetTag("query_type", queryType)
	span.SetTag("fragment_duration", duration)
	span.SetTag("fragment_start", time.UnixMilli(expected.GetQuery().GetStartTime()).UTC().Format(time.RFC3339))

	r, err := q.resultCacheBucket.Get(ctx, key)
	if err != nil {
		if q.resultCacheBucket.IsObjNotFoundErr(err) {
			span.SetTag("outcome", "miss")
			q.resultCacheMetrics.lookups.WithLabelValues(queryType, duration, "miss").Inc()
			return false, nil
		}
		span.SetTag("outcome", "error")
		q.resultCacheMetrics.lookups.WithLabelValues(queryType, duration, "error").Inc()
		return false, err
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		span.SetTag("outcome", "error")
		q.resultCacheMetrics.lookups.WithLabelValues(queryType, duration, "error").Inc()
		return false, err
	}
	entry := new(queryv1.ResultCacheEntry)
	if err := proto.Unmarshal(b, entry); err != nil {
		span.SetTag("outcome", "error")
		q.resultCacheMetrics.lookups.WithLabelValues(queryType, duration, "error").Inc()
		return false, nil
	}
	if !proto.Equal(entry.Key, expected) {
		span.SetTag("outcome", "collision")
		q.resultCacheMetrics.lookups.WithLabelValues(queryType, duration, "collision").Inc()
		return false, fmt.Errorf("result cache collision")
	}
	if err := aggregator.aggregateResponse(&queryv1.InvokeResponse{Reports: entry.Reports}); err != nil {
		span.SetTag("outcome", "error")
		return false, err
	}
	span.SetTag("outcome", "hit")
	q.resultCacheMetrics.lookups.WithLabelValues(queryType, duration, "hit").Inc()
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
	sort.Slice(result, func(i, j int) bool { return result[i].Id < result[j].Id })
	return result
}

func (q *QueryBackend) enqueueResultCacheWrite(job resultCacheWriteJob) {
	select {
	case q.resultCacheWrites <- job:
	default:
		q.resultCacheMetrics.writes.WithLabelValues(job.queryType, job.duration, "dropped").Inc()
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
			span.SetTag("fragment_duration", job.duration)
			span.SetTag("fragment_start", time.UnixMilli(job.identity.GetQuery().GetStartTime()).UTC().Format(time.RFC3339))
			entry := &queryv1.ResultCacheEntry{Key: job.identity, Reports: job.reports}
			data, err := proto.Marshal(entry)
			if err == nil {
				writeCtx, cancel := context.WithTimeout(writeCtx, resultCacheWriteTimeout)
				err = q.resultCacheBucket.Upload(writeCtx, job.key, bytes.NewReader(data))
				cancel()
			}
			if err != nil {
				span.SetTag("outcome", "error")
				q.resultCacheMetrics.writes.WithLabelValues(job.queryType, job.duration, "error").Inc()
			} else {
				span.SetTag("outcome", "success")
				q.resultCacheMetrics.writes.WithLabelValues(job.queryType, job.duration, "success").Inc()
			}
			span.Finish()
		}
	}
}
