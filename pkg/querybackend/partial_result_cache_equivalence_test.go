package querybackend

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	thanosobjstore "github.com/thanos-io/objstore"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend/profilediff"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

const (
	// After the last profile of the fixture blocks.
	partialResultTestEnd      = int64(1739265000000)
	partialResultTestTenant   = "anonymous"
	partialResultTestSelector = `{service_name="test-app",__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`
)

type readCountingBucket struct {
	thanosobjstore.Bucket
	reads atomic.Int64
}

func (b *readCountingBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	b.reads.Add(1)
	return b.Bucket.Get(ctx, name)
}

func (b *readCountingBucket) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	b.reads.Add(1)
	return b.Bucket.GetRange(ctx, name, off, length)
}

// roundTripHandler serializes requests and responses, as gRPC does between
// query-backends, so that no state is shared between plan nodes.
type roundTripHandler struct{ QueryHandler }

func (h roundTripHandler) Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	b, err := req.MarshalVT()
	if err != nil {
		return nil, err
	}
	r := new(queryv1.InvokeRequest)
	if err = r.UnmarshalVT(b); err != nil {
		return nil, err
	}
	resp, err := h.QueryHandler.Invoke(ctx, r)
	if err != nil {
		return nil, err
	}
	if b, err = resp.MarshalVT(); err != nil {
		return nil, err
	}
	out := new(queryv1.InvokeResponse)
	return out, out.UnmarshalVT(b)
}

type partialResultFixture struct {
	primary thanosobjstore.Bucket
	blocks  []*metastorev1.BlockMeta
}

func newPartialResultFixture(t *testing.T) *partialResultFixture {
	s := &testSuite{dir: "testdata/samples"}
	s.SetT(t)
	s.SetupSuite()
	s.SetupTest()
	return &partialResultFixture{primary: s.bucket, blocks: s.meta}
}

// plan builds a multi-level plan: MERGE nodes over several READ nodes.
func (f *partialResultFixture) plan() *queryv1.QueryPlan {
	return queryplan.Build(f.blocks, 2, 2)
}

func testPartialResultConfig() Config {
	return Config{ResultCache: ResultCacheConfig{PartialResults: PartialResultCacheConfig{
		MaxSizeBytes:   32 << 20,
		ExecutionDelay: 100 * time.Millisecond,
	}}}
}

func (f *partialResultFixture) backend(t *testing.T, cache objstore.Bucket, overrides ResultCacheOverrides) (*QueryBackend, *readCountingBucket) {
	t.Helper()
	primary := &readCountingBucket{Bucket: f.primary}
	return newPartialResultBackend(t, testPartialResultConfig(), primary, cache, overrides), primary
}

func newPartialResultBackend(t *testing.T, cfg Config, primary thanosobjstore.Bucket, cache objstore.Bucket, overrides ResultCacheOverrides) *QueryBackend {
	t.Helper()
	reader := NewBlockReader(log.NewNopLogger(), &objstore.ReaderAtBucket{Bucket: primary}, nil, validation.MockDefaultOverrides())
	q, err := New(cfg, log.NewNopLogger(), nil, nil, reader, cache, overrides)
	require.NoError(t, err)
	q.backendClient = roundTripHandler{q}
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), q.Service()))
	t.Cleanup(func() { _ = services.StopAndAwaitTerminated(context.Background(), q.Service()) })
	return q
}

func partialResultRequest(plan *queryv1.QueryPlan, selector string, query *queryv1.Query) *queryv1.InvokeRequest {
	return &queryv1.InvokeRequest{
		Tenant:        []string{partialResultTestTenant},
		StartTime:     0,
		EndTime:       partialResultTestEnd,
		LabelSelector: selector,
		Query:         []*queryv1.Query{query},
		QueryPlan:     plan.CloneVT(),
	}
}

func treeQuery(maxNodes int64) *queryv1.Query {
	return &queryv1.Query{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: maxNodes}}
}

func functionsQuery(path []string) *queryv1.Query {
	return &queryv1.Query{QueryType: queryv1.QueryType_QUERY_FUNCTIONS, Functions: &queryv1.FunctionsQuery{StackTraceSelector: callSiteSelector(path)}}
}

func functionTreeQuery(path []string, o *typesv1.FunctionTreeOptions) *queryv1.Query {
	return &queryv1.Query{QueryType: queryv1.QueryType_QUERY_FUNCTION_TREE, FunctionTree: &queryv1.FunctionTreeQuery{StackTraceSelector: callSiteSelector(path), Options: o}}
}

func callSiteSelector(path []string) *typesv1.StackTraceSelector {
	s := new(typesv1.StackTraceSelector)
	for _, name := range path {
		s.CallSite = append(s.CallSite, &typesv1.Location{Name: name})
	}
	return s
}

func invokeReports(t *testing.T, q QueryHandler, req *queryv1.InvokeRequest) []byte {
	t.Helper()
	resp, err := q.Invoke(context.Background(), req)
	require.NoError(t, err)
	require.Nil(t, resp.PartialResult, "partial results must not leave the plan root")
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(&queryv1.QueryResponse{Reports: resp.Reports})
	require.NoError(t, err)
	return b
}

// writePartialResult runs a truncated flame graph query and waits for its entry.
func writePartialResult(t *testing.T, q *QueryBackend, cache objstore.Bucket, req *queryv1.InvokeRequest) {
	t.Helper()
	identity, err := partialResultCacheIdentity(req)
	require.NoError(t, err)
	key, err := partialResultCacheObjectKey(0, identity)
	require.NoError(t, err)
	_, err = q.Invoke(context.Background(), req)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		ok, err := cache.Exists(context.Background(), key)
		return err == nil && ok
	}, 10*time.Second, 10*time.Millisecond)
}

func fixtureStacks(t *testing.T, q QueryHandler, plan *queryv1.QueryPlan, selector string) [][]string {
	t.Helper()
	resp, err := q.Invoke(context.Background(), partialResultRequest(plan, selector, treeQuery(0)))
	require.NoError(t, err)
	require.Len(t, resp.Reports, 1)
	tree, err := model.UnmarshalTree[model.FunctionName, model.FunctionNameI](resp.Reports[0].Tree.Tree)
	require.NoError(t, err)
	var stacks [][]string
	tree.IterateStacks(func(_ model.FunctionName, _ int64, stack []model.FunctionName) {
		names := make([]string, 0, len(stack))
		for _, name := range slices.Backward(stack) {
			names = append(names, string(name))
		}
		// UnmarshalTree retains the serialized virtual root in parent pointers.
		if len(names) > 0 && names[0] == "" {
			names = names[1:]
		}
		stacks = append(stacks, names)
	})
	require.NotEmpty(t, stacks)
	return stacks
}

func projectionQueries(stacks [][]string, seed int64) []*queryv1.Query {
	rng := rand.New(rand.NewSource(seed))
	depths := []*int32{nil, proto.Int32(0), proto.Int32(1), proto.Int32(2), proto.Int32(16)}
	var queries []*queryv1.Query
	addTree := func(path []string, direction typesv1.FunctionTreeDirection, selection typesv1.FunctionTreeSelection) {
		for _, depth := range depths {
			queries = append(queries, functionTreeQuery(path, &typesv1.FunctionTreeOptions{Direction: direction, Selection: selection, MaxDepth: depth}))
		}
	}
	queries = append(queries, functionsQuery(nil), functionsQuery([]string{"missing"}))
	addTree(nil, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH)
	addTree([]string{"missing"}, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN)
	for range 8 {
		stack := stacks[rng.Intn(len(stacks))]
		if len(stack) == 0 {
			continue
		}
		prefix := stack[:1+rng.Intn(min(len(stack), 4))]
		queries = append(queries, functionsQuery(prefix))
		addTree(prefix, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH)
		start := rng.Intn(len(stack))
		chain := stack[start : start+1+rng.Intn(min(len(stack)-start, 3))]
		addTree(chain, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN)
		addTree(chain, typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN)
		addTree(chain[len(chain)-1:], typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH, typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN)
	}
	return queries
}

func TestPartialResultCacheEquivalence(t *testing.T) {
	f := newPartialResultFixture(t)
	plan := f.plan()
	require.Equal(t, queryv1.QueryNode_MERGE, plan.Root.Type)
	require.Equal(t, queryv1.QueryNode_MERGE, plan.Root.Children[0].Type, "the plan must have intermediate merge nodes")
	cache := objstore.NewBucket(thanosobjstore.NewInMemBucket())
	cached, primary := f.backend(t, cache, resultCacheOverrides{partial: true})
	uncached, _ := f.backend(t, nil, resultCacheOverrides{})

	for _, selector := range []string{partialResultTestSelector, `{__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`} {
		t.Run(selector, func(t *testing.T) {
			writePartialResult(t, cached, cache, partialResultRequest(plan, selector, treeQuery(16)))
			queries := projectionQueries(fixtureStacks(t, uncached, plan, selector), 1)
			hits := map[string]float64{}
			for _, qt := range []string{"functions", "function_tree"} {
				hits[qt] = testutil.ToFloat64(cached.partialResults.metrics.lookups.WithLabelValues(qt, "hit"))
			}
			reads := primary.reads.Load()
			for i, query := range queries {
				want := invokeReports(t, uncached, partialResultRequest(plan, selector, query))
				got := invokeReports(t, cached, partialResultRequest(plan, selector, query))
				require.Equal(t, want, got, "query %d: %v", i, query)
			}
			require.Equal(t, reads, primary.reads.Load(), "hits must not read blocks")
			var n float64
			for _, qt := range []string{"functions", "function_tree"} {
				n += testutil.ToFloat64(cached.partialResults.metrics.lookups.WithLabelValues(qt, "hit")) - hits[qt]
			}
			require.Equal(t, float64(len(queries)), n, "every projection must be answered from the cache")
		})
	}
}

func TestPartialResultCacheHitSkipsBlockReads(t *testing.T) {
	f := newPartialResultFixture(t)
	plan := f.plan()
	cache := objstore.NewBucket(thanosobjstore.NewInMemBucket())
	cached, primary := f.backend(t, cache, resultCacheOverrides{partial: true})
	writePartialResult(t, cached, cache, partialResultRequest(plan, partialResultTestSelector, treeQuery(16)))

	reads := primary.reads.Load()
	resp, err := cached.Invoke(context.Background(), partialResultRequest(plan, partialResultTestSelector, functionTreeQuery([]string{}, &typesv1.FunctionTreeOptions{
		Direction: typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES,
		Selection: typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
		MaxDepth:  proto.Int32(3),
	})))
	require.NoError(t, err)
	require.Equal(t, reads, primary.reads.Load(), "a hit must not read blocks")
	require.NotNil(t, resp.Reports[0].FunctionTree.Tree.Callees.Root)
	require.Zero(t, resp.Diagnostics.ExecutionNode.Stats.BytesFetched)
	require.Equal(t, float64(1), testutil.ToFloat64(cached.partialResults.metrics.lookups.WithLabelValues("function_tree", "hit")))

	// The same base query with different projections shares the entry; a
	// different base query does not.
	_, err = cached.Invoke(context.Background(), partialResultRequest(plan, `{service_name="pyroscope",__profile_type__="process_cpu:cpu:nanoseconds:cpu:nanoseconds"}`, functionsQuery(nil)))
	require.NoError(t, err)
	require.Equal(t, float64(1), testutil.ToFloat64(cached.partialResults.metrics.lookups.WithLabelValues("functions", "miss")))
	require.Greater(t, primary.reads.Load(), reads)
}

func TestPartialResultCacheDiff(t *testing.T) {
	f := newPartialResultFixture(t)
	plan := f.plan()
	cache := objstore.NewBucket(thanosobjstore.NewInMemBucket())
	cached, _ := f.backend(t, cache, resultCacheOverrides{partial: true})
	uncached, _ := f.backend(t, nil, resultCacheOverrides{})
	const profileType = "process_cpu:cpu:nanoseconds:cpu:nanoseconds"
	left := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: profileType, LabelSelector: `{service_name="test-app"}`, End: partialResultTestEnd}
	right := &querierv1.SelectMergeStacktracesRequest{ProfileTypeID: profileType, LabelSelector: `{service_name="pyroscope"}`, End: partialResultTestEnd - 1}
	selector := func(r *querierv1.SelectMergeStacktracesRequest) string {
		return fmt.Sprintf(`%s,__profile_type__=%q}`, r.LabelSelector[:len(r.LabelSelector)-1], r.ProfileTypeID)
	}
	for _, side := range []*querierv1.SelectMergeStacktracesRequest{left, right} {
		req := partialResultRequest(plan, selector(side), treeQuery(16))
		req.EndTime = side.End
		writePartialResult(t, cached, cache, req)
	}
	// Mirrors the query frontend: each diff side is an independent projection
	// query, with the format and options of the left side.
	selectFunc := func(q QueryHandler) profilediff.SelectFunc {
		return func(ctx context.Context, c *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
			r := c.Msg
			var query *queryv1.Query
			if r.Format == querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS {
				query = &queryv1.Query{QueryType: queryv1.QueryType_QUERY_FUNCTIONS, Functions: &queryv1.FunctionsQuery{StackTraceSelector: r.StackTraceSelector}}
			} else {
				query = functionTreeQuery(nil, r.GetFormatOptions().GetFunctionTree())
				query.FunctionTree.StackTraceSelector = r.StackTraceSelector
			}
			req := partialResultRequest(plan, selector(r), query)
			req.EndTime = r.End
			resp, err := q.Invoke(ctx, req)
			if err != nil {
				return nil, err
			}
			out := new(querierv1.SelectMergeStacktracesResponse)
			if len(resp.Reports) > 0 {
				out.Functions = resp.Reports[0].GetFunctions().GetTable()
				out.FunctionTree = resp.Reports[0].GetFunctionTree().GetTree()
			}
			return connect.NewResponse(out), nil
		}
	}
	stacks := fixtureStacks(t, uncached, plan, selector(left))
	hits := testutil.ToFloat64(cached.partialResults.metrics.lookups.WithLabelValues("function_tree", "hit")) +
		testutil.ToFloat64(cached.partialResults.metrics.lookups.WithLabelValues("functions", "hit"))
	var diffs int
	for _, query := range projectionQueries(stacks, 2) {
		l, r := left.CloneVT(), right.CloneVT()
		if query.QueryType == queryv1.QueryType_QUERY_FUNCTIONS {
			l.Format = querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS
			l.StackTraceSelector = query.Functions.StackTraceSelector
			l.FormatOptions = &querierv1.ProfileFormatOptions{Functions: &querierv1.FunctionsOptions{Limit: 5}}
		} else {
			l.Format = querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTION_TREE
			l.StackTraceSelector = query.FunctionTree.StackTraceSelector
			l.FormatOptions = &querierv1.ProfileFormatOptions{FunctionTree: query.FunctionTree.Options}
		}
		r.StackTraceSelector = l.StackTraceSelector
		req := &querierv1.DiffRequest{Left: l, Right: r}
		want, err := profilediff.Diff(context.Background(), req.CloneVT(), selectFunc(uncached))
		require.NoError(t, err)
		got, err := profilediff.Diff(context.Background(), req.CloneVT(), selectFunc(cached))
		require.NoError(t, err)
		wantBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(want.Msg)
		require.NoError(t, err)
		gotBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(got.Msg)
		require.NoError(t, err)
		require.Equal(t, wantBytes, gotBytes, "diff %v", query)
		diffs++
	}
	got := testutil.ToFloat64(cached.partialResults.metrics.lookups.WithLabelValues("function_tree", "hit")) +
		testutil.ToFloat64(cached.partialResults.metrics.lookups.WithLabelValues("functions", "hit")) - hits
	require.Equal(t, float64(2*diffs), got, "both diff sides must be answered from the cache")
}

func TestPartialResultCacheReadRoot(t *testing.T) {
	f := newPartialResultFixture(t)
	plan := queryplan.Build(f.blocks, len(f.blocks), 2)
	require.Equal(t, queryv1.QueryNode_READ, plan.Root.Type)
	cache := objstore.NewBucket(thanosobjstore.NewInMemBucket())
	cached, primary := f.backend(t, cache, resultCacheOverrides{partial: true})
	uncached, _ := f.backend(t, nil, resultCacheOverrides{})
	writePartialResult(t, cached, cache, partialResultRequest(plan, partialResultTestSelector, treeQuery(16)))
	reads := primary.reads.Load()
	for _, query := range projectionQueries(fixtureStacks(t, uncached, plan, partialResultTestSelector), 3) {
		require.Equal(t,
			invokeReports(t, uncached, partialResultRequest(plan, partialResultTestSelector, query)),
			invokeReports(t, cached, partialResultRequest(plan, partialResultTestSelector, query)), "%v", query)
	}
	require.Equal(t, reads, primary.reads.Load())
}
