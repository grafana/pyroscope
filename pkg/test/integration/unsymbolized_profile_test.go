package integration

import (
	"bytes"
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/require"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/test/integration/cluster"
)

const unsymbolizedBuildID = "2fa2055ef20fabc972d5751147e093275514b142"

// Native profiles can arrive with addresses only. Their addresses and build
// IDs must survive ingestion and compaction so they can still be symbolized
// outside Pyroscope.
func TestMicroServicesIntegrationV2UnsymbolizedProfile(t *testing.T) {
	c := cluster.NewMicroServiceCluster(cluster.WithV2())
	ctx := context.Background()
	require.NoError(t, c.Prepare(ctx))
	require.NoError(t, c.Start(ctx))
	defer func() {
		waitStopped := c.Stop()
		require.NoError(t, waitStopped(ctx))
	}()

	var buf bytes.Buffer
	require.NoError(t, unsymbolizedProfile().Write(&buf))
	rawProfile := buf.Bytes()

	ctx = tenant.InjectTenantID(ctx, "test-tenant")
	pusher := c.PushClient()
	querier := c.QueryClient()
	push := func(serviceName string) error {
		_, err := pusher.Push(ctx, connect.NewRequest(&pushv1.PushRequest{
			Series: []*pushv1.RawProfileSeries{{
				Labels: []*typesv1.LabelPair{
					{Name: "service_name", Value: serviceName},
					{Name: "__name__", Value: "process_cpu"},
				},
				Samples: []*pushv1.RawSample{{RawProfile: rawProfile}},
			}},
		}))
		return err
	}

	const serviceName = "test-unsymbolized-service"
	require.NoError(t, push(serviceName))

	now := time.Now()
	q := connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		Start:         now.Add(-time.Hour).UnixMilli(),
		End:           now.Add(time.Hour).UnixMilli(),
		LabelSelector: `{service_name="` + serviceName + `"}`,
	})
	var p *profilev1.Profile
	require.Eventually(t, func() bool {
		resp, err := querier.SelectMergeProfile(ctx, q) //nolint:staticcheck // Legacy querier.v1 integration coverage.
		if err != nil {
			t.Logf("Error querying profile: %v", err)
			return false
		}
		p = resp.Msg
		return len(p.Sample) > 0
	}, 5*time.Second, 100*time.Millisecond)
	requireUnsymbolizedProfile(t, p)

	// Age-based batch flushing is evaluated only when another block arrives
	// at the same compaction level, so keep pushing filler blocks, to a
	// service the query does not select, until an L0 compaction finishes.
	require.Eventually(t, func() bool {
		if err := push(serviceName + "-compaction-filler"); err != nil {
			t.Logf("Error pushing filler profile: %v", err)
			return false
		}
		n, err := c.CompactionJobsFinished(ctx)
		return err == nil && n >= 1
	}, 30*time.Second, time.Second, "no L0 compaction observed")

	// CompactionJobsFinished counts jobs on every shard and the first one may
	// be a filler's, so keep checking for a while.
	queried := false
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		resp, err := querier.SelectMergeProfile(ctx, q) //nolint:staticcheck // Legacy querier.v1 integration coverage.
		if err != nil {
			t.Logf("Error querying profile: %v", err)
			continue
		}
		requireUnsymbolizedProfile(t, resp.Msg)
		queried = true
	}
	require.True(t, queried, "profile was not queryable after compaction")
}

func unsymbolizedProfile() *profile.Profile {
	m := &profile.Mapping{
		ID:      1,
		Limit:   0x1000000,
		File:    "libfoo.so",
		BuildID: unsymbolizedBuildID,
	}
	loc1 := &profile.Location{ID: 1, Mapping: m, Address: 0x1500}
	loc2 := &profile.Location{ID: 2, Mapping: m, Address: 0x3c5a}
	return &profile.Profile{
		DurationNanos: int64(10 * time.Second),
		Period:        1000000000,
		SampleType:    []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}},
		PeriodType:    &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
		Mapping:       []*profile.Mapping{m},
		Location:      []*profile.Location{loc1, loc2},
		Sample: []*profile.Sample{
			{Location: []*profile.Location{loc1}, Value: []int64{100}},
			{Location: []*profile.Location{loc2}, Value: []int64{200}},
			{Location: []*profile.Location{loc1, loc2}, Value: []int64{3}},
		},
	}
}

// requireUnsymbolizedProfile asserts that p still holds the samples and raw
// frames of unsymbolizedProfile.
func requireUnsymbolizedProfile(t *testing.T, p *profilev1.Profile) {
	t.Helper()
	var total int64
	for _, s := range p.Sample {
		total += s.Value[0]
	}
	require.Equal(t, int64(303), total)

	mappings := make(map[uint64]*profilev1.Mapping, len(p.Mapping))
	for _, m := range p.Mapping {
		mappings[m.Id] = m
	}
	found := make(map[uint64]bool, len(p.Location))
	for _, loc := range p.Location {
		m, ok := mappings[loc.MappingId]
		if ok && p.StringTable[m.BuildId] == unsymbolizedBuildID && p.StringTable[m.Filename] == "libfoo.so" {
			found[loc.Address] = true
		}
	}
	for _, a := range []uint64{0x1500, 0x3c5a} {
		require.True(t, found[a], "missing frame %#x in libfoo.so (found %v)", a, found)
	}
}
