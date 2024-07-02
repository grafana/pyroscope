package metastoreclient

import (
	"context"
	"flag"
	"fmt"

	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"google.golang.org/grpc"
)

type Config struct {
	MetastoreAddress string            `yaml:"address"`
	GRPCClientConfig grpcclient.Config `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate between the query-frontends and the query-schedulers."`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.MetastoreAddress, "metastore.address", "localhost:9095", "")
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix("metastore.grpc-client-config", f)
}

func (cfg *Config) Validate() error {
	if cfg.MetastoreAddress == "" {
		return fmt.Errorf("metastore.address is required")
	}
	return cfg.GRPCClientConfig.Validate()
}

type Client struct {
	service services.Service
	conn    *grpc.ClientConn
	config  Config
}

func New(config Config) (*Client, error) {
	c := Client{config: config}
	c.service = services.NewIdleService(c.starting, c.stopping)
	return &c, nil
}

func (c *Client) starting(context.Context) (err error) {
	c.conn, err = dial(c.config)
	return err
}

func (c *Client) stopping(error) error { return c.conn.Close() }

func (c *Client) Service() services.Service { return c.service }

func dial(cfg Config) (*grpc.ClientConn, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	options, err := cfg.GRPCClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}
	const grpcServiceConfig = `{{"healthCheckConfig": {"serviceName": "pyroscope.metastore.raft_leader"}, "loadBalancingPolicy":"round_robin"}`
	options = append(options, grpc.WithDefaultServiceConfig(grpcServiceConfig))
	return grpc.Dial(cfg.MetastoreAddress, options...)
}
