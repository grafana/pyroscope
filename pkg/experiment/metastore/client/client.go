package metastoreclient

import (
	"context"
	"github.com/go-kit/log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/raft"
	"sync"

	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/opentracing-contrib/go-grpc"
	"github.com/opentracing/opentracing-go"
	"google.golang.org/grpc"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

var _ metastorev1.MetastoreServiceClient = (*Client)(nil)
var _ compactorv1.CompactionPlannerClient = (*Client)(nil)

type Client struct {
	service services.Service

	discovery discovery.Discovery

	mu sync.Mutex

	leader           raft.ServerID
	servers          map[raft.ServerID]*client
	stopped          bool
	logger           log.Logger
	grpcClientConfig grpcclient.Config
}

type client struct {
	metastorev1.MetastoreServiceClient
	compactorv1.CompactionPlannerClient
	conn *grpc.ClientConn
	srv  discovery.Server
}

func New(address string, logger log.Logger, grpcClientConfig grpcclient.Config) (*Client, error) {
	var (
		c   = new(Client)
		err error
	)

	c.service = services.NewIdleService(c.starting, c.stopping)
	c.logger = logger
	c.grpcClientConfig = grpcClientConfig
	c.servers = make(map[raft.ServerID]*client)

	c.discovery, err = discovery.NewKubeResolverDiscovery(logger, address, nil, discovery.UpdateFunc(func(servers []discovery.Server) {
		c.updateServers(servers)
	}))
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) Service() services.Service      { return c.service }
func (c *Client) starting(context.Context) error { return nil }
func (c *Client) stopping(error) error {
	c.discovery.Close()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	var multiErr error
	for _, srv := range c.servers {
		err := srv.conn.Close()
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}
	return multiErr
}

func (c *Client) updateServers(servers []discovery.Server) {
	byID := make(map[raft.ServerID][]discovery.Server, len(servers))
	for _, srv := range servers {
		byID[srv.Raft.ID] = append(byID[srv.Raft.ID], srv)
	}
	for k, ss := range byID {
		if len(ss) > 1 {
			c.logger.Log("msg", "multiple servers with the same ID", "id", k, "servers", ss)
			delete(byID, k)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	newServers := make(map[raft.ServerID]*client, len(byID))

	for k, s := range byID {
		prev, ok := c.servers[k]
		if ok {
			if prev.srv == s[0] {
				newServers[k] = prev
				c.logger.Log("msg", "server already exists", "id", k, "server", s[0])
				continue
			}
			_ = prev.conn.Close()
			c.servers[k] = nil
		}
		cl, err := newClient(s[0], c.grpcClientConfig, c.logger)
		if err != nil {
			c.logger.Log("msg", "failed to crate client", "err", err)
			continue
		}
		newServers[k] = cl
	}
	c.servers = newServers
}

func newClient(s discovery.Server, config grpcclient.Config, logger log.Logger) (*client, error) {
	address := s.Raft.Address
	if s.IP != "" {
		address = raft.ServerAddress(s.IP)
	}
	conn, err := dial(string(address), config, logger)
	if err != nil {
		return nil, err
	}
	return &client{
		MetastoreServiceClient:  metastorev1.NewMetastoreServiceClient(conn),
		CompactionPlannerClient: compactorv1.NewCompactionPlannerClient(conn),
		conn:                    conn,
		srv:                     s,
	}, nil
}

func dial(address string, grpcClientConfig grpcclient.Config, _ log.Logger) (*grpc.ClientConn, error) {
	options, err := grpcClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}
	// TODO: https://github.com/grpc/grpc-proto/blob/master/grpc/service_config/service_config.proto
	options = append(options,
		//grpc.WithDefaultServiceConfig(grpcServiceConfig),
		grpc.WithUnaryInterceptor(otgrpc.OpenTracingClientInterceptor(opentracing.GlobalTracer())),
	)
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
