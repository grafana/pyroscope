package distributor

import (
	"context"
	"flag"
	"net/http"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"

	"github.com/grafana/fire/pkg/gen/ingester/v1/ingestv1connect"
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

	cfg           Config
	ingestersRing ring.ReadRing
	client        ingestv1connect.IngesterClient
}

func New(cfg Config, ingestersRing ring.ReadRing, logger log.Logger) (*Distributor, error) {
	d := &Distributor{
		cfg:           cfg,
		logger:        logger,
		ingestersRing: ingestersRing,
		// todo replication with the readRing and client pool
		client: ingestv1connect.NewIngesterClient(http.DefaultClient, "http://localhost:4100"),
	}
	d.Service = services.NewBasicService(nil, d.running, nil)
	return d, nil
}

func (d *Distributor) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (d *Distributor) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	// userID, err := tenant.TenantID(ctx)
	// if err != nil {
	// 	return nil, err
	// }
	// res := connect.NewResponse(&pushv1.PushResponse{})

	// todo replication
	return d.client.Push(ctx, req)
	// unzip and protobuf decode the request
	// reader := new(gzip.Reader)
	// for _, series := range req.Msg.Series {
	// 	for _, sample := range series.Samples {
	// 		buf := bytes.NewBuffer(sample.RawProfile)
	// 		if err := reader.Reset(buf); err != nil {
	// 			return nil, err
	// 		}
	// 		p, err := profile.Parse(reader)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		level.Debug(d.logger).Log("msg", "profile received", "profile: ", p.DefaultSampleType)
	// 	}
	// }

	// return res, nil
}
