package querybackend

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
)

func TestTreeResultCacheBoundsAndCopies(t *testing.T) {
	cache := newTreeResultCache(1000, nil)
	key := treeResultKey{object: "block/a", datasetOffset: 1}
	report := &queryv1.Report{ReportType: queryv1.ReportType_REPORT_TREE,
		Tree: &queryv1.TreeReport{Tree: []byte("original")}}
	cache.add(key, report)
	report.Tree.Tree[0] = 'X'
	got, ok := cache.get(key)
	require.True(t, ok)
	require.Equal(t, "original", string(got.Tree.Tree))
	got.Tree.Tree[0] = 'Y'
	again, ok := cache.get(key)
	require.True(t, ok)
	require.Equal(t, "original", string(again.Tree.Tree))

	// Concurrent misses may replace the same key; byte accounting must not
	// double count the replaced entry.
	cache.add(key, &queryv1.Report{ReportType: queryv1.ReportType_REPORT_TREE,
		Tree: &queryv1.TreeReport{Tree: []byte("new")}})
	require.LessOrEqual(t, cache.bytes, cache.maxBytes)
	require.Equal(t, 1, cache.lru.Len())
	cache.add(treeResultKey{object: "oversize"}, &queryv1.Report{ReportType: queryv1.ReportType_REPORT_TREE,
		Tree: &queryv1.TreeReport{Tree: make([]byte, 1001)}})
	require.Equal(t, 1, cache.lru.Len())
}

func TestTreeResultCacheEvictsToByteBudget(t *testing.T) {
	report := &queryv1.Report{ReportType: queryv1.ReportType_REPORT_TREE,
		Tree: &queryv1.TreeReport{Tree: make([]byte, 100)}}
	b, err := report.MarshalVT()
	require.NoError(t, err)
	cache := newTreeResultCache(int64(len(b))+1, nil)
	a := treeResultKey{object: "a"}
	z := treeResultKey{object: "z"}
	cache.add(a, report)
	cache.add(z, report)
	_, ok := cache.get(a)
	require.False(t, ok)
	_, ok = cache.get(z)
	require.True(t, ok)
	require.LessOrEqual(t, cache.bytes, cache.maxBytes)
}

func TestTreeResultCacheConcurrentReplacement(t *testing.T) {
	cache := newTreeResultCache(1<<20, nil)
	key := treeResultKey{object: "same-block", datasetOffset: 7}
	report := &queryv1.Report{ReportType: queryv1.ReportType_REPORT_TREE,
		Tree: &queryv1.TreeReport{Tree: []byte("immutable")}}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				cache.add(key, report)
				if got, ok := cache.get(key); ok {
					if string(got.Tree.Tree) != "immutable" {
						t.Errorf("unexpected cached result %q", got.Tree.Tree)
					}
				}
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 1, cache.lru.Len())
	require.LessOrEqual(t, cache.bytes, cache.maxBytes)
}

func (s *testSuite) Test_QueryTree_ResultCache() {
	s.reader.treeResults = newTreeResultCache(8<<20, nil)
	req := &queryv1.InvokeRequest{
		EndTime:       time.Now().UnixMilli(),
		LabelSelector: "{}",
		QueryPlan:     s.plan,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_TREE,
			Tree:      &queryv1.TreeQuery{MaxNodes: 16},
		}},
		Tenant: s.tenant,
	}
	first, err := s.reader.Invoke(context.Background(), req.CloneVT())
	s.Require().NoError(err)
	s.Require().Greater(testutil.ToFloat64(s.reader.treeResults.metrics.misses), float64(0))
	// Both windows fully cover the fixture datasets, so the shifted window
	// must reuse the same cached reports and produce the same result.
	shifted := req.CloneVT()
	shifted.StartTime++
	shifted.EndTime++
	second, err := s.reader.Invoke(context.Background(), shifted)
	s.Require().NoError(err)
	s.Require().True(proto.Equal(first.Reports[0], second.Reports[0]))
	s.Require().Greater(testutil.ToFloat64(s.reader.treeResults.metrics.hits), float64(0))
	// Format1 dataset-index resolution still reads storage on a cache hit;
	// cached tree datasets must nevertheless avoid their profile/symbol reads.
	s.Require().Less(second.GetDiagnostics().GetExecutionNode().GetStats().GetBytesFetched(),
		first.GetDiagnostics().GetExecutionNode().GetStats().GetBytesFetched())
	// A report that has been returned and possibly aggregated must not share
	// mutable protobuf state with the cache.
	first.Reports[0].Tree.Tree[0] ^= 0xff
	third, err := s.reader.Invoke(context.Background(), req.CloneVT())
	s.Require().NoError(err)
	s.Require().True(proto.Equal(second.Reports[0], third.Reports[0]))

	// Changing a result-affecting parameter must miss the old entry.
	misses := testutil.ToFloat64(s.reader.treeResults.metrics.misses)
	changed := req.CloneVT()
	changed.Query[0].Tree.MaxNodes = 8
	_, err = s.reader.Invoke(context.Background(), changed)
	s.Require().NoError(err)
	s.Require().Greater(testutil.ToFloat64(s.reader.treeResults.metrics.misses), misses)
}

func (s *testSuite) Test_QueryTree_ResultCacheKeySeparatesInputs() {
	md := s.meta[0]
	s.Require().NotEmpty(md.Datasets)
	req := &queryv1.InvokeRequest{
		Tenant: s.tenant, StartTime: 1, EndTime: 2, LabelSelector: "{}",
		Query: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: 16}}},
	}
	keyFor := func(r *queryv1.InvokeRequest) treeResultKey {
		b := &blockContext{ctx: context.Background(), obj: block.NewObject(nil, md), req: &request{src: r}}
		q := b.newQueryContext(md.Datasets[0])
		key, ok := treeResultCacheKey(q, r.Query[0])
		s.Require().True(ok)
		return key
	}
	base := keyFor(req)
	for _, mutate := range []func(*queryv1.InvokeRequest){
		func(r *queryv1.InvokeRequest) { r.Tenant = []string{"another-tenant"} },
		func(r *queryv1.InvokeRequest) { r.StartTime++ },
		func(r *queryv1.InvokeRequest) { r.EndTime++ },
		func(r *queryv1.InvokeRequest) { r.LabelSelector = `{service_name="other"}` },
		func(r *queryv1.InvokeRequest) { r.Query[0].Tree.MaxNodes++ },
		func(r *queryv1.InvokeRequest) { r.Query[0].Tree.SpanSelector = []string{"span"} },
		func(r *queryv1.InvokeRequest) { r.Options = &queryv1.InvokeOptions{SanitizeOnMerge: true} },
	} {
		changed := req.CloneVT()
		mutate(changed)
		s.Require().NotEqual(base, keyFor(changed))
	}
}

func (s *testSuite) Test_QueryTree_ResultCacheKeyNormalizesCoveredDatasetWindow() {
	md := s.meta[0].CloneVT()
	s.Require().NotEmpty(md.Datasets)
	md.Datasets[0].MinTime = 1000
	md.Datasets[0].MaxTime = 2000
	req := &queryv1.InvokeRequest{
		Tenant: s.tenant, StartTime: 500, EndTime: 2500, LabelSelector: "{}",
		Query: []*queryv1.Query{{QueryType: queryv1.QueryType_QUERY_TREE, Tree: &queryv1.TreeQuery{MaxNodes: 16}}},
	}
	keyFor := func(r *queryv1.InvokeRequest) treeResultKey {
		b := &blockContext{ctx: context.Background(), obj: block.NewObject(nil, md), req: &request{src: r}}
		q := b.newQueryContext(md.Datasets[0])
		key, ok := treeResultCacheKey(q, r.Query[0])
		s.Require().True(ok)
		return key
	}
	base := keyFor(req)
	covered := req.CloneVT()
	covered.StartTime = 999
	covered.EndTime = 2001
	s.Require().Equal(base, keyFor(covered))

	// MaxTime may be truncated from a nanosecond timestamp. EndTime at the
	// same millisecond cannot be assumed to include the final profile.
	partialEnd := req.CloneVT()
	partialEnd.EndTime = 2000
	s.Require().NotEqual(base, keyFor(partialEnd))
	partialStart := req.CloneVT()
	partialStart.StartTime = 1001
	s.Require().NotEqual(base, keyFor(partialStart))

	// Unknown bounds retain the original request window as the cache key.
	md.Datasets[0].MinTime = 0
	s.Require().NotEqual(keyFor(req), keyFor(covered))
}
