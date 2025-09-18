package integration

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	googlev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
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
	pusher := c.PushClient()
	querier := c.QueryClient()

	now := time.Now().Truncate(time.Second)
	tenantID := "test-tenant"
	serviceName := "test-symbolization-service"

	builder := testhelper.NewProfileBuilder(now.UnixNano()).
		CPUProfile().
		WithLabels(
			"service_name", serviceName,
		)

	builder.ForStacktraceString("placeholder").AddSamples(100)

	profile := builder.Profile

	buildIDIdx := int64(len(profile.StringTable))
	profile.StringTable = append(profile.StringTable, testBuildID)

	profile.Mapping[0].BuildId = buildIDIdx
	profile.Mapping[0].HasFunctions = false
	profile.Mapping[0].MemoryStart = 0x0
	profile.Mapping[0].MemoryLimit = 0x1000000
	profile.Mapping[0].FileOffset = 0x0

	profile.Location = []*googlev1.Location{
		{
			Id:        1,
			MappingId: 1,
			Address:   0x1500,
		},
		{
			Id:        2,
			MappingId: 1,
			Address:   0x3c5a,
		},
	}

	profile.Sample = []*googlev1.Sample{
		{
			LocationId: []uint64{1},
			Value:      []int64{100},
		},
		{
			LocationId: []uint64{2},
			Value:      []int64{200},
		},
	}

	profile.Function = nil
	rawProfile, err := profile.MarshalVT()
	require.NoError(t, err)

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

	time.Sleep(5 * time.Second)

	resp, err := querier.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		Start:         now.Add(-time.Hour).UnixMilli(),
		End:           now.Add(time.Hour).UnixMilli(),
		LabelSelector: `{service_name="` + serviceName + `"}`,
	}))
	require.NoError(t, err, "Failed to query profile")
	require.NotNil(t, resp.Msg, "Response message is nil")

	require.Len(t, resp.Msg.Mapping, 1)
	assert.True(t, resp.Msg.Mapping[0].HasFunctions, "Mapping should have HasFunctions=true after symbolization")

	foundMain := false
	foundAtollB := false

	for _, fn := range resp.Msg.Function {
		if fn.Name > 0 && fn.Name < int64(len(resp.Msg.StringTable)) {
			functionName := resp.Msg.StringTable[fn.Name]
			if functionName == "main" {
				foundMain = true
			}
			if functionName == "atoll_b" {
				foundAtollB = true
			}
		}
	}

	assert.True(t, foundMain && foundAtollB, "Expected to find both symbolized function names (main and atoll_b) in profile")
	t.Log("Symbolization successful! Found both 'main' and 'atoll_b' functions")
}
