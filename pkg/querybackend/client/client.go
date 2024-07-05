package querybackendclient

import (
	"context"

	"github.com/grafana/dskit/services"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
)

type Config struct {
	Address string
}

type Client struct {
	service    services.Service
	grpcClient querybackendv1.QueryBackendServiceClient
}

func New(Config) (*Client, error) {
	var c Client
	// TODO:
	//  A standard grpc client.
	//  No health checks.
	//  Client-side load balancing.
	//  Retries.
	c.service = services.NewIdleService(c.starting, c.stopping)
	return &c, nil
}

func (b *Client) Service() services.Service      { return b.service }
func (b *Client) starting(context.Context) error { return nil }
func (b *Client) stopping(error) error           { return nil }

func (b *Client) Invoke(ctx context.Context, req *querybackendv1.InvokeRequest) (*querybackendv1.InvokeResponse, error) {
	return b.grpcClient.Invoke(ctx, req)
}
