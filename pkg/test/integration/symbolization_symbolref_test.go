package integration

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/integration/cluster"
)

const processCPUProfileTypeID = "process_cpu:cpu:nanoseconds:cpu:nanoseconds"

// pushProfile pushes src as service_name=serviceName, __name__=process_cpu.
func pushProfile(t *testing.T, ctx context.Context, pusher pushv1connect.PusherServiceClient, src *profile.Profile, serviceName string) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, src.Write(&buf))
	_, err := pusher.Push(ctx, connect.NewRequest(&pushv1.PushRequest{
		Series: []*pushv1.RawProfileSeries{{
			Labels: []*typesv1.LabelPair{
				{Name: "service_name", Value: serviceName},
				{Name: "__name__", Value: "process_cpu"},
			},
			Samples: []*pushv1.RawSample{{RawProfile: buf.Bytes()}},
		}},
	}))
	require.NoError(t, err)
}

// buildSymbolizedTestProfile returns a profile that is already fully
// symbolized (every location has line info, so ingest never attaches
// __unsymbolized__ to its dataset): symbolized_leaf called from
// symbolized_root, self value 42.
func buildSymbolizedTestProfile() *profile.Profile {
	p := &profile.Profile{
		DurationNanos: int64(10 * time.Second),
		Period:        1000000000,
		SampleType:    []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}},
		PeriodType:    &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
	}
	m := &profile.Mapping{ID: 1, Start: 0, Limit: 0x1000000, File: "already-symbolized-binary", HasFunctions: true}
	p.Mapping = []*profile.Mapping{m}
	fRoot := &profile.Function{ID: 1, Name: "symbolized_root", Filename: "app.go"}
	fLeaf := &profile.Function{ID: 2, Name: "symbolized_leaf", Filename: "app.go"}
	p.Function = []*profile.Function{fRoot, fLeaf}
	locRoot := &profile.Location{ID: 1, Mapping: m, Line: []profile.Line{{Function: fRoot, Line: 10}}}
	locLeaf := &profile.Location{ID: 2, Mapping: m, Line: []profile.Line{{Function: fLeaf, Line: 20}}}
	p.Location = []*profile.Location{locRoot, locLeaf}
	// pprof samples list locations leaf-first.
	p.Sample = []*profile.Sample{
		{Location: []*profile.Location{locLeaf, locRoot}, Value: []int64{42}},
	}
	return p
}

// collectNames returns every distinct, non-empty frame name appearing
// anywhere in t's stacks (leaf or ancestor), dropping the marshal format's
// spurious zero-value frame.
func collectNames(t *phlaremodel.FunctionNameTree) []string {
	seen := make(map[string]struct{})
	t.IterateStacks(func(_ phlaremodel.FunctionName, _ int64, stack []phlaremodel.FunctionName) {
		for _, n := range stack {
			if n == "" {
				continue
			}
			seen[string(n)] = struct{}{}
		}
	})
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	return out
}

func containsAll(haystack []string, needles ...string) bool {
	set := make(map[string]struct{}, len(haystack))
	for _, h := range haystack {
		set[h] = struct{}{}
	}
	for _, n := range needles {
		if _, ok := set[n]; !ok {
			return false
		}
	}
	return true
}

// stacksByKey maps every stack in t to its accumulated self value, keyed by
// its frame names joined leaf-first (e.g. "leaf/root"), dropping the
// marshal format's spurious zero-value frame wherever it lands.
func stacksByKey(t *phlaremodel.FunctionNameTree) map[string]int64 {
	out := make(map[string]int64)
	t.IterateStacks(func(_ phlaremodel.FunctionName, self int64, stack []phlaremodel.FunctionName) {
		names := make([]string, 0, len(stack))
		for _, n := range stack {
			if n == "" {
				continue
			}
			names = append(names, string(n))
		}
		out[strings.Join(names, "/")] += self
	})
	return out
}

// stacksByKeyRootedAt filters stacksByKey(t) down to stacks whose root-most
// (last, since keys are leaf-first) frame is rootName.
func stacksByKeyRootedAt(t *phlaremodel.FunctionNameTree, rootName string) map[string]int64 {
	out := make(map[string]int64)
	suffix := "/" + rootName
	for k, v := range stacksByKey(t) {
		if k == rootName || strings.HasSuffix(k, suffix) {
			out[k] = v
		}
	}
	return out
}

func selectMergeStacktracesTree(ctx context.Context, t *testing.T, c *cluster.Cluster, req *querierv1.SelectMergeStacktracesRequest) (*phlaremodel.FunctionNameTree, error) {
	t.Helper()
	req.Format = querierv1.ProfileFormat_PROFILE_FORMAT_TREE
	resp, err := c.QueryClient().SelectMergeStacktraces(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](resp.Msg.Tree)
}

// TestMicroServicesIntegrationV2SymbolRefTrees_Queryless proves a
// queryless {} tree query, served via the tenant-index path (no
// service_name selector, so block metadata never carries the
// __unsymbolized__ label the old frontend gate depended on), is
// symbolized.
func TestMicroServicesIntegrationV2SymbolRefTrees_Queryless(t *testing.T) {
	ctx := context.Background()
	c := startSymbolizationCluster(t, ctx, cluster.WithSymbolRefTreesEnabled())

	now := time.Now().Truncate(time.Second)
	tenantID := "test-tenant-queryless"
	serviceName := "test-symbolref-queryless"
	tctx := tenant.InjectTenantID(ctx, tenantID)

	pushProfile(t, tctx, c.PushClient(), buildUnsymbolizedTestProfile(testBuildID), serviceName)

	req := &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: processCPUProfileTypeID,
		Start:         now.Add(-time.Hour).UnixMilli(),
		End:           now.Add(time.Hour).UnixMilli(),
		LabelSelector: `{}`, // no service_name: served via the tenant-wide datasets index.
	}

	var names []string
	require.Eventually(t, func() bool {
		tree, err := selectMergeStacktracesTree(tctx, t, c, req)
		if err != nil {
			t.Logf("query error: %v", err)
			return false
		}
		if tree.Total() == 0 {
			return false
		}
		names = collectNames(tree)
		return containsAll(names, "main", "atoll_b")
	}, 15*time.Second, 200*time.Millisecond)

	assert.Contains(t, names, "main")
	assert.Contains(t, names, "atoll_b")
	for _, n := range names {
		assert.NotContains(t, n, "0x", "queryless query must not leave raw addresses unsymbolized: got %q", n)
	}
}

// TestMicroServicesIntegrationV2SymbolRefTrees_SpanFiltered verifies that a
// span-selector tree query over unsymbolized data returns correctly
// symbolized frames for the matched span.
func TestMicroServicesIntegrationV2SymbolRefTrees_SpanFiltered(t *testing.T) {
	ctx := context.Background()
	c := startSymbolizationCluster(t, ctx, cluster.WithSymbolRefTreesEnabled())

	now := time.Now().Truncate(time.Second)
	tenantID := "test-tenant-span-filtered"
	serviceName := "test-symbolref-span-filtered"
	tctx := tenant.InjectTenantID(ctx, tenantID)

	const matchedSpanID = "0123456789abcdef"
	const otherSpanID = "fedcba9876543210"

	src := buildUnsymbolizedTestProfile(testBuildID)
	// sample[0] (loc1 alone, "main", value 100) carries the span under
	// test; the rest carry a different span, so the selector has something
	// to exclude.
	src.Sample[0].Label = map[string][]string{"span_id": {matchedSpanID}}
	src.Sample[1].Label = map[string][]string{"span_id": {otherSpanID}}
	src.Sample[2].Label = map[string][]string{"span_id": {otherSpanID}}
	pushProfile(t, tctx, c.PushClient(), src, serviceName)

	q := connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: processCPUProfileTypeID,
		Start:         now.Add(-time.Hour).UnixMilli(),
		End:           now.Add(time.Hour).UnixMilli(),
		LabelSelector: `{service_name="` + serviceName + `"}`,
		SpanSelector:  []string{matchedSpanID},
		Format:        querierv1.ProfileFormat_PROFILE_FORMAT_TREE,
	})

	var tree *phlaremodel.FunctionNameTree
	require.Eventually(t, func() bool {
		resp, err := c.QueryClient().SelectMergeStacktraces(tctx, q)
		if err != nil {
			t.Logf("query error: %v", err)
			return false
		}
		if len(resp.Msg.Tree) == 0 {
			return false
		}
		got, err := phlaremodel.UnmarshalTree[phlaremodel.FunctionName, phlaremodel.FunctionNameI](resp.Msg.Tree)
		if err != nil || got.Total() == 0 {
			return false
		}
		tree = got
		return true
	}, 15*time.Second, 200*time.Millisecond)

	assert.Equal(t, int64(100), tree.Total(), "only the span-matched sample's value must be counted")
	names := collectNames(tree)
	assert.Contains(t, names, "main", "the span-matched sample's location must be symbolized")
	assert.NotContains(t, names, "atoll_b", "samples outside the span selector must be excluded")
}

// TestMicroServicesIntegrationV2SymbolRefTrees_MixedDatasets proves that
// an already-symbolized service and an unsymbolized service in the same
// tenant, queried together with {}, both resolve correctly, and the
// symbolized service's stacks are unaffected by the unsymbolized one
// sharing the query/aggregation, matching a run with the feature entirely
// off.
func TestMicroServicesIntegrationV2SymbolRefTrees_MixedDatasets(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	symbolizedServiceName := "test-symbolref-mixed-symbolized"

	// Baseline: the same already-symbolized profile, queried with the
	// feature entirely absent, on its own cluster with no debuginfod/
	// symbolizer configured at all.
	baseline := cluster.NewMicroServiceCluster(cluster.WithV2())
	require.NoError(t, baseline.Prepare(ctx))
	require.NoError(t, baseline.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, baseline.Stop()(ctx))
	})
	baselineTenant := tenant.InjectTenantID(ctx, "test-tenant-mixed-baseline")
	pushProfile(t, baselineTenant, baseline.PushClient(), buildSymbolizedTestProfile(), symbolizedServiceName)

	var baselineStacks map[string]int64
	require.Eventually(t, func() bool {
		tree, err := selectMergeStacktracesTree(baselineTenant, t, baseline, &querierv1.SelectMergeStacktracesRequest{
			ProfileTypeID: processCPUProfileTypeID,
			Start:         now.Add(-time.Hour).UnixMilli(),
			End:           now.Add(time.Hour).UnixMilli(),
			LabelSelector: `{service_name="` + symbolizedServiceName + `"}`,
		})
		if err != nil || tree.Total() == 0 {
			return false
		}
		baselineStacks = stacksByKey(tree)
		return true
	}, 15*time.Second, 200*time.Millisecond)
	require.NotEmpty(t, baselineStacks)

	// Mixed cluster: the feature is on, and both a symbolized and an
	// unsymbolized dataset live under the same tenant.
	c := startSymbolizationCluster(t, ctx, cluster.WithSymbolRefTreesEnabled())
	tctx := tenant.InjectTenantID(ctx, "test-tenant-mixed")
	pushProfile(t, tctx, c.PushClient(), buildSymbolizedTestProfile(), symbolizedServiceName)
	pushProfile(t, tctx, c.PushClient(), buildUnsymbolizedTestProfile(testBuildID), "test-symbolref-mixed-unsymbolized")

	var mixedTree *phlaremodel.FunctionNameTree
	require.Eventually(t, func() bool {
		tree, err := selectMergeStacktracesTree(tctx, t, c, &querierv1.SelectMergeStacktracesRequest{
			ProfileTypeID: processCPUProfileTypeID,
			Start:         now.Add(-time.Hour).UnixMilli(),
			End:           now.Add(time.Hour).UnixMilli(),
			LabelSelector: `{}`,
		})
		if err != nil {
			t.Logf("query error: %v", err)
			return false
		}
		if tree.Total() == 0 {
			return false
		}
		names := collectNames(tree)
		if !containsAll(names, "symbolized_root", "symbolized_leaf", "main", "atoll_b") {
			return false
		}
		mixedTree = tree
		return true
	}, 15*time.Second, 200*time.Millisecond)

	mixedSymbolizedStacks := stacksByKeyRootedAt(mixedTree, "symbolized_root")
	assert.Equal(t, baselineStacks, mixedSymbolizedStacks,
		"the symbolized service's stacks must be identical whether or not an unsymbolized dataset shares the query")

	names := collectNames(mixedTree)
	assert.Contains(t, names, "main")
	assert.Contains(t, names, "atoll_b")
}

// TestMicroServicesIntegrationV2SymbolRefTrees_UnknownBuildID covers an
// unresolvable build ID (the mock debuginfod server 404s, since it was
// never registered): locations render the binary!0xaddr fallback end to
// end, exactly as an unresolvable build ID does on the legacy pprof-detour
// path.
func TestMicroServicesIntegrationV2SymbolRefTrees_UnknownBuildID(t *testing.T) {
	ctx := context.Background()
	c := startSymbolizationCluster(t, ctx, cluster.WithSymbolRefTreesEnabled())

	const unknownBuildID = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	now := time.Now().Truncate(time.Second)
	tenantID := "test-tenant-unknown-buildid"
	serviceName := "test-symbolref-unknown-buildid"
	tctx := tenant.InjectTenantID(ctx, tenantID)

	pushProfile(t, tctx, c.PushClient(), buildUnsymbolizedTestProfile(unknownBuildID), serviceName)

	req := &querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: processCPUProfileTypeID,
		Start:         now.Add(-time.Hour).UnixMilli(),
		End:           now.Add(time.Hour).UnixMilli(),
		LabelSelector: `{service_name="` + serviceName + `"}`,
	}

	// libfoo.so is the mapping's File; createFallbackSymbol renders
	// "{binary}!0x{addr:hex}" for a location it cannot resolve.
	const fallback1500 = "libfoo.so!0x1500"
	const fallback3c5a = "libfoo.so!0x3c5a"

	var names []string
	require.Eventually(t, func() bool {
		tree, err := selectMergeStacktracesTree(tctx, t, c, req)
		if err != nil {
			t.Logf("query error: %v", err)
			return false
		}
		if tree.Total() == 0 {
			return false
		}
		names = collectNames(tree)
		return containsAll(names, fallback1500, fallback3c5a)
	}, 15*time.Second, 200*time.Millisecond)

	assert.Contains(t, names, fallback1500)
	assert.Contains(t, names, fallback3c5a)
}
