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
	"github.com/grafana/pyroscope/v2/pkg/testhelper"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func newReadinessSegmentWriterClient(t *testing.T, ctx context.Context, logger log.Logger, r ring.ReadRing) *segmentwriterclient.Client {
	t.Helper()
	var grpcCfg grpcclient.Config
	grpcCfg.RegisterFlags(flag.NewFlagSet("", flag.PanicOnError))

	swClient, err := segmentwriterclient.NewSegmentWriterClient(
		grpcCfg, logger, nil, r, nil,
	)
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(ctx, swClient.Service()))
	t.Cleanup(func() {
		_ = services.StopAndAwaitTerminated(context.Background(), swClient.Service())
	})
	return swClient
}

func newReadinessDistributor(
	t *testing.T,
	ctx context.Context,
	logger log.Logger,
	path writepath.WritePath,
	ingesterRing ring.ReadRing,
	swClient SegmentWriterClient,
) *Distributor {
	t.Helper()
	overrides := validation.MockOverrides(func(defaults *validation.Limits, _ map[string]*validation.Limits) {
		defaults.WritePathOverrides.WritePath = path
	})

	d, err := New(
		Config{
			DistributorRing: ringConfig,
			PoolConfig:      clientpool.PoolConfig{ClientCleanupPeriod: time.Second},
		},
		ingesterRing,
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
	return d
}

func TestDistributor_CheckReady_V2SegmentWriterPath(t *testing.T) {
	logger := log.NewLogfmtLogger(os.Stdout)
	ctx := context.Background()

	tests := []struct {
		name              string
		segmentWriterRing ring.ReadRing
		wantErr           bool
	}{
		{
			name:              "ready when segment-writer ring has healthy instances",
			segmentWriterRing: testhelper.NewMockRing([]ring.InstanceDesc{{Addr: "foo"}}, 1),
		},
		{
			name:              "not ready when segment-writer ring is empty",
			segmentWriterRing: testhelper.NewMockRing(nil, 1),
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			swClient := newReadinessSegmentWriterClient(t, ctx, logger, tt.segmentWriterRing)
			d := newReadinessDistributor(
				t,
				ctx,
				logger,
				writepath.SegmentWriterPath,
				testhelper.NewMockRing([]ring.InstanceDesc{{Addr: "foo"}}, 3),
				swClient,
			)

			err := d.CheckReady(ctx)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDistributor_CheckReady_V1IngesterPath(t *testing.T) {
	logger := log.NewLogfmtLogger(os.Stdout)
	ctx := context.Background()

	tests := []struct {
		name         string
		ingesterRing ring.ReadRing
		wantErr      bool
	}{
		{
			name:         "ready when ingester ring has healthy instances",
			ingesterRing: testhelper.NewMockRing([]ring.InstanceDesc{{Addr: "foo"}}, 3),
		},
		{
			name:         "not ready when ingester ring is empty",
			ingesterRing: testhelper.NewMockRing(nil, 3),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newReadinessDistributor(
				t,
				ctx,
				logger,
				writepath.IngesterPath,
				tt.ingesterRing,
				nil,
			)

			err := d.CheckReady(ctx)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
