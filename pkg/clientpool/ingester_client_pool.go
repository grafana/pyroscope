package clientpool

import (
	"context"
	"crypto/tls"
	"flag"
	"io"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1/ingesterv1connect"
	"github.com/grafana/pyroscope/v2/pkg/util"
)

// PoolConfig is config for creating a Pool.
type PoolConfig struct {
	ClientCleanupPeriod  time.Duration `yaml:"client_cleanup_period"`
	HealthCheckIngesters bool          `yaml:"health_check_ingesters"`
	RemoteTimeout        time.Duration `yaml:"remote_timeout"`
	TLSEnabled           bool          `yaml:"tls_enabled"`
}

// RegisterFlagsWithPrefix adds the flags required to config this to the given FlagSet.
func (cfg *PoolConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	f.DurationVar(&cfg.ClientCleanupPeriod, prefix+".client-cleanup-period", 15*time.Second, "How frequently to clean up clients for ingesters that have gone away.")
	f.BoolVar(&cfg.HealthCheckIngesters, prefix+".health-check-ingesters", true, "Run a health check on each ingester client during periodic cleanup.")
	f.DurationVar(&cfg.RemoteTimeout, prefix+".health-check-timeout", 5*time.Second, "Timeout for ingester client healthcheck RPCs.")
	f.BoolVar(&cfg.TLSEnabled, prefix+".tls-enabled", false, "Enable TLS for ingester client connections.")
}

func NewIngesterPool(cfg PoolConfig, ring ring.ReadRing, factory ring_client.PoolFactory, clientsMetric prometheus.Gauge, logger log.Logger, options ...connect.ClientOption) *ring_client.Pool {
	if factory == nil {
		factory = newIngesterPoolFactory(cfg.TLSEnabled, options...)
	}
	poolCfg := ring_client.PoolConfig{
		CheckInterval:      cfg.ClientCleanupPeriod,
		HealthCheckEnabled: cfg.HealthCheckIngesters,
		HealthCheckTimeout: cfg.RemoteTimeout,
	}

	return ring_client.NewPool("ingester", poolCfg, ring_client.NewRingServiceDiscovery(ring), factory, clientsMetric, logger)
}

type ingesterPoolFactory struct {
	tlsEnabled bool
	options    []connect.ClientOption
}

func newIngesterPoolFactory(tlsEnabled bool, options ...connect.ClientOption) ring_client.PoolFactory {
	return &ingesterPoolFactory{
		tlsEnabled: tlsEnabled,
		options:    options,
	}
}

func (f *ingesterPoolFactory) FromInstance(inst ring.InstanceDesc) (ring_client.PoolClient, error) {
	var dialOpts []grpc.DialOption
	var scheme string

	if f.tlsEnabled {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
		scheme = "https://"
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		scheme = "http://"
	}

	conn, err := grpc.Dial(inst.Addr, dialOpts...)
	if err != nil {
		return nil, err
	}

	httpClient := util.InstrumentedDefaultHTTPClient(util.WithTracingTransport(), util.WithBaggageTransport())
	return &ingesterPoolClient{
		IngesterServiceClient: ingesterv1connect.NewIngesterServiceClient(httpClient, scheme+inst.Addr, f.options...),
		HealthClient:          grpc_health_v1.NewHealthClient(conn),
		Closer:                conn,
	}, nil
}

type ingesterPoolClient struct {
	ingesterv1connect.IngesterServiceClient
	grpc_health_v1.HealthClient
	io.Closer
}

func (c *ingesterPoolClient) MergeProfilesStacktraces(ctx context.Context) BidiClientMergeProfilesStacktraces {
	return c.IngesterServiceClient.MergeProfilesStacktraces(ctx)
}

func (c *ingesterPoolClient) MergeProfilesLabels(ctx context.Context) BidiClientMergeProfilesLabels {
	return c.IngesterServiceClient.MergeProfilesLabels(ctx)
}

func (c *ingesterPoolClient) MergeProfilesPprof(ctx context.Context) BidiClientMergeProfilesPprof {
	return c.IngesterServiceClient.MergeProfilesPprof(ctx)
}

func (c *ingesterPoolClient) MergeSpanProfile(ctx context.Context) BidiClientMergeSpanProfile {
	return c.IngesterServiceClient.MergeSpanProfile(ctx)
}
