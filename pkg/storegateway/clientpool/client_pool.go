package clientpool

import (
	"context"
	"io"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	"github.com/grafana/phlare/api/gen/proto/go/storegateway/v1/storegatewayv1connect"
	"github.com/grafana/phlare/pkg/util"
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
		factory = PoolFactoryFn(options...)
	}
	poolCfg := ring_client.PoolConfig{
		CheckInterval:      10 * time.Second,
		HealthCheckEnabled: true,
		HealthCheckTimeout: 10 * time.Second,
	}

	return ring_client.NewPool("store-gateway", poolCfg, ring_client.NewRingServiceDiscovery(ring), factory, clientsMetric, logger)
}

func PoolFactoryFn(options ...connect.ClientOption) ring_client.PoolFactory {
	return func(addr string) (ring_client.PoolClient, error) {
		conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		return &storeGatewayPoolClient{
			StoreGatewayServiceClient: storegatewayv1connect.NewStoreGatewayServiceClient(util.InstrumentedHTTPClient(), "http://"+addr, options...),
			HealthClient:              grpc_health_v1.NewHealthClient(conn),
			Closer:                    conn,
		}, nil
	}
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
