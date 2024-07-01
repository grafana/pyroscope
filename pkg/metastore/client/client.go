package metastoreclient

import (
	"flag"
	"fmt"

	"github.com/grafana/dskit/grpcclient"
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

func Dial(cfg Config) (*grpc.ClientConn, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	options, err := cfg.GRPCClientConfig.DialOption(nil, nil)
	if err != nil {
		return nil, err
	}
	const grpcServiceConfig = `{"loadBalancingPolicy":"round_robin"}`
	options = append(options, grpc.WithDefaultServiceConfig(grpcServiceConfig))
	return grpc.Dial(cfg.MetastoreAddress, options...)
}
