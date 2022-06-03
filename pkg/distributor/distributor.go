package distributor

import (
	"context"
	"flag"

	"github.com/grafana/dskit/services"

	"github.com/grafana/fire/pkg/gen/proto/go/push"
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

	cfg Config
}

func New(cfg Config) (*Distributor, error) {
	d := &Distributor{
		cfg: cfg,
	}
	d.Service = services.NewBasicService(nil, d.running, nil)
	return d, nil
}

func (d *Distributor) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (d *Distributor) Push(ctx context.Context, req *push.PushRequest) error {
	return nil
}
