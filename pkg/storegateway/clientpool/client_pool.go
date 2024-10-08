package clientpool

import (
	"context"
	"io"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/storegateway/v1/storegatewayv1connect"
	"github.com/grafana/pyroscope/pkg/util"
)

type BidiClientMergeProfilesStacktraces interface {
	Send(*ingestv1.MergeProfilesStacktracesRequest) error
	Receive() (*ingestv1.MergeProfilesStacktracesResponse, error)
	CloseRequest() error
	CloseResponse() error
}

type BidiClientMergeProfilesLabels interface {
	Send(*ingestv1.MergeProfilesLabelsRequest) error
	Receive() (*ingestv1.MergeProfilesLabelsResponse, error)
	CloseRequest() error
	CloseResponse() error
}

type BidiClientMergeProfilesPprof interface {
	Send(*ingestv1.MergeProfilesPprofRequest) error
	Receive() (*ingestv1.MergeProfilesPprofResponse, error)
	CloseRequest() error
	CloseResponse() error
}

func NewPool(ring ring.ReadRing, factory ring_client.PoolFactory, clientsMetric prometheus.Gauge, logger log.Logger, options ...connect.ClientOption) *ring_client.Pool {
	if factory == nil {
		factory = newPoolFactory()
	}
	poolCfg := ring_client.PoolConfig{
		CheckInterval:      10 * time.Second,
		HealthCheckEnabled: true,
		HealthCheckTimeout: 10 * time.Second,
	}

	return ring_client.NewPool("store-gateway", poolCfg, ring_client.NewRingServiceDiscovery(ring), factory, clientsMetric, logger)
}

type poolFactory struct {
	options []connect.ClientOption
}

func newPoolFactory(options ...connect.ClientOption) ring_client.PoolFactory {
	return &poolFactory{options: options}
}

func (f *poolFactory) FromInstance(inst ring.InstanceDesc) (ring_client.PoolClient, error) {
	conn, err := grpc.Dial(inst.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	httpClient := util.InstrumentedDefaultHTTPClient(util.WithTracingTransport(), util.WithBaggageTransport())
	return &storeGatewayPoolClient{
		StoreGatewayServiceClient: storegatewayv1connect.NewStoreGatewayServiceClient(httpClient, "http://"+inst.Addr, f.options...),
		HealthClient:              grpc_health_v1.NewHealthClient(conn),
		Closer:                    conn,
	}, nil
}

type storeGatewayPoolClient struct {
	storegatewayv1connect.StoreGatewayServiceClient
	grpc_health_v1.HealthClient
	io.Closer
}

func (c *storeGatewayPoolClient) MergeProfilesStacktraces(ctx context.Context) BidiClientMergeProfilesStacktraces {
	return c.StoreGatewayServiceClient.MergeProfilesStacktraces(ctx)
}

func (c *storeGatewayPoolClient) MergeProfilesLabels(ctx context.Context) BidiClientMergeProfilesLabels {
	return c.StoreGatewayServiceClient.MergeProfilesLabels(ctx)
}

func (c *storeGatewayPoolClient) MergeProfilesPprof(ctx context.Context) BidiClientMergeProfilesPprof {
	return c.StoreGatewayServiceClient.MergeProfilesPprof(ctx)
}
