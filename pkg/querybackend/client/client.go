package querybackendclient

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"google.golang.org/grpc"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

type Client struct {
	service    services.Service
	grpcClient queryv1.QueryBackendServiceClient
}

func New(address string, grpcClientConfig grpcclient.Config, timeout time.Duration, dialOpts ...grpc.DialOption) (*Client, error) {
	conn, err := dial(address, grpcClientConfig, timeout, dialOpts...)
	if err != nil {
		return nil, err
	}
	var c Client
	c.grpcClient = queryv1.NewQueryBackendServiceClient(conn)
	c.service = services.NewIdleService(c.starting, c.stopping)
	return &c, nil
}

func dial(address string, grpcClientConfig grpcclient.Config, timeout time.Duration, dialOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	options, err := grpcClientConfig.DialOption(nil, nil, nil)
	if err != nil {
		return nil, err
	}
	// TODO: https://github.com/grpc/grpc-proto/blob/master/grpc/service_config/service_config.proto
	serviceConfig := fmt.Sprintf(grpcServiceConfigTemplate, timeout.Seconds())
	options = append(options,
		grpc.WithDefaultServiceConfig(serviceConfig),
		grpc.WithMaxCallAttempts(500),
	)
	options = append(options, dialOpts...)
	return grpc.NewClient(address, options...)
}

func (b *Client) Service() services.Service      { return b.service }
func (b *Client) starting(context.Context) error { return nil }
func (b *Client) stopping(error) error           { return nil }

func (b *Client) Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	return b.grpcClient.Invoke(ctx, req)
}

// InvokeStream opens a gRPC server-streaming call to the query backend.
// It satisfies the QueryBackend interface used by QueryFrontend.
func (b *Client) InvokeStream(ctx context.Context, req *queryv1.InvokeRequest) (queryv1.QueryBackendService_InvokeStreamClient, error) {
	return b.grpcClient.InvokeStream(ctx, req)
}

// InvokeStreamEvents implements the streamEventSender interface used by QueryBackend
// when this client is used as the backendClient in a multi-tier deployment.
func (b *Client) InvokeStreamEvents(ctx context.Context, req *queryv1.InvokeRequest, send func(*queryv1.InvokeStreamEvent) error) error {
	stream, err := b.grpcClient.InvokeStream(ctx, req)
	if err != nil {
		return err
	}
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := send(event); err != nil {
			return err
		}
	}
}

const grpcServiceConfigTemplate = `{
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
        "timeout": "%.0fs"
    }]
}`
