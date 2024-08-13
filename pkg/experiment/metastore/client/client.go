package metastoreclient

import (
	"context"
	"fmt"
	"os"

	"github.com/go-kit/log"

	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/opentracing-contrib/go-grpc"
	"github.com/opentracing/opentracing-go"
	"google.golang.org/grpc"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type Client struct {
	metastorev1.MetastoreServiceClient
	compactorv1.CompactionPlannerClient
	service services.Service
	conn    *grpc.ClientConn
}

func New(address string, grpcClientConfig grpcclient.Config, logger log.Logger) (c *Client, err error) {
	c.conn, err = dial(address, grpcClientConfig, logger)
	if err != nil {
		return nil, err
	}
	c.MetastoreServiceClient = metastorev1.NewMetastoreServiceClient(c.conn)
	c.CompactionPlannerClient = compactorv1.NewCompactionPlannerClient(c.conn)
	c.service = services.NewIdleService(c.starting, c.stopping)
	return c, nil
}

func (c *Client) Service() services.Service      { return c.service }
func (c *Client) starting(context.Context) error { return nil }
func (c *Client) stopping(error) error           { return c.conn.Close() }

func dial(address string, grpcClientConfig grpcclient.Config, logger log.Logger) (*grpc.ClientConn, error) {
	if err := grpcClientConfig.Validate(); err != nil {
		return nil, err
	}
	options, err := grpcClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}
	// TODO: https://github.com/grpc/grpc-proto/blob/master/grpc/service_config/service_config.proto
	options = append(options,
		grpc.WithDefaultServiceConfig(grpcServiceConfig),
		grpc.WithUnaryInterceptor(otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer())),
	)
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		builder, err := NewGrpcResolverBuilder(logger, address)
		if err != nil {
			return nil, fmt.Errorf("failed to create grpc resolver builder: %w", err)
		}
		options = append(options, grpc.WithResolvers(builder))
		return grpc.Dial(builder.resolverAddrStub(), options...)
	}
	return grpc.Dial(address, options...)
}

const grpcServiceConfig = `{
	"healthCheckConfig": {
		"serviceName": "metastore.v1.MetastoreService.RaftLeader"
	},
    "loadBalancingPolicy":"round_robin",
    "methodConfig": [{
        "name": [{"service": "metastore.v1.MetastoreService"}],
        "waitForReady": true,
        "retryPolicy": {
            "MaxAttempts": 16,
            "InitialBackoff": ".01s",
            "MaxBackoff": ".01s",
            "BackoffMultiplier": 1.0,
            "RetryableStatusCodes": [ "UNAVAILABLE" ]
        }
    }]
}`
