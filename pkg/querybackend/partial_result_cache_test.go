package querybackend

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	thanosobjstore "github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
)

func partialResultKeyTestRequest(query *queryv1.Query) *queryv1.InvokeRequest {
	blocks := []*metastorev1.BlockMeta{
		{Id: "block-a", StringTable: []string{"", "tenant-a", "service-a"}, Datasets: []*metastorev1.Dataset{{Tenant: 1, Name: 2}}},
		{Id: "block-b", StringTable: []string{"", "tenant-a", "service-a"}, Datasets: []*metastorev1.Dataset{{Tenant: 1, Name: 2}}},
	}
	return &queryv1.InvokeRequest{
		Tenant:        []string{"tenant-a"},
		StartTime:     100,
		EndTime:       200,
		LabelSelector: `{service_name="service-a",__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`,
		Query:         []*queryv1.Query{query},
		QueryPlan:     queryplan.Build(blocks, 1, 2),
	}
}

func partialResultTestKey(t *testing.T, req *queryv1.InvokeRequest, generation uint) string {
	t.Helper()
	identity, err := partialResultCacheIdentity(req)
	require.NoError(t, err)
	key, err := partialResultCacheObjectKey(generation, identity)
	require.NoError(t, err)
	return key
}

func TestPartialResultCacheMode(t *testing.T) {
	bucket := objstore.NewBucket(thanosobjstore.NewInMemBucket())
	q, err := New(Config{}, log.NewNopLogger(), nil, nil, nil, bucket, resultCacheOverrides{partial: true})
	require.NoError(t, err)
	chain := &typesv1.FunctionTreeOptions{
		Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS,
		Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN,
	}
	rootPath := &typesv1.FunctionTreeOptions{
		Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
		Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
	}
	for _, tc := range []struct {
		name   string
		query  *queryv1.Query
		mutate func(*queryv1.InvokeRequest)
		want   partialResultMode
	}{
		{name: "tree", query: treeQuery(16), want: partialResultWrite},
		{name: "tree without node limit", query: treeQuery(0)},
		{name: "full symbols tree", query: &queryv1.Query{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: 16, FullSymbols: true}}},
		{name: "tree with call site", query: &queryv1.Query{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: 16, StackTraceSelector: callSiteSelector([]string{"main"})}}},
		{name: "tree with go pgo", query: &queryv1.Query{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: 16, StackTraceSelector: &typesv1.StackTraceSelector{GoPgo: &typesv1.GoPGO{}}}}},
		{name: "tree with invalid span selector", query: &queryv1.Query{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: 16, SpanSelector: []string{"x"}}}},
		{name: "tree with span and trace selectors", query: &queryv1.Query{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{
			MaxNodes: 16, SpanSelector: []string{"9a517183f26a089d"}, TraceIdSelector: []string{"7c9e66797425440de944be07fc1f90ae"},
		}}},
		{name: "tree with span selector", query: &queryv1.Query{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: 16, SpanSelector: []string{"9a517183f26a089d"}}}, want: partialResultWrite},
		{name: "functions", query: functionsQuery([]string{"main"}), want: partialResultRead},
		{name: "function tree chain", query: functionTreeQuery([]string{"main", "F"}, chain), want: partialResultRead},
		{name: "function tree root path", query: functionTreeQuery(nil, rootPath), want: partialResultRead},
		{name: "function tree root path callers", query: functionTreeQuery(nil, &typesv1.FunctionTreeOptions{
			Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS,
			Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
		})},
		{name: "function tree both with chain", query: functionTreeQuery([]string{"main", "F"}, &typesv1.FunctionTreeOptions{
			Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH,
			Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN,
		})},
		{name: "function tree empty chain", query: functionTreeQuery(nil, chain)},
		{name: "function tree depth out of range", query: functionTreeQuery([]string{"main"}, &typesv1.FunctionTreeOptions{
			Direction: chain.Direction, Selection: chain.Selection, MaxDepth: proto.Int32(129),
		})},
		{name: "function tree without options", query: functionTreeQuery([]string{"main"}, nil)},
		{name: "pprof", query: &queryv1.Query{QueryType: queryv1.QueryType_QUERY_PPROF, Pprof: &queryv1.PprofQuery{MaxNodes: 16}}},
		{name: "label names", query: &queryv1.Query{QueryType: queryv1.QueryType_QUERY_LABEL_NAMES, LabelNames: &queryv1.LabelNamesQuery{}}},
		{name: "multiple queries", query: treeQuery(16), mutate: func(r *queryv1.InvokeRequest) { r.Query = append(r.Query, functionsQuery(nil)) }},
		{name: "multiple tenants", query: treeQuery(16), mutate: func(r *queryv1.InvokeRequest) { r.Tenant = append(r.Tenant, "tenant-b") }},
		{name: "diagnostics", query: treeQuery(16), mutate: func(r *queryv1.InvokeRequest) { r.Options = &queryv1.InvokeOptions{CollectDiagnostics: true} }},
		{name: "child request", query: functionsQuery(nil), mutate: func(r *queryv1.InvokeRequest) {
			r.Options = &queryv1.InvokeOptions{PartialResultCache: queryv1.PartialResultCacheMode_PARTIAL_RESULT_CACHE_MODE_SKIP}
		}},
		{name: "collecting child request", query: treeQuery(16), mutate: func(r *queryv1.InvokeRequest) {
			r.Options = &queryv1.InvokeOptions{PartialResultCache: queryv1.PartialResultCacheMode_PARTIAL_RESULT_CACHE_MODE_COLLECT}
		}},
		{name: "invalid selector", query: treeQuery(16), mutate: func(r *queryv1.InvokeRequest) { r.LabelSelector = "{" }},
		{name: "no blocks", query: treeQuery(16), mutate: func(r *queryv1.InvokeRequest) {
			r.QueryPlan = &queryv1.QueryPlan{Root: &queryv1.QueryNode{Type: queryv1.QueryNode_READ}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := partialResultKeyTestRequest(tc.query)
			if tc.mutate != nil {
				tc.mutate(req)
			}
			mode, _ := q.partialResults.mode(req)
			require.Equal(t, tc.want, mode)
		})
	}

	t.Run("disabled tenant", func(t *testing.T) {
		q, err := New(Config{}, log.NewNopLogger(), nil, nil, nil, bucket, resultCacheOverrides{enabled: true})
		require.NoError(t, err)
		mode, _ := q.partialResults.mode(partialResultKeyTestRequest(treeQuery(16)))
		require.Equal(t, partialResultNone, mode)
	})
	t.Run("no bucket", func(t *testing.T) {
		q, err := New(Config{}, log.NewNopLogger(), nil, nil, nil, nil, resultCacheOverrides{partial: true})
		require.NoError(t, err)
		mode, _ := q.partialResults.mode(partialResultKeyTestRequest(treeQuery(16)))
		require.Equal(t, partialResultNone, mode)
	})
}

func TestPartialResultCacheKey(t *testing.T) {
	base := partialResultKeyTestRequest(treeQuery(16))
	baseKey := partialResultTestKey(t, base, 1)

	// Inputs that select the same samples share the entry.
	for name, mutate := range map[string]func(*queryv1.InvokeRequest){
		"max nodes": func(r *queryv1.InvokeRequest) { r.Query[0].Tree.MaxNodes = 1 << 20 },
		"functions": func(r *queryv1.InvokeRequest) { r.Query[0] = functionsQuery([]string{"main"}) },
		"function tree": func(r *queryv1.InvokeRequest) {
			r.Query[0] = functionTreeQuery([]string{"main", "F"}, &typesv1.FunctionTreeOptions{
				Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS,
				Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN,
				MaxDepth:  proto.Int32(4),
			})
		},
		"matcher order": func(r *queryv1.InvokeRequest) {
			r.LabelSelector = `{__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds", service_name="service-a"}`
		},
		"plan shape": func(r *queryv1.InvokeRequest) {
			blocks := planBlocks(r.QueryPlan.Root, nil)
			blocks[0], blocks[1] = blocks[1], blocks[0]
			r.QueryPlan = queryplan.Build(blocks, 2, 2)
		},
		"diagnostics and sanitize": func(r *queryv1.InvokeRequest) {
			r.Options = &queryv1.InvokeOptions{SanitizeOnMerge: true, CollectDiagnostics: true}
		},
	} {
		t.Run("same/"+name, func(t *testing.T) {
			req := base.CloneVT()
			mutate(req)
			require.Equal(t, baseKey, partialResultTestKey(t, req, 1))
		})
	}

	withSelectors := func(profileIDs, spans, traces []string) func(*queryv1.InvokeRequest) {
		return func(r *queryv1.InvokeRequest) {
			r.Query[0].Tree.ProfileIdSelector = profileIDs
			r.Query[0].Tree.SpanSelector = spans
			r.Query[0].Tree.TraceIdSelector = traces
		}
	}
	spanKey := func(spans ...string) string {
		req := base.CloneVT()
		withSelectors(nil, spans, nil)(req)
		return partialResultTestKey(t, req, 1)
	}
	require.Equal(t, spanKey("9a517183f26a089d", "5a4fe264a9c987fe"), spanKey("5a4fe264a9c987fe", "9a517183f26a089d"))

	keys := map[string]string{"base": baseKey}
	for name, mutate := range map[string]func(*queryv1.InvokeRequest){
		"tenant": func(r *queryv1.InvokeRequest) { r.Tenant = []string{"tenant-b"} },
		"start":  func(r *queryv1.InvokeRequest) { r.StartTime++ },
		"end":    func(r *queryv1.InvokeRequest) { r.EndTime++ },
		"selector": func(r *queryv1.InvokeRequest) {
			r.LabelSelector = `{service_name="service-b",__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`
		},
		"profile type": func(r *queryv1.InvokeRequest) {
			r.LabelSelector = `{service_name="service-a",__profile_type__="memory:alloc_space:bytes:space:bytes"}`
		},
		"extra matcher": func(r *queryv1.InvokeRequest) {
			r.LabelSelector = `{service_name="service-a",pod="a",__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`
		},
		"profile ids": withSelectors([]string{"7c9e6679-7425-40de-944b-e07fc1f90ae7"}, nil, nil),
		"spans":       withSelectors(nil, []string{"9a517183f26a089d"}, nil),
		"traces":      withSelectors(nil, nil, []string{"7c9e66797425440de944be07fc1f90ae"}),
		"projection ids": func(r *queryv1.InvokeRequest) {
			r.Query[0] = functionsQuery(nil)
			r.Query[0].Functions.SpanSelector = []string{"5a4fe264a9c987fe"}
		},
		"block id": func(r *queryv1.InvokeRequest) { r.QueryPlan.Root.Children[0].Blocks[0].Id = "block-c" },
		"block added": func(r *queryv1.InvokeRequest) {
			r.QueryPlan.Root.Children = append(r.QueryPlan.Root.Children, &queryv1.QueryNode{Type: queryv1.QueryNode_READ, Blocks: []*metastorev1.BlockMeta{{Id: "block-c"}}})
		},
		"block removed": func(r *queryv1.InvokeRequest) { r.QueryPlan.Root.Children = r.QueryPlan.Root.Children[:1] },
		"dataset": func(r *queryv1.InvokeRequest) {
			b := r.QueryPlan.Root.Children[0].Blocks[0]
			b.Datasets = append(b.Datasets, &metastorev1.Dataset{Tenant: 1, Name: 2, MinTime: 1})
		},
		"compaction level": func(r *queryv1.InvokeRequest) { r.QueryPlan.Root.Children[0].Blocks[0].CompactionLevel++ },
	} {
		req := base.CloneVT()
		mutate(req)
		key := partialResultTestKey(t, req, 1)
		for other, k := range keys {
			require.NotEqual(t, k, key, "%s and %s must not share a key", name, other)
		}
		keys[name] = key
	}
	require.NotEqual(t, baseKey, partialResultTestKey(t, base, 2), "generation")

	identity, err := partialResultCacheIdentity(base)
	require.NoError(t, err)
	identity.Version++
	key, err := partialResultCacheObjectKey(1, identity)
	require.NoError(t, err)
	require.NotEqual(t, baseKey, key, "version")
}

func TestPartialResultCacheWritesOnlyTruncatedTrees(t *testing.T) {
	f := newPartialResultFixture(t)
	plan := f.plan()
	cache := objstore.NewBucket(thanosobjstore.NewInMemBucket())
	q, _ := f.backend(t, cache, resultCacheOverrides{partial: true})
	uncached, _ := f.backend(t, nil, resultCacheOverrides{})
	objects := func() int {
		var n int
		require.NoError(t, cache.Iter(context.Background(), "", func(string) error { n++; return nil }, thanosobjstore.WithRecursiveIter()))
		return n
	}

	require.Equal(t,
		invokeReports(t, uncached, partialResultRequest(plan, partialResultTestSelector, treeQuery(1<<20))),
		invokeReports(t, q, partialResultRequest(plan, partialResultTestSelector, treeQuery(1<<20))))
	collected := testutil.ToFloat64(q.partialResults.metrics.collected)
	require.Positive(t, collected)
	for _, query := range []*queryv1.Query{
		functionsQuery(nil),
		functionTreeQuery(nil, &typesv1.FunctionTreeOptions{
			Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
			Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
		}),
	} {
		require.Equal(t,
			invokeReports(t, uncached, partialResultRequest(plan, partialResultTestSelector, query)),
			invokeReports(t, q, partialResultRequest(plan, partialResultTestSelector, query)), "%v", query)
	}
	// Their merged results depend on the plan, which is not deterministic.
	for _, query := range []*queryv1.Query{
		{QueryType: queryv1.QueryType_QUERY_PPROF, Pprof: &queryv1.PprofQuery{MaxNodes: 16}},
		{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: 16, FullSymbols: true}},
	} {
		invokeReports(t, q, partialResultRequest(plan, partialResultTestSelector, query))
	}
	require.Equal(t, float64(1), testutil.ToFloat64(q.partialResults.metrics.skips.WithLabelValues("not_truncated")))
	require.Equal(t, collected, testutil.ToFloat64(q.partialResults.metrics.collected), "only flame graph queries collect trees")
	require.Zero(t, objects())

	req := partialResultRequest(plan, partialResultTestSelector, treeQuery(16))
	require.Equal(t,
		invokeReports(t, uncached, req.CloneVT()),
		invokeReports(t, q, req.CloneVT()), "collecting trees must not change the flame graph")
	require.Eventually(t, func() bool { return objects() == 1 }, 10*time.Second, 10*time.Millisecond)
	require.Equal(t, float64(1), testutil.ToFloat64(q.partialResults.metrics.writes.WithLabelValues("success")))

	// An existing entry is not collected again.
	collected = testutil.ToFloat64(q.partialResults.metrics.collected)
	require.Equal(t,
		invokeReports(t, uncached, req.CloneVT()),
		invokeReports(t, q, req.CloneVT()))
	require.Equal(t, float64(1), testutil.ToFloat64(q.partialResults.metrics.skips.WithLabelValues("exists")))
	require.Equal(t, collected, testutil.ToFloat64(q.partialResults.metrics.collected))
}

func TestPartialResultCacheTooLarge(t *testing.T) {
	f := newPartialResultFixture(t)
	cfg := testPartialResultConfig()
	cfg.ResultCache.PartialResults.MaxSizeBytes = 64
	cache := objstore.NewBucket(thanosobjstore.NewInMemBucket())
	q := newPartialResultBackend(t, cfg, f.primary, cache, resultCacheOverrides{partial: true})
	uncached, _ := f.backend(t, nil, resultCacheOverrides{})
	req := partialResultRequest(f.plan(), partialResultTestSelector, treeQuery(16))
	require.Equal(t, invokeReports(t, uncached, req.CloneVT()), invokeReports(t, q, req.CloneVT()))
	require.Equal(t, float64(1), testutil.ToFloat64(q.partialResults.metrics.skips.WithLabelValues("too_large")))
}

type failingUploadBucket struct {
	objstore.Bucket
	uploads atomic.Int32
	block   chan struct{}
}

func (b *failingUploadBucket) Upload(ctx context.Context, _ string, _ io.Reader, _ ...thanosobjstore.ObjectUploadOption) error {
	b.uploads.Add(1)
	if b.block != nil {
		select {
		case <-b.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return errors.New("upload failed")
}

func TestPartialResultCacheWriteFailures(t *testing.T) {
	f := newPartialResultFixture(t)
	plan := f.plan()
	uncached, _ := f.backend(t, nil, resultCacheOverrides{})
	want := invokeReports(t, uncached, partialResultRequest(plan, partialResultTestSelector, treeQuery(16)))

	t.Run("upload error", func(t *testing.T) {
		cache := &failingUploadBucket{Bucket: objstore.NewBucket(thanosobjstore.NewInMemBucket())}
		q, _ := f.backend(t, cache, resultCacheOverrides{partial: true})
		require.Equal(t, want, invokeReports(t, q, partialResultRequest(plan, partialResultTestSelector, treeQuery(16))))
		require.Eventually(t, func() bool {
			return testutil.ToFloat64(q.partialResults.metrics.writes.WithLabelValues("error")) == 1
		}, 10*time.Second, 10*time.Millisecond)
		require.Zero(t, q.partialResults.queued.Load())
	})

	t.Run("slow uploads fill the queue", func(t *testing.T) {
		cache := &failingUploadBucket{Bucket: objstore.NewBucket(thanosobjstore.NewInMemBucket()), block: make(chan struct{})}
		defer close(cache.block)
		q, _ := f.backend(t, cache, resultCacheOverrides{partial: true})
		for range partialResultCacheMaxQueued + 2 {
			start := time.Now()
			require.Equal(t, want, invokeReports(t, q, partialResultRequest(plan, partialResultTestSelector, treeQuery(16))))
			require.Less(t, time.Since(start), 5*time.Second, "queries must not wait for uploads")
		}
		require.Equal(t, float64(2), testutil.ToFloat64(q.partialResults.metrics.skips.WithLabelValues("queue_full")))
		require.Equal(t, int32(partialResultCacheMaxQueued), q.partialResults.queued.Load())
	})
}

func TestPartialResultCacheFallback(t *testing.T) {
	f := newPartialResultFixture(t)
	plan := f.plan()
	uncached, _ := f.backend(t, nil, resultCacheOverrides{})
	query := functionTreeQuery([]string{}, &typesv1.FunctionTreeOptions{
		Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
		Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
		MaxDepth:  proto.Int32(4),
	})
	req := partialResultRequest(plan, partialResultTestSelector, query)
	want := invokeReports(t, uncached, req.CloneVT())
	identity, err := partialResultCacheIdentity(req)
	require.NoError(t, err)
	key, err := partialResultCacheObjectKey(0, identity)
	require.NoError(t, err)
	validTree := func() []byte {
		tree := model.NewFunctionStackTree()
		b, err := tree.Proto().MarshalVT()
		require.NoError(t, err)
		return newPartialResultCache(new(resultCache), log.NewNopLogger(), testPartialResultConfig().ResultCache.PartialResults, nil).encoder.EncodeAll(b, nil)
	}
	entry := func(mutate func(*queryv1.PartialResultCacheEntry)) []byte {
		e := &queryv1.PartialResultCacheEntry{Key: identity.CloneVT(), FunctionStackTree: validTree()}
		mutate(e)
		b, err := e.MarshalVT()
		require.NoError(t, err)
		return b
	}

	for _, tc := range []struct {
		name    string
		object  []byte
		outcome string
		keep    bool
	}{
		{name: "miss", outcome: "miss"},
		{name: "corrupt entry", object: []byte("not a protobuf entry"), outcome: "corrupt"},
		{name: "old version", object: entry(func(e *queryv1.PartialResultCacheEntry) { e.Key.Version = 0 }), outcome: "version_mismatch"},
		{name: "collision", object: entry(func(e *queryv1.PartialResultCacheEntry) { e.Key.Tenant = "other" }), outcome: "collision", keep: true},
		{name: "corrupt compression", object: entry(func(e *queryv1.PartialResultCacheEntry) { e.FunctionStackTree = []byte("garbage") }), outcome: "corrupt"},
		{name: "corrupt tree", object: entry(func(e *queryv1.PartialResultCacheEntry) {
			b, _ := (&queryv1.FunctionStackTree{Names: []string{""}, NodeName: []uint32{0}, NodeChildren: []uint32{1}, NodeSelf: []uint64{0}}).MarshalVT()
			e.FunctionStackTree = newPartialResultCache(new(resultCache), log.NewNopLogger(), testPartialResultConfig().ResultCache.PartialResults, nil).encoder.EncodeAll(b, nil)
		}), outcome: "corrupt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := objstore.NewBucket(thanosobjstore.NewInMemBucket())
			if tc.object != nil {
				require.NoError(t, cache.Upload(context.Background(), key, bytes.NewReader(tc.object)))
			}
			q, _ := f.backend(t, cache, resultCacheOverrides{partial: true})
			require.Equal(t, want, invokeReports(t, q, req.CloneVT()))
			require.Equal(t, float64(1), testutil.ToFloat64(q.partialResults.metrics.lookups.WithLabelValues("function_tree", tc.outcome)))
			if tc.object == nil {
				return
			}
			if tc.keep {
				ok, err := cache.Exists(context.Background(), key)
				require.NoError(t, err)
				require.True(t, ok, "entries of other identities are kept")
				return
			}
			require.Eventually(t, func() bool {
				ok, err := cache.Exists(context.Background(), key)
				return err == nil && !ok
			}, 10*time.Second, 10*time.Millisecond, "invalid entries are deleted so that they can be written again")
		})
	}

	t.Run("partial coverage", func(t *testing.T) {
		cache := objstore.NewBucket(thanosobjstore.NewInMemBucket())
		q, _ := f.backend(t, cache, resultCacheOverrides{partial: true})
		subset := queryplan.Build(f.blocks[:len(f.blocks)-1], 2, 2)
		writePartialResult(t, q, cache, partialResultRequest(subset, partialResultTestSelector, treeQuery(16)))
		require.Equal(t, want, invokeReports(t, q, req.CloneVT()))
		require.Equal(t, float64(1), testutil.ToFloat64(q.partialResults.metrics.lookups.WithLabelValues("function_tree", "miss")))
	})

	t.Run("lookup error", func(t *testing.T) {
		cache := &erroringGetBucket{Bucket: objstore.NewBucket(thanosobjstore.NewInMemBucket())}
		q, _ := f.backend(t, cache, resultCacheOverrides{partial: true})
		require.Equal(t, want, invokeReports(t, q, req.CloneVT()))
		require.Equal(t, float64(1), testutil.ToFloat64(q.partialResults.metrics.lookups.WithLabelValues("function_tree", "error")))
	})

	t.Run("slow lookup", func(t *testing.T) {
		release := make(chan struct{})
		defer close(release)
		cache := &delayedCacheBucket{Bucket: objstore.NewBucket(thanosobjstore.NewInMemBucket()), release: release}
		q, _ := f.backend(t, cache, resultCacheOverrides{partial: true})
		require.Equal(t, want, invokeReports(t, q, req.CloneVT()))
		require.Equal(t, float64(1), testutil.ToFloat64(q.partialResults.metrics.lookups.WithLabelValues("function_tree", "slow")))
	})
}

type erroringGetBucket struct{ objstore.Bucket }

func (b *erroringGetBucket) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("unavailable")
}

func TestPartialResultCacheRace(t *testing.T) {
	hit := &queryv1.InvokeResponse{Reports: []*queryv1.Report{{ReportType: queryv1.ReportType_REPORT_FUNCTIONS}}}
	executed := &queryv1.InvokeResponse{}
	newCache := func(delay time.Duration) *partialResultCache {
		return newPartialResultCache(new(resultCache), log.NewNopLogger(), PartialResultCacheConfig{MaxSizeBytes: 1 << 20, ExecutionDelay: delay}, nil)
	}
	t.Run("found entry delays execution", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var executions atomic.Int32
			resp, outcome, err := newCache(time.Millisecond).race(t.Context(),
				func(_ context.Context, found func()) (*queryv1.InvokeResponse, string) {
					found()
					time.Sleep(time.Second)
					return hit, "hit"
				},
				func(context.Context) (*queryv1.InvokeResponse, error) {
					executions.Add(1)
					return executed, nil
				})
			require.NoError(t, err)
			require.Equal(t, "hit", outcome)
			require.Same(t, hit, resp)
			require.Zero(t, executions.Load())
		})
	})
	t.Run("miss starts execution immediately", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			resp, outcome, err := newCache(time.Hour).race(t.Context(),
				func(context.Context, func()) (*queryv1.InvokeResponse, string) { return nil, "miss" },
				func(context.Context) (*queryv1.InvokeResponse, error) { return executed, nil })
			require.NoError(t, err)
			require.Equal(t, "miss", outcome)
			require.Same(t, executed, resp)
			require.Zero(t, time.Since(start))
		})
	})
	t.Run("late hit cancels execution", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var canceled atomic.Bool
			resp, outcome, err := newCache(time.Millisecond).race(t.Context(),
				func(context.Context, func()) (*queryv1.InvokeResponse, string) {
					time.Sleep(time.Second)
					return hit, "hit"
				},
				func(ctx context.Context) (*queryv1.InvokeResponse, error) {
					<-ctx.Done()
					canceled.Store(true)
					return nil, ctx.Err()
				})
			require.NoError(t, err)
			require.Equal(t, "hit", outcome)
			require.Same(t, hit, resp)
			require.True(t, canceled.Load(), "execution must be joined")
		})
	})
	t.Run("execution errors are returned", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			_, _, err := newCache(0).race(t.Context(),
				func(context.Context, func()) (*queryv1.InvokeResponse, string) { return nil, "miss" },
				func(context.Context) (*queryv1.InvokeResponse, error) { return nil, errors.New("boom") })
			require.EqualError(t, err, "boom")
		})
	})
}

func TestPartialResultCacheShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var active atomic.Int32
		bucket := &resultCacheWriterBucket{Bucket: objstore.NewBucket(thanosobjstore.NewInMemBucket())}
		bucket.upload = func(ctx context.Context, _ string, _ io.Reader) error {
			active.Add(1)
			defer active.Add(-1)
			<-ctx.Done()
			return ctx.Err()
		}
		q, err := New(testPartialResultConfig(), log.NewNopLogger(), nil, nil, nil, bucket, resultCacheOverrides{partial: true})
		require.NoError(t, err)
		require.NoError(t, q.starting(t.Context()))
		identity, err := partialResultCacheIdentity(partialResultKeyTestRequest(treeQuery(16)))
		require.NoError(t, err)
		tree := model.NewFunctionStackTree().Proto()
		for i := range partialResultCacheMaxQueued + 1 {
			reason := q.partialResults.enqueue(identity, strconv.Itoa(i), []*queryv1.FunctionStackTree{tree})
			if i < partialResultCacheMaxQueued {
				require.Empty(t, reason)
			} else {
				require.Equal(t, "queue_full", reason)
			}
		}
		synctest.Wait()
		require.Equal(t, int32(resultCacheWorkers), active.Load())
		// In-flight uploads reach their own timeout during shutdown; the
		// queued ones share the remaining shutdown budget.
		time.Sleep(10 * time.Second)

		start := time.Now()
		require.NoError(t, q.stopping(nil))
		require.Equal(t, resultCacheShutdownTimeout, time.Since(start))
		require.Zero(t, active.Load())
		require.True(t, bucket.closed.Load())
		require.Zero(t, q.partialResults.queued.Load())
		require.Equal(t, float64(partialResultCacheMaxQueued), testutil.ToFloat64(q.partialResults.metrics.writes.WithLabelValues("error")))
	})
}

func TestPartialResultCacheShutdownDropsQueuedWrites(t *testing.T) {
	q, err := New(testPartialResultConfig(), log.NewNopLogger(), nil, nil, nil, objstore.NewBucket(thanosobjstore.NewInMemBucket()), resultCacheOverrides{partial: true})
	require.NoError(t, err)
	identity, err := partialResultCacheIdentity(partialResultKeyTestRequest(treeQuery(16)))
	require.NoError(t, err)
	for i := range 3 {
		require.Empty(t, q.partialResults.enqueue(identity, strconv.Itoa(i), nil))
	}
	q.dropResultCacheWrites()
	require.Zero(t, q.partialResults.queued.Load())
	require.Equal(t, float64(3), testutil.ToFloat64(q.partialResults.metrics.skips.WithLabelValues("shutdown")))
}
