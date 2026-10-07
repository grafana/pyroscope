package integration

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/integration/cluster"
)

const testBuildID = "2fa2055ef20fabc972d5751147e093275514b142"

func TestMicroServicesIntegrationV2Symbolization(t *testing.T) {
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
	for _, comp := range c.Components {
		t.Log(comp.String())
	}

	require.NoError(t, c.Start(ctx))
	t.Log("Cluster ready")
	defer func() {
		waitStopped := c.Stop()
		require.NoError(t, waitStopped(ctx))
	}()

	t.Run("SymbolizationFlow", func(t *testing.T) {
		testSymbolizationFlow(t, ctx, c)
	})
}

func testSymbolizationFlow(t *testing.T, ctx context.Context, c *cluster.Cluster) {
	tests := []struct {
		name     string
		profile  func(now time.Time) *profile.Profile
		expected string
		// Symbolized frames that must still be present after the pushed
		// segment has been compacted; nil skips the post-compaction phase.
		postCompactionSymbols map[string]uint64
		skip                  bool
	}{
		{
			name: "fully unsymbolized",
			profile: func(now time.Time) *profile.Profile {
				p := &profile.Profile{
					DurationNanos: int64(10 * time.Second),
					Period:        1000000000,
					SampleType: []*profile.ValueType{
						{Type: "cpu", Unit: "nanoseconds"},
					},
					PeriodType: &profile.ValueType{
						Type: "cpu",
						Unit: "nanoseconds",
					},
				}

				m := &profile.Mapping{
					ID:           1,
					Start:        0,
					Limit:        0x1000000,
					Offset:       0,
					File:         "libfoo.so",
					BuildID:      testBuildID,
					HasFunctions: false,
				}
				p.Mapping = []*profile.Mapping{m}

				loc1 := &profile.Location{
					ID:      1,
					Mapping: m,
					Address: 0x1500,
				}
				loc2 := &profile.Location{
					ID:      2,
					Mapping: m,
					Address: 0x3c5a,
				}
				p.Location = []*profile.Location{loc1, loc2}

				p.Sample = []*profile.Sample{
					{
						Location: []*profile.Location{loc1},
						Value:    []int64{100},
					},
					{
						Location: []*profile.Location{loc2},
						Value:    []int64{200},
					},
					{
						Location: []*profile.Location{loc1, loc2},
						Value:    []int64{3},
					},
				}

				return p
			},
			expected: `PeriodType: cpu nanoseconds
Period: 1000000000
Samples:
cpu/nanoseconds[dflt]
        200: 1 
        100: 2 
          3: 2 1 
Locations
     1: 0x3c5a M=1 atoll_b :0:0 s=0()
     2: 0x1500 M=1 main :0:0 s=0()
Mappings
1: 0x0/0x1000000/0x0 libfoo.so 2fa2055ef20fabc972d5751147e093275514b142 [FN]
`,
			postCompactionSymbols: map[string]uint64{"main": 0x1500, "atoll_b": 0x3c5a},
		},
		{
			name: "partially symbolized",
			profile: func(now time.Time) *profile.Profile {
				p := &profile.Profile{
					DurationNanos: int64(10 * time.Second),
					Period:        1000000000,
					SampleType: []*profile.ValueType{
						{Type: "cpu", Unit: "nanoseconds"},
					},
					PeriodType: &profile.ValueType{
						Type: "cpu",
						Unit: "nanoseconds",
					},
				}

				m := &profile.Mapping{
					ID:           1,
					Start:        0,
					Limit:        0x1000000,
					Offset:       0,
					File:         "libfoo.so",
					BuildID:      testBuildID,
					HasFunctions: true,
				}
				p.Mapping = []*profile.Mapping{m}
				f1 := &profile.Function{
					ID:       1,
					Name:     "symbolized_func",
					Filename: "src.c",
				}
				loc1 := &profile.Location{
					ID:      1,
					Mapping: m,
					Address: 0x1500,
					Line:    []profile.Line{{Function: f1, Line: 239}},
				}
				loc2 := &profile.Location{
					ID:      2,
					Mapping: m,
					Address: 0x3c5a,
				}
				p.Location = []*profile.Location{loc1, loc2}

				p.Sample = []*profile.Sample{
					{
						Location: []*profile.Location{loc1},
						Value:    []int64{100},
					},
					{
						Location: []*profile.Location{loc2},
						Value:    []int64{200},
					},
					{
						Location: []*profile.Location{loc1, loc2},
						Value:    []int64{3},
					},
				}
				p.Function = []*profile.Function{
					f1,
				}

				return p
			},
			expected: `PeriodType: cpu nanoseconds
Period: 1000000000
Samples:
cpu/nanoseconds[dflt]
        200: 2 
          3: 1 2 
        100: 1 
Locations
     1: 0x0 M=1 symbolized_func src.c:239:0 s=0()
     2: 0x0 M=1 atoll_b :0:0 s=0()
Mappings
1: 0x0/0x0/0x0 libfoo.so 2fa2055ef20fabc972d5751147e093275514b142 [FN]
`,
			skip: true, // TODO fix the testdata or symbolization
		},
	}
	pusher := c.PushClient()
	querier := c.QueryClient()

	now := time.Now().Truncate(time.Second)
	tenantID := "test-tenant"

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.skip {
				t.Skip()
			}
			serviceName := "test-symbolization-service-" + test.name
			src := test.profile(now)

			var buf bytes.Buffer
			err := src.Write(&buf)
			require.NoError(t, err)
			rawProfile := buf.Bytes()

			ctx = tenant.InjectTenantID(ctx, tenantID)
			_, err = pusher.Push(ctx, connect.NewRequest(&pushv1.PushRequest{
				Series: []*pushv1.RawProfileSeries{{
					Labels: []*typesv1.LabelPair{
						{Name: "service_name", Value: serviceName},
						{Name: "__name__", Value: "process_cpu"},
					},
					Samples: []*pushv1.RawSample{{RawProfile: rawProfile}},
				}},
			}))
			require.NoError(t, err)

			q := connect.NewRequest(&querierv1.SelectMergeProfileRequest{
				ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
				Start:         now.Add(-time.Hour).UnixMilli(),
				End:           now.Add(time.Hour).UnixMilli(),
				LabelSelector: `{service_name="` + serviceName + `"}`,
			})
			require.Eventually(t, func() bool {
				resp, err := querier.SelectMergeProfile(ctx, q) //nolint:staticcheck // Legacy querier.v1 integration coverage.
				if err != nil {
					t.Logf("Error querying profile: %v", err)
					return false
				}
				normalized := normalizePprof(resp.Msg)
				rp := pprof.RawFromProto(normalized)
				rp.TimeNanos = 0
				actual := rp.DebugString()

				fmt.Println(actual)

				if len(normalized.Sample) == 0 {
					return false
				}

				if actual != test.expected {
					assert.Equal(t, test.expected, actual)
					//fmt.Println(src.String())
					return false
				}
				return true
			}, 5*time.Second, 100*time.Millisecond)

			if test.postCompactionSymbols == nil {
				return
			}
			// The cluster compacts L0 blocks within seconds, and compaction
			// deletes the source blocks: if the rewrite loses the addresses
			// of line-less locations, the profile can never be symbolized
			// again. The golden comparison above is order-sensitive and
			// compaction may renumber locations, so past this point only the
			// symbolized frames are asserted.
			// Age-based batch flushing is evaluated only when another block
			// arrives at the same compaction level, so keep pushing filler
			// blocks — to a service the query below does not select — until
			// the planner picks up the staged blocks.
			require.Eventually(t, func() bool {
				_, err := pusher.Push(ctx, connect.NewRequest(&pushv1.PushRequest{
					Series: []*pushv1.RawProfileSeries{{
						Labels: []*typesv1.LabelPair{
							{Name: "service_name", Value: serviceName + "-compaction-filler"},
							{Name: "__name__", Value: "process_cpu"},
						},
						Samples: []*pushv1.RawSample{{RawProfile: rawProfile}},
					}},
				}))
				if err != nil {
					t.Logf("Error pushing filler profile: %v", err)
					return false
				}
				n, err := c.CompactionJobsFinished(ctx)
				return err == nil && n >= 1
			}, 30*time.Second, time.Second, "no L0 compaction observed")

			queried := false
			for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
				resp, err := querier.SelectMergeProfile(ctx, q) //nolint:staticcheck // Legacy querier.v1 integration coverage.
				if err != nil {
					t.Logf("Error querying profile: %v", err)
					continue
				}
				requireSymbolizedFrames(t, resp.Msg, test.postCompactionSymbols)
				queried = true
			}
			require.True(t, queried, "profile was not queryable after compaction")
		})
	}

}

// requireSymbolizedFrames asserts that the profile contains a symbolized
// frame for every expected function name → address pair, and no fallback
// (binary!0xaddr) frames.
func requireSymbolizedFrames(t *testing.T, p *profilev1.Profile, symbols map[string]uint64) {
	t.Helper()
	names := make(map[uint64]string, len(p.Function))
	for _, f := range p.Function {
		names[f.Id] = p.StringTable[f.Name]
	}
	type frame struct {
		name    string
		address uint64
	}
	frames := make(map[frame]struct{})
	for _, loc := range p.Location {
		for _, line := range loc.Line {
			name := names[line.FunctionId]
			require.NotContains(t, name, "!0x", "unexpected fallback frame")
			frames[frame{name: name, address: loc.Address}] = struct{}{}
		}
	}
	for name, address := range symbols {
		_, ok := frames[frame{name: name, address: address}]
		require.True(t, ok, "missing symbolized frame %s@%#x (got: %v)", name, address, frames)
	}
}
