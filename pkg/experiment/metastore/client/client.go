package metastoreclient

import (
	"context"
	"fmt"
	"github.com/go-kit/log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/discovery"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/raft"
	"io"
	"sync"

	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	otgrpc "github.com/opentracing-contrib/go-grpc"
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
	conn io.Closer
	srv  discovery.Server
}

// todo
type instance interface {
	metastorev1.MetastoreServiceClient
	compactorv1.CompactionPlannerClient
}

func New(logger log.Logger, grpcClientConfig grpcclient.Config, d discovery.Discovery) *Client {
	var (
		c = new(Client)
	)
	logger = log.With(logger, "component", "metastore-client")

	c.service = services.NewIdleService(c.starting, c.stopping)
	c.logger = logger
	c.grpcClientConfig = grpcClientConfig
	c.servers = make(map[raft.ServerID]*client)
	c.discovery = d
	c.discovery.Subscribe(discovery.UpdateFunc(func(servers []discovery.Server) {
		c.updateServers(servers)
	}))
	return c
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
		c.logger.Log("msg", "connection closed", "resolved_address", srv.srv.ResolvedAddress, "raft_address", srv.srv.Raft.Address)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}
	c.servers = nil
	return multiErr
}

func (c *Client) updateServers(servers []discovery.Server) {
	c.logger.Log("msg", "updating servers", "servers", fmt.Sprintf("%+v", servers))
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
	clientSet := make(map[*client]struct{})
	for k, s := range byID {
		prev, ok := c.servers[k]
		if ok {
			if prev.srv == s[0] {
				newServers[k] = prev
				clientSet[prev] = struct{}{}
				c.logger.Log("msg", "server already exists", "id", k, "server", s[0])
				continue
			}
		}
		cl, err := newClient(s[0], c.grpcClientConfig, c.logger)
		if err != nil {
			c.logger.Log("msg", "failed to crate client", "err", err)
			continue
		}
		c.logger.Log("msg", "new client created", "resolved_address", cl.srv.ResolvedAddress, "raft_address", cl.srv.Raft.Address)
		newServers[k] = cl
		clientSet[cl] = struct{}{}
	}
	for _, oldClient := range c.servers {
		if _, ok := clientSet[oldClient]; !ok {
			err := oldClient.conn.Close()
			if err != nil {
				c.logger.Log("msg", "failed to close connection", "err", err)
			} else {
				c.logger.Log("msg", "connection closed", "resolved_address", oldClient.srv.ResolvedAddress, "raft_address", oldClient.srv.Raft.Address)
			}
		}
	}
	c.servers = newServers
}

func newClient(s discovery.Server, config grpcclient.Config, logger log.Logger) (*client, error) {
	address := s.Raft.Address
	if s.ResolvedAddress != "" {
		address = raft.ServerAddress(s.ResolvedAddress)
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
