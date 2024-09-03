package query_backend

import (
	"context"
	"flag"
	"fmt"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	queryplan "github.com/grafana/pyroscope/pkg/experiment/query_backend/query_plan"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/util"
)

const defaultConcurrencyLimit = 25

type Config struct {
	Address          string            `yaml:"address"`
	GRPCClientConfig grpcclient.Config `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate between the query-frontends and the query-schedulers."`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.Address, "query-backend.address", "localhost:9095", "")
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix("query-backend.grpc-client-config", f)
}

func (cfg *Config) Validate() error {
	if cfg.Address == "" {
		return fmt.Errorf("query-backend.address is required")
	}
	return cfg.GRPCClientConfig.Validate()
}

type QueryHandler interface {
	Invoke(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)
}

type QueryBackend struct {
	service services.Service
	queryv1.QueryBackendServiceServer

	config Config
	logger log.Logger
	reg    prometheus.Registerer

	backendClient QueryHandler
	blockReader   QueryHandler

	concurrency uint32
	running     atomic.Uint32
}

func New(
	config Config,
	logger log.Logger,
	reg prometheus.Registerer,
	backendClient QueryHandler,
	blockReader QueryHandler,
) (*QueryBackend, error) {
	q := QueryBackend{
		config:        config,
		logger:        logger,
		reg:           reg,
		backendClient: backendClient,
		blockReader:   blockReader,

		concurrency: defaultConcurrencyLimit,
	}
	q.service = services.NewIdleService(q.starting, q.stopping)
	return &q, nil
}

func (q *QueryBackend) Service() services.Service      { return q.service }
func (q *QueryBackend) starting(context.Context) error { return nil }
func (q *QueryBackend) stopping(error) error           { return nil }

func (q *QueryBackend) Invoke(
	ctx context.Context,
	req *queryv1.InvokeRequest,
) (*queryv1.InvokeResponse, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryBackend.Invoke")
	defer span.Finish()

	p := queryplan.Open(req.QueryPlan)
	switch r := p.Root(); r.Type {
	case queryplan.NodeMerge:
		return q.merge(ctx, req, r.Children())
	case queryplan.NodeRead:
		return q.withThrottling(func() (*queryv1.InvokeResponse, error) {
			return q.read(ctx, req, r.Blocks())
		})
	default:
		panic("query plan: unknown node type")
	}
}

func (q *QueryBackend) merge(
	ctx context.Context,
	request *queryv1.InvokeRequest,
	children iter.Iterator[*queryplan.Node],
) (*queryv1.InvokeResponse, error) {
	request.QueryPlan = nil
	m := newAggregator(request)
	g, ctx := errgroup.WithContext(ctx)
	for children.Next() {
		req := request.CloneVT()
		req.QueryPlan = children.At().Plan().Proto()
		g.Go(util.RecoverPanic(func() error {
			// TODO: Speculative retry.
			return m.aggregateResponse(q.backendClient.Invoke(ctx, req))
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return m.response()
}

func (q *QueryBackend) read(
	ctx context.Context,
	request *queryv1.InvokeRequest,
	blocks iter.Iterator[*metastorev1.BlockMeta],
) (*queryv1.InvokeResponse, error) {
	request.QueryPlan = &queryv1.QueryPlan{
		Blocks: iter.MustSlice(blocks),
	}
	return q.blockReader.Invoke(ctx, request)
}

func (q *QueryBackend) withThrottling(fn func() (*queryv1.InvokeResponse, error)) (*queryv1.InvokeResponse, error) {
	if q.running.Inc() > q.concurrency {
		return nil, status.Error(codes.ResourceExhausted, "all minions are busy, please try later")
	}
	defer q.running.Dec()
	return fn()
}
