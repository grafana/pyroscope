package metastoreclient

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/raft"
	"google.golang.org/grpc"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/discovery"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb"
)

// TODO(kolesnikovae): Implement raft leader routing as a grpc load balancer or interceptor.

type Client struct {
	service   services.Service
	discovery discovery.Discovery

	mu               sync.Mutex
	leader           raft.ServerID
	servers          map[raft.ServerID]*client
	stopped          bool
	logger           log.Logger
	grpcClientConfig grpcclient.Config
	dialOpts         []grpc.DialOption
}

type client struct {
	metastorev1.IndexServiceClient
	metastorev1.CompactionServiceClient
	metastorev1.MetadataQueryServiceClient
	metastorev1.TenantServiceClient
	raftnodepb.RaftNodeServiceClient

	conn io.Closer
	srv  discovery.Server
}

// todo
type instance interface {
	metastorev1.IndexServiceClient
	metastorev1.MetadataQueryServiceClient
	metastorev1.TenantServiceClient
	metastorev1.CompactionServiceClient
	raftnodepb.RaftNodeServiceClient
}

func New(logger log.Logger, grpcClientConfig grpcclient.Config, d discovery.Discovery, dialOpts ...grpc.DialOption) *Client {
	var c Client
	logger = log.With(logger, "component", "metastore-client")
	c.service = services.NewIdleService(c.starting, c.stopping)
	c.logger = logger
	c.grpcClientConfig = grpcClientConfig
	c.servers = make(map[raft.ServerID]*client)
	c.discovery = d
	c.dialOpts = dialOpts

	c.discovery.Subscribe(discovery.UpdateFunc(func(servers []discovery.Server) {
		c.updateServers(servers)
	}))
	return &c
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
		level.Debug(c.logger).Log("msg", "connection closed", "resolved_address", srv.srv.ResolvedAddress, "raft_address", srv.srv.Raft.Address)
		if err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}
	c.servers = nil
	return multiErr
}

func (c *Client) updateServers(servers []discovery.Server) {
	level.Info(c.logger).Log("msg", "updating servers", "servers", fmt.Sprintf("%+v", servers))
	byID := make(map[raft.ServerID][]discovery.Server, len(servers))
	for _, srv := range servers {
		id := stripPort(string(srv.Raft.ID))
		byID[id] = append(byID[id], srv)
	}
	for k, ss := range byID {
		if len(ss) > 1 {
			level.Warn(c.logger).Log("msg", "multiple servers with the same ID", "id", k, "servers", ss)
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
				level.Debug(c.logger).Log("msg", "server already exists", "id", k, "server", fmt.Sprintf("%+v", s[0]))
				continue
			}
		}
		cl, err := newClient(s[0], c.grpcClientConfig, c.dialOpts...)
		if err != nil {
			level.Error(c.logger).Log("msg", "failed to create client", "err", err)
			continue
		}
		level.Info(c.logger).Log("msg", "new client created", "resolved_address", cl.srv.ResolvedAddress, "raft_address", cl.srv.Raft.Address)
		newServers[k] = cl
		clientSet[cl] = struct{}{}
	}
	for _, oldClient := range c.servers {
		if _, ok := clientSet[oldClient]; !ok {
			err := oldClient.conn.Close()
			if err != nil {
				level.Warn(c.logger).Log("msg", "failed to close connection", "err", err)
			} else {
				level.Debug(c.logger).Log("msg", "connection closed", "resolved_address", oldClient.srv.ResolvedAddress, "raft_address", oldClient.srv.Raft.Address)
			}
		}
	}
	c.servers = newServers
}

func newClient(s discovery.Server, config grpcclient.Config, dialOpts ...grpc.DialOption) (*client, error) {
	address := s.Raft.Address
	if s.ResolvedAddress != "" {
		address = raft.ServerAddress(s.ResolvedAddress)
	}
	conn, err := dial(string(address), config, dialOpts...)
	if err != nil {
		return nil, err
	}
	return &client{
		IndexServiceClient:         metastorev1.NewIndexServiceClient(conn),
		CompactionServiceClient:    metastorev1.NewCompactionServiceClient(conn),
		MetadataQueryServiceClient: metastorev1.NewMetadataQueryServiceClient(conn),
		TenantServiceClient:        metastorev1.NewTenantServiceClient(conn),
		RaftNodeServiceClient:      raftnodepb.NewRaftNodeServiceClient(conn),
		conn:                       conn,
		srv:                        s,
	}, nil
}

func dial(address string, grpcClientConfig grpcclient.Config, dialOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	options, err := grpcClientConfig.DialOption(nil, nil, nil)
	if err != nil {
		return nil, err
	}
	// TODO: https://github.com/grpc/grpc-proto/blob/master/grpc/service_config/service_config.proto
	options = append(options, grpc.WithDefaultServiceConfig(grpcServiceConfig))
	options = append(options, dialOpts...)
	return grpc.Dial(address, options...)
}

const grpcServiceConfig = `{
	"healthCheckConfig": {
		"serviceName": "pyroscope.metastore"
	}
}`
