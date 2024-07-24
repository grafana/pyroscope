package querybackend

import (
	"context"
	"flag"
	"fmt"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/querybackend/queryplan"
	"github.com/grafana/pyroscope/pkg/util"
)

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
	Invoke(context.Context, *querybackendv1.InvokeRequest) (*querybackendv1.InvokeResponse, error)
}

type QueryBackend struct {
	service services.Service
	querybackendv1.QueryBackendServiceServer

	config Config
	logger log.Logger
	reg    prometheus.Registerer

	backendClient QueryHandler
	blockReader   QueryHandler
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
	}
	q.service = services.NewIdleService(q.starting, q.stopping)
	return &q, nil
}

func (q *QueryBackend) Service() services.Service      { return q.service }
func (q *QueryBackend) starting(context.Context) error { return nil }
func (q *QueryBackend) stopping(error) error           { return nil }

func (q *QueryBackend) Invoke(
	ctx context.Context,
	req *querybackendv1.InvokeRequest,
) (*querybackendv1.InvokeResponse, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryBackend.Invoke")
	defer span.Finish()

	// TODO: Return codes.ResourceExhausted if the query
	//  exceeds the budget of the current instance: e.g,
	//  100MB of in-flight data, or 30 queries/blocks, or
	//  100 merges, or memory available, etc.

	p := queryplan.Open(req.QueryPlan)
	switch r := p.Root(); r.Type {
	case queryplan.NodeMerge:
		return q.merge(ctx, req, r.Children())
	case queryplan.NodeRead:
		return q.read(ctx, req, r.Blocks())
	default:
		panic("query plan: unknown node type")
	}
}

func (q *QueryBackend) merge(
	ctx context.Context,
	request *querybackendv1.InvokeRequest,
	children iter.Iterator[*queryplan.Node],
) (*querybackendv1.InvokeResponse, error) {
	request.QueryPlan = nil
	m := newMerger()
	g, ctx := errgroup.WithContext(ctx)
	for children.Next() {
		req := request.CloneVT()
		req.QueryPlan = children.At().Plan().Proto()
		g.Go(util.RecoverPanic(func() error {
			// TODO: Speculative retry.
			return m.mergeResponse(q.backendClient.Invoke(ctx, req))
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return m.response()
}

func (q *QueryBackend) read(
	ctx context.Context,
	request *querybackendv1.InvokeRequest,
	blocks iter.Iterator[*metastorev1.BlockMeta],
) (*querybackendv1.InvokeResponse, error) {
	request.QueryPlan = &querybackendv1.QueryPlan{
		Blocks: iter.MustSlice(blocks),
	}
	return q.blockReader.Invoke(ctx, request)
}
