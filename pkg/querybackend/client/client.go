package querybackendclient

import (
	"context"

	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/opentracing/opentracing-go"
	"google.golang.org/grpc"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
)

type Client struct {
	service    services.Service
	grpcClient querybackendv1.QueryBackendServiceClient
}

func New(address string, grpcClientConfig grpcclient.Config) (*Client, error) {
	conn, err := dial(address, grpcClientConfig)
	if err != nil {
		return nil, err
	}
	var c Client
	c.grpcClient = querybackendv1.NewQueryBackendServiceClient(conn)
	c.service = services.NewIdleService(c.starting, c.stopping)
	return &c, nil
}

func dial(address string, grpcClientConfig grpcclient.Config) (*grpc.ClientConn, error) {
	if err := grpcClientConfig.Validate(); err != nil {
		return nil, err
	}
	options, err := grpcClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}
	// TODO: https://github.com/grpc/grpc-proto/blob/master/grpc/service_config/service_config.proto
	options = append(options,
		grpc.WithUnaryInterceptor(otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer())),
		grpc.WithDefaultServiceConfig(grpcServiceConfig),
	)
	return grpc.Dial(address, options...)
}

func (b *Client) Service() services.Service      { return b.service }
func (b *Client) starting(context.Context) error { return nil }
func (b *Client) stopping(error) error           { return nil }

func (b *Client) Invoke(ctx context.Context, req *querybackendv1.InvokeRequest) (*querybackendv1.InvokeResponse, error) {
	return b.grpcClient.Invoke(ctx, req)
}

const grpcServiceConfig = `{
    "loadBalancingPolicy":"round_robin",
    "methodConfig": [{
        "name": [{"service": ""}],
        "waitForReady": true,
        "retryPolicy": {
            "MaxAttempts": 4,
            "InitialBackoff": ".01s",
            "MaxBackoff": ".01s",
            "BackoffMultiplier": 1.0,
            "RetryableStatusCodes": [ "UNAVAILABLE" ]
        }
    }]
}`
