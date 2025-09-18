package integration

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/pprof/profile"
	"github.com/grafana/pyroscope/pkg/pprof"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/test/integration/cluster"
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
	}{
		{
			name: "fully unsymbolized",
			profile: func(now time.Time) *profile.Profile {
				p := &profile.Profile{
					TimeNanos:     now.UnixNano(),
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
        200: 2 
          3: 1 2 
        100: 1 
Locations
     1: 0x1500 M=1 main :0:0 s=0()
     2: 0x3c5a M=1 atoll_b :0:0 s=0()
Mappings
1: 0x0/0x1000000/0x0 libfoo.so 2fa2055ef20fabc972d5751147e093275514b142 [FN]
`,
		},
	}
	pusher := c.PushClient()
	querier := c.QueryClient()

	now := time.Now().Truncate(time.Second)
	tenantID := "test-tenant"

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serviceName := "test-symbolization-service-" + test.name
			p := test.profile(now)

			var buf bytes.Buffer
			err := p.Write(&buf)
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
				resp, err := querier.SelectMergeProfile(ctx, q)
				if err != nil {
					t.Logf("Error querying profile: %v", err)
					return false
				}

				p := pprof.RawFromProto(resp.Msg)
				p.TimeNanos = 0
				s := p.DebugString()

				if s != test.expected {
					assert.Equal(t, test.expected, s)
					//t.Logf("Expected:\n%s\nGot:\n%s", test.expected, s)
					return false
				}
				return true
			}, 5*time.Second, 100*time.Millisecond)
		})
	}

}
