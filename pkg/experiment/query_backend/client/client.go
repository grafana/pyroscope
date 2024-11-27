package querybackendclient

import (
	"context"

	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"google.golang.org/grpc"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

type Client struct {
	service    services.Service
	grpcClient queryv1.QueryBackendServiceClient
}

func New(address string, grpcClientConfig grpcclient.Config) (*Client, error) {
	conn, err := dial(address, grpcClientConfig)
	if err != nil {
		return nil, err
	}
	var c Client
	c.grpcClient = queryv1.NewQueryBackendServiceClient(conn)
	c.service = services.NewIdleService(c.starting, c.stopping)
	return &c, nil
}

func dial(address string, grpcClientConfig grpcclient.Config) (*grpc.ClientConn, error) {
	options, err := grpcClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}
	// TODO: https://github.com/grpc/grpc-proto/blob/master/grpc/service_config/service_config.proto
	options = append(options,
		grpc.WithDefaultServiceConfig(grpcServiceConfig),
		grpc.WithMaxCallAttempts(500),
	)
	return grpc.NewClient(address, options...)
}

func (b *Client) Service() services.Service      { return b.service }
func (b *Client) starting(context.Context) error { return nil }
func (b *Client) stopping(error) error           { return nil }

func (b *Client) Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	return b.grpcClient.Invoke(ctx, req)
}

const grpcServiceConfig = `{
    "loadBalancingPolicy":"round_robin",
    "methodConfig": [{
        "name": [{"service": ""}],
        "waitForReady": true,
        "retryPolicy": {
            "MaxAttempts": 500,
            "InitialBackoff": "1s",
            "MaxBackoff": "2s",
            "BackoffMultiplier": 1.1,
            "RetryableStatusCodes": [
              "UNAVAILABLE",
              "RESOURCE_EXHAUSTED"
            ]
        },
        "timeout": "20s"
    }]
}`
