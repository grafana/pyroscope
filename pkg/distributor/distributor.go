package distributor

import (
	"context"
	"flag"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"

	pushv1 "github.com/grafana/fire/pkg/gen/push/v1"
)

// Config for a Distributor.
type Config struct {
	// Distributors ring
	DistributorRing RingConfig `yaml:"ring,omitempty"`
}

// RegisterFlags registers distributor-related flags.
func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	cfg.DistributorRing.RegisterFlags(fs)
}

// Distributor coordinates replicates and distribution of log streams.
type Distributor struct {
	services.Service
	logger log.Logger

	cfg Config
}

func New(cfg Config, logger log.Logger) (*Distributor, error) {
	d := &Distributor{
		cfg:    cfg,
		logger: logger,
	}
	d.Service = services.NewBasicService(nil, d.running, nil)
	return d, nil
}

func (d *Distributor) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (d *Distributor) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	level.Debug(d.logger).Log("msg", "message received", "request headers: ", req.Header())
	res := connect.NewResponse(&pushv1.PushResponse{})

	// unzip and protobuf decode the request

	return res, nil
}
