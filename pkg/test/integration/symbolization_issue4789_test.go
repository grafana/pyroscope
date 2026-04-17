package integration

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/test/integration/cluster"
)

// TestMicroServicesIntegrationV2Issue4789Reproducer reproduces
// https://github.com/grafana/pyroscope/issues/4789.
//
// When a flamegraph query hits blocks with unsymbolized profiles, the
// query-frontend rewrites the tree query into a pprof query (see
// backendTreeSymbolizer in pkg/frontend/readpath/queryfrontend/symbolizer.go)
// and forwards MaxNodes (8192 by default via ValidateMaxNodes). The pprof
// resolver then picks pprofTree, which builds its truncation tree from
// each location's Line slice. Unsymbolized locations have an empty Line
// slice, so every unsymbolized stack trace collapses onto a single empty
// tree node and ends up in the "fully truncated" bucket as one merged
// "other" sample. After post-query symbolization the user sees a single
// "other" node where the distinct unsymbolized stacks used to be.
//
// The profile pushed here mirrors the one in the issue: one fully
// symbolized stack (foo, bar) and two fully unsymbolized stacks with
// addresses in library.so (no BuildID, so the symbolizer cannot resolve
// them). On a fixed code path, the flamegraph preserves all three stacks
// and "other" does not appear. On current main, the two unsymbolized
// stacks collapse and "other" is present.
func TestMicroServicesIntegrationV2Issue4789Reproducer(t *testing.T) {
	// The symbolizer has to be enabled so that the query-frontend wraps
	// the backend with backendTreeSymbolizer — that wrapper is what
	// rewrites QUERY_TREE into QUERY_PPROF and triggers the buggy path.
	// The URL below is never contacted: our mappings carry no BuildID,
	// so the symbolizer short-circuits to fallback symbols without any
	// network call (see Symbolizer.symbolize, empty_build_id branch).
	c := cluster.NewMicroServiceCluster(
		cluster.WithV2(),
		cluster.WithSymbolizer("http://127.0.0.1:1"),
	)

	ctx := context.Background()
	require.NoError(t, c.Prepare(ctx))
	require.NoError(t, c.Start(ctx))
	t.Log("Cluster ready")
	defer func() {
		waitStopped := c.Stop()
		require.NoError(t, waitStopped(ctx))
	}()

	pusher := c.PushClient()
	querier := c.QueryClient()

	const (
		serviceName = "test-issue-4789-service"
		tenantID    = "test-tenant"
	)
	ctx = tenant.InjectTenantID(ctx, tenantID)

	src := &profile.Profile{
		DurationNanos: int64(10 * time.Second),
		Period:        1_000_000_000,
		SampleType: []*profile.ValueType{
			{Type: "cpu", Unit: "nanoseconds"},
		},
		PeriodType: &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
	}
	// Mirrors the profile from issue #4789: one empty mapping that
	// owns the symbolized foo/bar locations, plus a library.so
	// mapping with HasFunctions=false that owns the unsymbolized
	// address-only locations. No BuildID, so the symbolizer cannot
	// resolve library.so's addresses.
	mSymbolized := &profile.Mapping{ID: 1}
	mUnsymbolized := &profile.Mapping{ID: 2, File: "library.so"}
	src.Mapping = []*profile.Mapping{mSymbolized, mUnsymbolized}

	foo := &profile.Function{ID: 1, Name: "foo"}
	bar := &profile.Function{ID: 2, Name: "bar"}
	src.Function = []*profile.Function{foo, bar}

	locFoo := &profile.Location{ID: 1, Mapping: mSymbolized, Line: []profile.Line{{Function: foo}}}
	locBar := &profile.Location{ID: 2, Mapping: mSymbolized, Line: []profile.Line{{Function: bar}}}
	locA := &profile.Location{ID: 3, Mapping: mUnsymbolized, Address: 0xcafe03000}
	locB := &profile.Location{ID: 4, Mapping: mUnsymbolized, Address: 0xcafe04000}
	locC := &profile.Location{ID: 5, Mapping: mUnsymbolized, Address: 0xcafe05000}
	locD := &profile.Location{ID: 6, Mapping: mUnsymbolized, Address: 0xcafe06000}
	src.Location = []*profile.Location{locFoo, locBar, locA, locB, locC, locD}

	src.Sample = []*profile.Sample{
		{Location: []*profile.Location{locFoo, locBar}, Value: []int64{1_000_000_000}},
		{Location: []*profile.Location{locA, locB}, Value: []int64{1_000_000_000}},
		{Location: []*profile.Location{locC, locD}, Value: []int64{1_000_000_000}},
	}

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

	now := time.Now()
	q := connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		Start:         now.Add(-time.Hour).UnixMilli(),
		End:           now.Add(time.Hour).UnixMilli(),
		LabelSelector: `{service_name="` + serviceName + `"}`,
	})

	// Unsymbolized locations get a fallback name of "<binary>!0x<addr>"
	// from the symbolizer (see createFallbackSymbol). We expect all four
	// library.so addresses to appear in the flamegraph on a fixed code
	// path; on current main they are lost because the backend collapses
	// them into a single "other" sample before the symbolizer runs.
	wantStubs := []string{
		"library.so!0xcafe03000",
		"library.so!0xcafe04000",
		"library.so!0xcafe05000",
		"library.so!0xcafe06000",
	}

	var fg *querierv1.FlameGraph
	require.Eventually(t, func() bool {
		resp, err := querier.SelectMergeStacktraces(ctx, q)
		if err != nil {
			t.Logf("query error: %v", err)
			return false
		}
		fg = resp.Msg.GetFlamegraph()
		if fg == nil || fg.Total == 0 {
			return false
		}
		// Wait until the symbolized part of the profile has
		// propagated through ingestion and into the query response.
		if !slices.Contains(fg.Names, "foo") || !slices.Contains(fg.Names, "bar") {
			t.Logf("flamegraph not yet populated: names=%v total=%d", fg.Names, fg.Total)
			return false
		}
		return true
	}, 10*time.Second, 500*time.Millisecond, "flamegraph never contained the symbolized function names")

	t.Logf("flamegraph names: %v, total: %d", fg.Names, fg.Total)
	assert.Contains(t, fg.Names, "foo")
	assert.Contains(t, fg.Names, "bar")
	for _, stub := range wantStubs {
		assert.Contains(t, fg.Names, stub,
			"unsymbolized fallback stub missing — the two unsymbolized stacks were merged in the backend before symbolization, see issue #4789")
	}
	assert.NotContains(t, fg.Names, "other",
		"the two unsymbolized stacks were merged into a single 'other' node — see issue #4789")
	assert.Equal(t, int64(3_000_000_000), fg.Total)
}
