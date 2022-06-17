package clientpool

import (
	"flag"
	"io"
	"net/http"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grafana/fire/pkg/gen/ingester/v1/ingestv1connect"
)

// PoolConfig is config for creating a Pool.
type PoolConfig struct {
	ClientCleanupPeriod  time.Duration `yaml:"client_cleanup_period"`
	HealthCheckIngesters bool          `yaml:"health_check_ingesters"`
	RemoteTimeout        time.Duration `yaml:"remote_timeout"`
}

// RegisterFlags adds the flags required to config this to the given FlagSet.
func (cfg *PoolConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&cfg.ClientCleanupPeriod, prefix+".client-cleanup-period", 15*time.Second, "How frequently to clean up clients for ingesters that have gone away.")
	f.BoolVar(&cfg.HealthCheckIngesters, prefix+".health-check-ingesters", true, "Run a health check on each ingester client during periodic cleanup.")
	f.DurationVar(&cfg.RemoteTimeout, prefix+".health-check-timeout", 5*time.Second, "Timeout for ingester client healthcheck RPCs.")
}

func NewPool(cfg PoolConfig, ring ring.ReadRing, factory ring_client.PoolFactory, clientsMetric prometheus.Gauge, logger log.Logger) *ring_client.Pool {
	if factory == nil {
		factory = PoolFactory
	}
	poolCfg := ring_client.PoolConfig{
		CheckInterval:      cfg.ClientCleanupPeriod,
		HealthCheckEnabled: cfg.HealthCheckIngesters,
		HealthCheckTimeout: cfg.RemoteTimeout,
	}

	return ring_client.NewPool("ingester", poolCfg, ring_client.NewRingServiceDiscovery(ring), factory, clientsMetric, logger)
}

func PoolFactory(addr string) (ring_client.PoolClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return &ingesterPoolClient{
		IngesterClient: ingestv1connect.NewIngesterClient(http.DefaultClient, "http://"+addr),
		HealthClient:   grpc_health_v1.NewHealthClient(conn),
		Closer:         conn,
	}, nil
}

type ingesterPoolClient struct {
	ingestv1connect.IngesterClient
	grpc_health_v1.HealthClient
	io.Closer
}
