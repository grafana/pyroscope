package querier

import (
	"context"
	"flag"
	"sort"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/ring"
	ring_client "github.com/grafana/dskit/ring/client"
	"github.com/grafana/dskit/services"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
	"github.com/grafana/fire/pkg/ingester/clientpool"
)

// todo: move to non global metrics.
var clients = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: "fire",
	Name:      "querier_ingester_clients",
	Help:      "The current number of ingester clients.",
})

type Config struct {
	PoolConfig      clientpool.PoolConfig `yaml:"pool_config,omitempty"`
	ExtraQueryDelay time.Duration         `yaml:"extra_query_delay,omitempty"`
}

// RegisterFlags registers distributor-related flags.
func (cfg *Config) RegisterFlags(fs *flag.FlagSet) {
	cfg.PoolConfig.RegisterFlagsWithPrefix("querier", fs)
	fs.DurationVar(&cfg.ExtraQueryDelay, "querier.extra-query-delay", 0, "Time to wait before sending more than the minimum successful query requests.")
}

type Querier struct {
	services.Service
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	cfg    Config
	logger log.Logger

	ingestersRing   ring.ReadRing
	pool            *ring_client.Pool
	ingesterQuerier *IngesterQuerier
}

func New(cfg Config, ingestersRing ring.ReadRing, factory ring_client.PoolFactory, logger log.Logger) (*Querier, error) {
	q := &Querier{
		cfg:           cfg,
		logger:        logger,
		ingestersRing: ingestersRing,
		pool:          clientpool.NewPool(cfg.PoolConfig, ingestersRing, factory, clients, logger),
	}
	var err error
	q.subservices, err = services.NewManager(q.pool)
	if err != nil {
		return nil, errors.Wrap(err, "services manager")
	}
	q.subservicesWatcher = services.NewFailureWatcher()
	q.subservicesWatcher.WatchManager(q.subservices)
	q.Service = services.NewBasicService(q.starting, q.running, q.stopping)
	q.ingesterQuerier = NewIngesterQuerier(q.pool, ingestersRing, cfg.ExtraQueryDelay)
	return q, nil
}

func (q *Querier) starting(ctx context.Context) error {
	return services.StartManagerAndAwaitHealthy(ctx, q.subservices)
}

func (q *Querier) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-q.subservicesWatcher.Chan():
		return errors.Wrap(err, "distributor subservice failed")
	}
}

func (q *Querier) stopping(_ error) error {
	return services.StopManagerAndAwaitStopped(context.Background(), q.subservices)
}

func (q *Querier) ProfileTypes(ctx context.Context) ([]string, error) {
	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(ic IngesterQueryClient) ([]string, error) {
		res, err := ic.ProfileTypes(ctx, connect.NewRequest(&ingestv1.ProfileTypesRequest{}))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, err
	}
	return uniqueSortedStrings(responses), nil
}

func (q *Querier) LabelValues(ctx context.Context, name string) ([]string, error) {
	responses, err := forAllIngesters(ctx, q.ingesterQuerier, func(ic IngesterQueryClient) ([]string, error) {
		res, err := ic.LabelValues(ctx, connect.NewRequest(&ingestv1.LabelValuesRequest{
			Name: name,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.Names, nil
	})
	if err != nil {
		return nil, err
	}
	return uniqueSortedStrings(responses), nil
}

func uniqueSortedStrings(responses []responseFromIngesters[[]string]) []string {
	total := 0
	for _, r := range responses {
		total += len(r.response)
	}
	unique := make(map[string]struct{}, total)
	result := make([]string, 0, total)
	for _, r := range responses {
		for _, elem := range r.response {
			if _, ok := unique[elem]; !ok {
				unique[elem] = struct{}{}
				result = append(result, elem)
			}
		}
	}
	sort.Strings(result)
	return result
}
