package distributor

import (
	"context"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/clientpool"
	"github.com/grafana/pyroscope/v2/pkg/distributor/writepath"
	segmentwriterclient "github.com/grafana/pyroscope/v2/pkg/segmentwriter/client"
	"github.com/grafana/pyroscope/v2/pkg/segmentwriter/client/distributor/placement"
	"github.com/grafana/pyroscope/v2/pkg/testhelper"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

type readinessPlacement struct{}

func (readinessPlacement) Policy(placement.Key) placement.Policy {
	return placement.Policy{
		TenantShards:  0,
		DatasetShards: 1,
		PickShard:     func(int) int { return 0 },
	}
}

// TestDistributor_CheckReady_EmptyRing tests that the distributor reports not ready
// while the segment-writer ring is empty (e.g. during a rollout), the distributor
// must not report itself as ready, because any incoming write would fail
// with a 503 ("empty ring"). The distributor and segment-writer client are
// both fully Running here; only the ring is unpopulated.
func TestDistributor_CheckReady_EmptyRing(t *testing.T) {
	logger := log.NewLogfmtLogger(os.Stdout)
	ctx := context.Background()

	var grpcCfg grpcclient.Config
	grpcCfg.RegisterFlags(flag.NewFlagSet("", flag.PanicOnError))

	emptyRing := testhelper.NewMockRing(nil, 1)
	swClient, err := segmentwriterclient.NewSegmentWriterClient(
		grpcCfg, logger, nil, emptyRing, readinessPlacement{},
	)
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(ctx, swClient.Service()))
	t.Cleanup(func() {
		_ = services.StopAndAwaitTerminated(context.Background(), swClient.Service())
	})

	// Simulate a V2-only deployment: deployment-default WritePath routes
	// to the segment-writer, so the readiness check should fail when its
	// ring is empty regardless of ingester ring state.
	overrides := validation.MockOverrides(func(defaults *validation.Limits, _ map[string]*validation.Limits) {
		defaults.WritePathOverrides.WritePath = writepath.SegmentWriterPath
	})

	d, err := New(
		Config{
			DistributorRing: ringConfig,
			PoolConfig:      clientpool.PoolConfig{ClientCleanupPeriod: time.Second},
		},
		testhelper.NewMockRing([]ring.InstanceDesc{{Addr: "foo"}}, 3),
		&poolFactory{f: func(string) (client.PoolClient, error) {
			return newFakeIngester(t, false), nil
		}},
		overrides,
		nil,
		logger,
		swClient,
	)
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(ctx, d))
	t.Cleanup(func() {
		_ = services.StopAndAwaitTerminated(context.Background(), d)
	})

	require.Equal(t, services.Running, d.State())
	require.Equal(t, services.Running, swClient.Service().State())
	require.Zero(t, emptyRing.InstancesCount(), "ring should be empty for this scenario")

	require.Error(t, d.CheckReady(ctx),
		"distributor must not report ready while segment-writer ring is empty")
}
