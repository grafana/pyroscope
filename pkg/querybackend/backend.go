package querybackend

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/grpcclient"
	"github.com/grafana/dskit/services"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/util"
)

type Config struct {
	Address          string            `yaml:"address"`
	GRPCClientConfig grpcclient.Config `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate between the query-frontends and the query-schedulers."`
	ClientTimeout    time.Duration     `yaml:"client_timeout"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&cfg.Address, "query-backend.address", "localhost:9095", "")
	f.DurationVar(&cfg.ClientTimeout, "query-backend.client-timeout", 30*time.Second, "Timeout for query-backend client requests.")
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
	req *queryv1.InvokeRequest,
) (*queryv1.InvokeResponse, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryBackend.Invoke")
	defer span.Finish()

	switch r := req.QueryPlan.Root; r.Type {
	case queryv1.QueryNode_MERGE:
		return q.merge(ctx, req, r.Children)
	case queryv1.QueryNode_READ:
		return q.read(ctx, req, r.Blocks)
	default:
		panic("query plan: unknown node type")
	}
}

func (q *QueryBackend) merge(
	ctx context.Context,
	request *queryv1.InvokeRequest,
	children []*queryv1.QueryNode,
) (*queryv1.InvokeResponse, error) {
	request.QueryPlan = nil
	m := newAggregator(request)
	g, ctx := errgroup.WithContext(ctx)
	for _, child := range children {
		req := request.CloneVT()
		req.QueryPlan = &queryv1.QueryPlan{
			Root: child,
		}
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
	blocks []*metastorev1.BlockMeta,
) (*queryv1.InvokeResponse, error) {
	request.QueryPlan = &queryv1.QueryPlan{
		Root: &queryv1.QueryNode{
			Blocks: blocks,
		},
	}
	return q.blockReader.Invoke(ctx, request)
}
