package integration

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
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
// forwarding MaxNodes (8192 by default via ValidateMaxNodes). The pprof
// resolver then picks pprofTree, which builds the truncation tree from
// each location's Line slice. Unsymbolized locations have an empty Line
// slice, so all unsymbolized stack traces collapse onto a single empty
// tree node and are reported as one "other" sample instead of remaining
// distinct. After post-query symbolization, the user sees a merged
// "other" function in the flamegraph rather than the individual function
// names.
//
// The test pushes an unsymbolized profile with two known addresses that
// the test debuginfod server resolves to "main" and "atoll_b", then asks
// for the flamegraph via SelectMergeStacktraces and asserts that both
// function names appear. On current main the flamegraph contains only
// "other" under "total".
func TestMicroServicesIntegrationV2Issue4789Reproducer(t *testing.T) {
	debuginfodServer, err := NewTestDebuginfodServer()
	require.NoError(t, err)

	_, currentFile, _, _ := runtime.Caller(0)
	testDataDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "symbolizer", "testdata")
	debugFilePath := filepath.Join(testDataDir, "symbols.debug")

	debuginfodServer.AddDebugFile(testBuildID, debugFilePath)

	require.NoError(t, debuginfodServer.Start())
	defer func() {
		_ = debuginfodServer.Stop()
	}()

	c := cluster.NewMicroServiceCluster(
		cluster.WithV2(),
		cluster.WithSymbolizer(debuginfodServer.URL()),
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
	m := &profile.Mapping{
		ID:      1,
		Start:   0,
		Limit:   0x1000000,
		Offset:  0,
		File:    "libfoo.so",
		BuildID: testBuildID,
	}
	src.Mapping = []*profile.Mapping{m}
	loc1 := &profile.Location{ID: 1, Mapping: m, Address: 0x1500}
	loc2 := &profile.Location{ID: 2, Mapping: m, Address: 0x3c5a}
	src.Location = []*profile.Location{loc1, loc2}
	src.Sample = []*profile.Sample{
		{Location: []*profile.Location{loc1}, Value: []int64{100}},
		{Location: []*profile.Location{loc2}, Value: []int64{200}},
		{Location: []*profile.Location{loc1, loc2}, Value: []int64{300}},
	}

	var buf bytes.Buffer
	require.NoError(t, src.Write(&buf))

	_, err = pusher.Push(ctx, connect.NewRequest(&pushv1.PushRequest{
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
		if !slices.Contains(fg.Names, "main") || !slices.Contains(fg.Names, "atoll_b") {
			t.Logf("flamegraph not yet symbolized: names=%v total=%d", fg.Names, fg.Total)
			return false
		}
		return true
	}, 10*time.Second, 500*time.Millisecond, "flamegraph never contained both resolved function names")

	t.Logf("flamegraph names: %v, total: %d", fg.Names, fg.Total)
	assert.Contains(t, fg.Names, "main")
	assert.Contains(t, fg.Names, "atoll_b")
	assert.NotContains(t, fg.Names, "other",
		"all unsymbolized stacks were merged into an 'other' node — see issue #4789")
	assert.Equal(t, int64(600), fg.Total)
}
