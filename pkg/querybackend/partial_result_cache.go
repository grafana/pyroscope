package querybackend

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tracing"
	"github.com/klauspost/compress/zstd"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/internal/pushback"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

const (
	partialResultCacheVersion = 1
	partialResultCachePrefix  = "results-cache/partial/v1/"
	// Queued and in-flight writes; each may hold up to MaxSizeBytes of trees.
	partialResultCacheMaxQueued = 4
	maxFunctionTreeDepth        = 128

	partialResultCorrupt = "corrupt"
)

type PartialResultCacheConfig struct {
	MaxSizeBytes   int           `yaml:"max_size_bytes" category:"experimental"`
	ExecutionDelay time.Duration `yaml:"execution_delay" category:"experimental"`
}

func (cfg *PartialResultCacheConfig) RegisterFlags(f *flag.FlagSet) {
	f.IntVar(&cfg.MaxSizeBytes, "query-backend.result-cache.partial-results.max-size-bytes", 32<<20, "Maximum uncompressed size of the function stack trees collected for a flame graph query. Larger results are not cached.")
	f.DurationVar(&cfg.ExecutionDelay, "query-backend.result-cache.partial-results.execution-delay", 100*time.Millisecond, "Delay before executing a function projection query while its partial result cache lookup has not found an entry. A miss starts execution immediately.")
}

func (cfg *PartialResultCacheConfig) Validate() error {
	if cfg.MaxSizeBytes < 0 {
		return fmt.Errorf("query-backend.result-cache.partial-results.max-size-bytes must not be negative")
	}
	if cfg.ExecutionDelay < 0 {
		return fmt.Errorf("query-backend.result-cache.partial-results.execution-delay must not be negative")
	}
	return nil
}

type partialResultCache struct {
	*resultCache
	logger  log.Logger
	config  PartialResultCacheConfig
	metrics *partialResultCacheMetrics
	queued  atomic.Int32
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

type partialResultCacheMetrics struct {
	lookups, writes, skips, bytes *prometheus.CounterVec
	collected                     prometheus.Counter
	entrySize                     prometheus.Histogram
}

func newPartialResultCacheMetrics(reg prometheus.Registerer) *partialResultCacheMetrics {
	m := &partialResultCacheMetrics{
		lookups: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope", Subsystem: "query_backend", Name: "partial_result_cache_lookups_total",
			Help: "Partial result cache lookups for function projection queries.",
		}, []string{"query_type", "outcome"}),
		writes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope", Subsystem: "query_backend", Name: "partial_result_cache_writes_total",
			Help: "Partial result cache entry uploads.",
		}, []string{"outcome"}),
		skips: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope", Subsystem: "query_backend", Name: "partial_result_cache_write_skips_total",
			Help: "Eligible flame graph queries whose partial result was not cached.",
		}, []string{"reason"}),
		bytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "pyroscope", Subsystem: "query_backend", Name: "partial_result_cache_bytes_total",
			Help: "Compressed partial result cache entry bytes read and written.",
		}, []string{"op"}),
		collected: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "pyroscope", Subsystem: "query_backend", Name: "partial_result_cache_collected_bytes_total",
			Help: "Uncompressed function stack tree bytes collected by read nodes for the partial result cache.",
		}),
		entrySize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "pyroscope", Subsystem: "query_backend", Name: "partial_result_cache_entry_size_bytes",
			Help:    "Compressed size of written partial result cache entries.",
			Buckets: prometheus.ExponentialBuckets(4<<10, 4, 10),
		}),
	}
	if reg != nil {
		reg.MustRegister(m.lookups, m.writes, m.skips, m.bytes, m.collected, m.entrySize)
	}
	return m
}

func newPartialResultCache(cache *resultCache, logger log.Logger, config PartialResultCacheConfig, reg prometheus.Registerer) *partialResultCache {
	c := &partialResultCache{
		resultCache: cache,
		logger:      logger,
		config:      config,
		metrics:     newPartialResultCacheMetrics(reg),
	}
	// Both are safe for concurrent EncodeAll and DecodeAll calls.
	c.encoder, _ = zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	c.decoder, _ = zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(uint64(max(config.MaxSizeBytes, 1))))
	return c
}

type partialResultMode int

const (
	partialResultNone partialResultMode = iota
	partialResultWrite
	partialResultRead
)

// mode decides whether the plan root writes (flame graph) or reads
// (function projections) the partial result cache for the request.
func (c *partialResultCache) mode(req *queryv1.InvokeRequest) (partialResultMode, string) {
	if c == nil || c.resultCacheBucket == nil || c.resultCacheOverrides == nil ||
		req.GetOptions().GetPartialResultCache() != queryv1.PartialResultCacheMode_PARTIAL_RESULT_CACHE_MODE_UNSPECIFIED ||
		req.GetOptions().GetCollectDiagnostics() || len(req.Tenant) != 1 || len(req.Query) != 1 || req.Query[0] == nil {
		return partialResultNone, ""
	}
	mode, queryType := partialResultQueryMode(req.Query[0])
	if mode == partialResultNone || !c.resultCacheOverrides.PartialResultCacheEnabled(req.Tenant[0]) ||
		len(planBlocks(req.GetQueryPlan().GetRoot(), nil)) == 0 {
		return partialResultNone, ""
	}
	if _, err := model.ParseMetricSelector(req.LabelSelector); err != nil {
		return partialResultNone, ""
	}
	return mode, queryType
}

func partialResultQueryMode(q *queryv1.Query) (partialResultMode, string) {
	switch q.QueryType {
	case queryv1.QueryType_QUERY_TREE:
		t := q.Tree
		if t == nil || t.FullSymbols || t.MaxNodes <= 0 || len(t.StackTraceSelector.GetCallSite()) > 0 || t.StackTraceSelector.GetGoPgo() != nil ||
			!validSampleSelectors(t.ProfileIdSelector, t.SpanSelector, t.TraceIdSelector) {
			return partialResultNone, ""
		}
		return partialResultWrite, "tree"
	case queryv1.QueryType_QUERY_FUNCTIONS:
		f := q.Functions
		if f == nil || f.StackTraceSelector.GetGoPgo() != nil || !validSampleSelectors(f.ProfileIdSelector, f.SpanSelector, f.TraceIdSelector) {
			return partialResultNone, ""
		}
		return partialResultRead, "functions"
	case queryv1.QueryType_QUERY_FUNCTION_TREE:
		f := q.FunctionTree
		if f == nil || f.StackTraceSelector.GetGoPgo() != nil || !validSampleSelectors(f.ProfileIdSelector, f.SpanSelector, f.TraceIdSelector) ||
			!supportedFunctionTreeOptions(f.Options, len(f.StackTraceSelector.GetCallSite())) {
			return partialResultNone, ""
		}
		return partialResultRead, "function_tree"
	}
	return partialResultNone, ""
}

// Requests that execution would reject must never be answered from the cache.
func validSampleSelectors(profileIDs, spans, traces []string) bool {
	if len(spans) > 0 && len(traces) > 0 {
		return false
	}
	if _, err := withProfileIDSelector(profileIDs...); err != nil {
		return false
	}
	if _, err := model.NewSpanSelector(spans); err != nil {
		return false
	}
	_, err := model.NewTraceSelector(traces)
	return err == nil
}

func supportedFunctionTreeOptions(o *typesv1.FunctionTreeOptions, callSite int) bool {
	if o == nil || o.GetMaxDepth() < 0 || o.GetMaxDepth() > maxFunctionTreeDepth {
		return false
	}
	switch o.GetDirection() {
	case typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS:
	case typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH:
		if callSite != 1 {
			return false
		}
	default:
		return false
	}
	switch o.GetSelection() {
	case typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH:
		return o.GetDirection() == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES
	case typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN:
		return callSite > 0
	}
	return false
}

func planBlocks(n *queryv1.QueryNode, dst []*metastorev1.BlockMeta) []*metastorev1.BlockMeta {
	if n == nil {
		return dst
	}
	dst = append(dst, n.Blocks...)
	for _, c := range n.Children {
		dst = planBlocks(c, dst)
	}
	return dst
}

// partialResultCacheIdentity describes the samples a flame graph or function
// projection query selects. Stack trace selectors are not part of it: the
// cached flame graph has none, and projections apply theirs to the result.
func partialResultCacheIdentity(req *queryv1.InvokeRequest) (*queryv1.PartialResultCacheKey, error) {
	selector, err := canonicalResultCacheSelector(req.LabelSelector)
	if err != nil {
		return nil, err
	}
	var profileIDs, spans, traces []string
	switch q := req.Query[0]; q.QueryType {
	case queryv1.QueryType_QUERY_TREE:
		profileIDs, spans, traces = q.Tree.ProfileIdSelector, q.Tree.SpanSelector, q.Tree.TraceIdSelector
	case queryv1.QueryType_QUERY_FUNCTIONS:
		profileIDs, spans, traces = q.Functions.ProfileIdSelector, q.Functions.SpanSelector, q.Functions.TraceIdSelector
	case queryv1.QueryType_QUERY_FUNCTION_TREE:
		profileIDs, spans, traces = q.FunctionTree.ProfileIdSelector, q.FunctionTree.SpanSelector, q.FunctionTree.TraceIdSelector
	default:
		return nil, fmt.Errorf("unsupported query type %s", q.QueryType)
	}
	blocks := planBlocks(req.QueryPlan.GetRoot(), nil)
	slices.SortStableFunc(blocks, func(a, b *metastorev1.BlockMeta) int {
		switch {
		case a.Id < b.Id:
			return -1
		case a.Id > b.Id:
			return 1
		}
		return 0
	})
	h := sha256.New()
	ids := make([]string, len(blocks))
	marshal := proto.MarshalOptions{Deterministic: true}
	var size [binary.MaxVarintLen64]byte
	for i, b := range blocks {
		data, err := marshal.Marshal(b)
		if err != nil {
			return nil, err
		}
		_, _ = h.Write(size[:binary.PutUvarint(size[:], uint64(len(data)))])
		_, _ = h.Write(data)
		ids[i] = b.Id
	}
	return &queryv1.PartialResultCacheKey{
		Version:           partialResultCacheVersion,
		Tenant:            req.Tenant[0],
		StartTime:         req.StartTime,
		EndTime:           req.EndTime,
		LabelSelector:     selector,
		ProfileIdSelector: sortedUnique(profileIDs),
		SpanSelector:      sortedUnique(spans),
		TraceIdSelector:   sortedUnique(traces),
		BlockIds:          ids,
		BlocksDigest:      h.Sum(nil),
	}, nil
}

func sortedUnique(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	s = slices.Clone(s)
	slices.Sort(s)
	return slices.Compact(s)
}

func partialResultCacheObjectKey(generation uint, identity *queryv1.PartialResultCacheKey) (string, error) {
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(identity)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return fmt.Sprintf("%s%s/%04d-%s", partialResultCachePrefix, identity.Tenant, generation, hex.EncodeToString(digest[:])), nil
}

func withPartialResultCacheMode(req *queryv1.InvokeRequest, mode queryv1.PartialResultCacheMode) *queryv1.InvokeRequest {
	options := req.GetOptions().CloneVT()
	if options == nil {
		options = new(queryv1.InvokeOptions)
	}
	options.PartialResultCache = mode
	return &queryv1.InvokeRequest{
		Tenant:        req.Tenant,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		LabelSelector: req.LabelSelector,
		Query:         req.Query,
		QueryPlan:     req.QueryPlan,
		Options:       options,
	}
}

// invokePartialResults runs the request at the plan root, writing or
// reading the partial result cache. Cache failures never fail the query.
func (q *QueryBackend) invokePartialResults(ctx context.Context, span *tracing.Span, req *queryv1.InvokeRequest, mode partialResultMode, queryType string) (*queryv1.InvokeResponse, error) {
	c := q.partialResults
	identity, err := partialResultCacheIdentity(req)
	var objectKey string
	if err == nil {
		objectKey, err = partialResultCacheObjectKey(c.resultCacheOverrides.ResultCacheGeneration(req.Tenant[0]), identity)
	}
	if err != nil {
		level.Warn(c.logger).Log("msg", "failed to compute partial result cache key", "err", err)
		return q.execute(ctx, withPartialResultCacheMode(req, queryv1.PartialResultCacheMode_PARTIAL_RESULT_CACHE_MODE_SKIP))
	}
	span.SetTag("partial_result_cache.key", objectKey)
	span.SetTag("partial_result_cache.blocks", len(identity.BlockIds))
	if mode == partialResultWrite {
		return q.invokeWithPartialResultWrite(ctx, span, req, identity, objectKey)
	}
	resp, outcome, err := c.race(ctx,
		func(ctx context.Context, found func()) (*queryv1.InvokeResponse, string) {
			return c.lookup(ctx, objectKey, identity, req.Query[0], found)
		},
		func(ctx context.Context) (*queryv1.InvokeResponse, error) {
			return q.execute(ctx, withPartialResultCacheMode(req, queryv1.PartialResultCacheMode_PARTIAL_RESULT_CACHE_MODE_SKIP))
		},
	)
	if outcome != "" {
		c.metrics.lookups.WithLabelValues(queryType, outcome).Inc()
		span.SetTag("partial_result_cache.lookup", outcome)
	}
	if err != nil {
		return nil, err
	}
	if outcome == resultCacheOutcomeHit {
		level.Debug(c.logger).Log("msg", "partial result cache hit", "key", objectKey, "query_type", queryType)
		pushback.SetNoRetry(ctx)
	}
	return resp, nil
}

func (q *QueryBackend) invokeWithPartialResultWrite(ctx context.Context, span *tracing.Span, req *queryv1.InvokeRequest, identity *queryv1.PartialResultCacheKey, objectKey string) (*queryv1.InvokeResponse, error) {
	c := q.partialResults
	if c.exists(ctx, objectKey) {
		c.metrics.skips.WithLabelValues("exists").Inc()
		span.SetTag("partial_result_cache.write", "skipped")
		span.SetTag("partial_result_cache.skip_reason", "exists")
		return q.execute(ctx, withPartialResultCacheMode(req, queryv1.PartialResultCacheMode_PARTIAL_RESULT_CACHE_MODE_SKIP))
	}
	resp, err := q.execute(ctx, withPartialResultCacheMode(req, queryv1.PartialResultCacheMode_PARTIAL_RESULT_CACHE_MODE_COLLECT))
	if err != nil {
		return nil, err
	}
	partial := resp.PartialResult
	resp.PartialResult = nil
	reason := partialResultSkipReason(resp, partial)
	if reason == "" {
		reason = c.enqueue(identity, objectKey, partial.Trees)
	}
	if reason != "" {
		c.metrics.skips.WithLabelValues(reason).Inc()
		span.SetTag("partial_result_cache.write", "skipped")
		span.SetTag("partial_result_cache.skip_reason", reason)
		return resp, nil
	}
	span.SetTag("partial_result_cache.write", "enqueued")
	return resp, nil
}

// exists reports whether the entry is already written. Collecting trees
// is skipped then; a slow or failed check collects them anyway.
func (c *partialResultCache) exists(ctx context.Context, objectKey string) bool {
	ctx, cancel := context.WithTimeout(ctx, c.config.ExecutionDelay)
	defer cancel()
	ok, err := c.resultCacheBucket.Exists(ctx, objectKey)
	return err == nil && ok
}

func partialResultSkipReason(resp *queryv1.InvokeResponse, partial *queryv1.PartialResult) string {
	switch {
	case partial == nil:
		return "incomplete"
	case partial.Incomplete != "":
		return partial.Incomplete
	case len(partial.Trees) == 0:
		return "empty"
	}
	for _, r := range resp.Reports {
		if r.GetReportType() != queryv1.ReportType_REPORT_TREE {
			continue
		}
		truncated, err := model.TreeBytesTruncated(r.Tree.GetTree())
		if err != nil {
			return "invalid_tree"
		}
		if truncated {
			return ""
		}
	}
	return "not_truncated"
}

func (c *partialResultCache) enqueue(identity *queryv1.PartialResultCacheKey, objectKey string, trees []*queryv1.FunctionStackTree) string {
	if c.queued.Add(1) > partialResultCacheMaxQueued {
		c.queued.Add(-1)
		return "queue_full"
	}
	w := &partialResultEntry{identity: identity, trees: trees}
	job := resultCacheWriteJob{
		queryType: "tree",
		key:       objectKey,
		upload:    func(ctx context.Context) { c.uploadPartialResult(ctx, objectKey, w) },
		drop:      c.dropPartialResult,
	}
	select {
	case c.resultCacheWrites <- job:
		return ""
	default:
		c.queued.Add(-1)
		return "queue_full"
	}
}

type partialResultEntry struct {
	identity *queryv1.PartialResultCacheKey
	trees    []*queryv1.FunctionStackTree
}

func (c *partialResultCache) encode(w *partialResultEntry) ([]byte, error) {
	if len(w.trees) == 0 {
		return nil, errors.New("no function stack trees")
	}
	encoded := w.trees[0]
	if len(w.trees) > 1 {
		tree := model.NewFunctionStackTree()
		for _, t := range w.trees {
			if err := tree.MergeProto(t); err != nil {
				return nil, err
			}
		}
		encoded = tree.Proto()
	}
	data, err := encoded.MarshalVT()
	if err != nil {
		return nil, err
	}
	if len(data) > c.config.MaxSizeBytes {
		return nil, errPartialResultTooLarge
	}
	entry := &queryv1.PartialResultCacheEntry{
		Key:               w.identity,
		FunctionStackTree: c.encoder.EncodeAll(data, nil),
	}
	return entry.MarshalVT()
}

var errPartialResultTooLarge = errors.New("partial result exceeds the maximum size")

// uploadPartialResult runs in the result cache writers.
func (c *partialResultCache) uploadPartialResult(ctx context.Context, objectKey string, w *partialResultEntry) {
	defer c.queued.Add(-1)
	data, err := c.encode(w)
	if errors.Is(err, errPartialResultTooLarge) {
		c.metrics.skips.WithLabelValues("too_large").Inc()
		return
	}
	if err == nil {
		writeCtx, cancel := context.WithTimeout(ctx, resultCacheWriteTimeout)
		err = c.resultCacheBucket.Upload(writeCtx, objectKey, bytes.NewReader(data))
		cancel()
	}
	if err != nil {
		c.metrics.writes.WithLabelValues(resultCacheOutcomeError).Inc()
		level.Warn(c.logger).Log("msg", "failed to write partial result cache entry", "key", objectKey, "err", err)
		return
	}
	c.metrics.writes.WithLabelValues("success").Inc()
	c.metrics.bytes.WithLabelValues("write").Add(float64(len(data)))
	c.metrics.entrySize.Observe(float64(len(data)))
	level.Debug(c.logger).Log("msg", "partial result cache entry written", "key", objectKey, "size", len(data))
}

func (c *partialResultCache) dropPartialResult() {
	c.queued.Add(-1)
	c.metrics.skips.WithLabelValues("shutdown").Inc()
}

func (c *partialResultCache) lookup(ctx context.Context, objectKey string, identity *queryv1.PartialResultCacheKey, query *queryv1.Query, found func()) (*queryv1.InvokeResponse, string) {
	r, err := c.resultCacheBucket.Get(ctx, objectKey)
	if err != nil {
		switch {
		case c.resultCacheBucket.IsObjNotFoundErr(err):
			return nil, resultCacheOutcomeMiss
		case errors.Is(err, context.Canceled):
			return nil, resultCacheOutcomeCanceled
		}
		return nil, resultCacheOutcomeError
	}
	defer r.Close()
	found()
	data, err := io.ReadAll(io.LimitReader(r, int64(c.config.MaxSizeBytes)+1))
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, resultCacheOutcomeCanceled
		}
		return nil, resultCacheOutcomeError
	}
	c.metrics.bytes.WithLabelValues("read").Add(float64(len(data)))
	if len(data) > c.config.MaxSizeBytes {
		return nil, "too_large"
	}
	entry := new(queryv1.PartialResultCacheEntry)
	if err = entry.UnmarshalVT(data); err != nil {
		return nil, c.invalid(objectKey, partialResultCorrupt)
	}
	if entry.Key.GetVersion() != partialResultCacheVersion {
		return nil, c.invalid(objectKey, "version_mismatch")
	}
	if !proto.Equal(entry.Key, identity) {
		return nil, "collision"
	}
	data, err = c.decoder.DecodeAll(entry.FunctionStackTree, nil)
	if err != nil {
		return nil, c.invalid(objectKey, partialResultCorrupt)
	}
	tree := new(queryv1.FunctionStackTree)
	if err = tree.UnmarshalVT(data); err != nil {
		return nil, c.invalid(objectKey, partialResultCorrupt)
	}
	x, err := model.NewFunctionStackIndex(tree)
	if err != nil {
		return nil, c.invalid(objectKey, partialResultCorrupt)
	}
	if ctx.Err() != nil {
		return nil, resultCacheOutcomeCanceled
	}
	return &queryv1.InvokeResponse{
		Reports:     []*queryv1.Report{projectFunctionStackIndex(x, query)},
		Diagnostics: &queryv1.Diagnostics{ExecutionNode: &queryv1.ExecutionNode{Stats: &queryv1.ExecutionStats{}}},
	}, resultCacheOutcomeHit
}

// invalid deletes an unusable entry in the background, so that the next
// flame graph query rewrites it instead of skipping an existing entry.
func (c *partialResultCache) invalid(objectKey, outcome string) string {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), resultCacheWriteTimeout)
		defer cancel()
		if err := c.resultCacheBucket.Delete(ctx, objectKey); err != nil && !c.resultCacheBucket.IsObjNotFoundErr(err) {
			level.Warn(c.logger).Log("msg", "failed to delete invalid partial result cache entry", "key", objectKey, "err", err)
		}
	}()
	return outcome
}

func projectFunctionStackIndex(x *model.FunctionStackIndex, query *queryv1.Query) *queryv1.Report {
	if query.QueryType == queryv1.QueryType_QUERY_FUNCTIONS {
		return &queryv1.Report{
			ReportType: queryv1.ReportType_REPORT_FUNCTIONS,
			Functions: &queryv1.FunctionsReport{
				Query: query.Functions.CloneVT(),
				Table: x.Functions(callSiteNames(query.Functions.StackTraceSelector)),
			},
		}
	}
	return &queryv1.Report{
		ReportType: queryv1.ReportType_REPORT_FUNCTION_TREE,
		FunctionTree: &queryv1.FunctionTreeReport{
			Query: query.FunctionTree.CloneVT(),
			Tree:  x.FunctionTree(callSiteNames(query.FunctionTree.StackTraceSelector), query.FunctionTree.Options),
		},
	}
}

func callSiteNames(selector *typesv1.StackTraceSelector) []string {
	names := make([]string, len(selector.GetCallSite()))
	for i, loc := range selector.GetCallSite() {
		names[i] = loc.GetName()
	}
	return names
}

// race starts execution once the lookup misses, or after the execution delay
// unless the lookup has found an entry by then. A successful lookup cancels
// execution; execution is joined before returning.
func (c *partialResultCache) race(
	ctx context.Context,
	lookup func(ctx context.Context, found func()) (*queryv1.InvokeResponse, string),
	execute func(ctx context.Context) (*queryv1.InvokeResponse, error),
) (*queryv1.InvokeResponse, string, error) {
	raceCtx, cancel := context.WithCancel(ctx)
	start := make(chan struct{})
	found := make(chan struct{})
	var startOnce, foundOnce sync.Once
	type lookupResult struct {
		resp    *queryv1.InvokeResponse
		outcome string
	}
	type executionResult struct {
		resp *queryv1.InvokeResponse
		err  error
	}
	lookupDone := make(chan lookupResult, 1)
	executionDone := make(chan executionResult, 1)
	joined := make(chan struct{})
	defer func() {
		cancel()
		<-joined
	}()
	go func() {
		resp, outcome := lookup(raceCtx, func() { foundOnce.Do(func() { close(found) }) })
		if outcome != resultCacheOutcomeHit {
			startOnce.Do(func() { close(start) })
		}
		lookupDone <- lookupResult{resp: resp, outcome: outcome}
	}()
	go func() {
		defer close(joined)
		timer := time.NewTimer(c.config.ExecutionDelay)
		defer timer.Stop()
		delay, entry := timer.C, found
		for waiting := true; waiting; {
			select {
			case <-raceCtx.Done():
				return
			case <-start:
				waiting = false
			case <-entry:
				delay, entry = nil, nil
			case <-delay:
				waiting = false
			}
		}
		var resp *queryv1.InvokeResponse
		err := util.RecoverPanic(func() error {
			var err error
			resp, err = execute(raceCtx)
			return err
		})()
		executionDone <- executionResult{resp: resp, err: err}
	}()
	var outcome string
	for {
		select {
		case <-ctx.Done():
			return nil, outcome, ctx.Err()
		case r := <-lookupDone:
			lookupDone = nil
			outcome = r.outcome
			if r.outcome == resultCacheOutcomeHit {
				return r.resp, outcome, nil
			}
		case r := <-executionDone:
			if outcome == "" {
				outcome = "slow"
			}
			return r.resp, outcome, r.err
		}
	}
}

// collector returns nil unless the plan root asked this READ node for trees.
func (c *partialResultCache) collector(req *queryv1.InvokeRequest) *functionStackTreeCollector {
	if c == nil || req.GetOptions().GetPartialResultCache() != queryv1.PartialResultCacheMode_PARTIAL_RESULT_CACHE_MODE_COLLECT ||
		len(req.Query) != 1 || req.Query[0].GetQueryType() != queryv1.QueryType_QUERY_TREE || req.Query[0].GetTree().GetFullSymbols() {
		return nil
	}
	return &functionStackTreeCollector{maxSize: c.config.MaxSizeBytes}
}

func (c *partialResultCache) collected(span *tracing.Span, p *queryv1.PartialResult) {
	var size, nodes int
	for _, t := range p.Trees {
		size += t.SizeVT()
		nodes += len(t.NodeName)
	}
	c.metrics.collected.Add(float64(size))
	span.SetTag("partial_result_cache.collected_nodes", nodes)
	span.SetTag("partial_result_cache.collected_bytes", size)
	if p.Incomplete != "" {
		span.SetTag("partial_result_cache.incomplete", p.Incomplete)
	}
}

// functionStackTreeCollector merges the stacks of all datasets of a READ node.
type functionStackTreeCollector struct {
	maxSize    int
	incomplete atomic.Bool
	mu         sync.Mutex
	tooLarge   bool
	tree       *model.FunctionStackTree
}

func (c *functionStackTreeCollector) add(stacks *model.StacktraceTree, names []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tooLarge {
		return
	}
	if c.tree == nil {
		c.tree = model.NewFunctionStackTree()
	}
	c.tree.MergeStacktraceTree(stacks, names)
	if c.tree.SizeEstimate() > c.maxSize {
		c.tooLarge, c.tree = true, nil
	}
}

func (c *functionStackTreeCollector) result() *queryv1.PartialResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.incomplete.Load():
		return &queryv1.PartialResult{Incomplete: "incomplete"}
	case c.tooLarge:
		return &queryv1.PartialResult{Incomplete: "too_large"}
	case c.tree == nil:
		return &queryv1.PartialResult{}
	}
	return &queryv1.PartialResult{Trees: []*queryv1.FunctionStackTree{c.tree.Proto()}}
}

// partialResultMerger merges the trees of MERGE node children as they arrive,
// so that a node holds one merged tree instead of all child trees.
type partialResultMerger struct {
	maxSize    int
	mu         sync.Mutex
	incomplete string
	tree       *model.FunctionStackTree
}

func (m *partialResultMerger) add(p *queryv1.PartialResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.incomplete != "":
		return
	case p == nil:
		m.incomplete = "incomplete"
	case p.Incomplete != "":
		m.incomplete = p.Incomplete
	}
	for _, t := range p.GetTrees() {
		if m.incomplete != "" {
			break
		}
		if m.tree == nil {
			m.tree = model.NewFunctionStackTree()
		}
		if err := m.tree.MergeProto(t); err != nil {
			m.incomplete = "invalid"
		} else if m.tree.SizeEstimate() > m.maxSize {
			m.incomplete = "too_large"
		}
	}
	if m.incomplete != "" {
		m.tree = nil
	}
}

func (m *partialResultMerger) result() *queryv1.PartialResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.incomplete != "" || m.tree == nil {
		return &queryv1.PartialResult{Incomplete: m.incomplete}
	}
	return &queryv1.PartialResult{Trees: []*queryv1.FunctionStackTree{m.tree.Proto()}}
}
