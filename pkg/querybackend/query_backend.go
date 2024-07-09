package querybackend

import (
	"context"
	"math"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/util"
)

type Config struct {
	// TODO: Type-specific configuration:
	//   - worker-pool
	//   - serverless
	Address string
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
	// TODO: Return codes.ResourceExhausted if the query
	//  exceeds the budget of the current instance: e.g,
	//  100MB of in-flight data, or 30 queries/blocks, or
	//  100 merges, or memory available, etc.
	if n := ranges(req); n > 0 {
		return q.invoke(ctx, req, q.backendClient, groupsOf(n))
	}
	return q.invoke(ctx, req, q.blockReader, eachBlock)
}

func ranges(req *querybackendv1.InvokeRequest) int64 {
	if len(req.Blocks) > int(req.Options.MaxBlockReadsPerWorker) {
		if len(req.Blocks) > int(req.Options.MaxBlockMergesPerWorker) {
			return req.Options.MaxBlockReadsPerWorker
		}
		return req.Options.MaxBlockMergesPerWorker
	}
	return 0
}

type splitFunc func(*querybackendv1.InvokeRequest) [][]*metastorev1.BlockMeta

func eachBlock(request *querybackendv1.InvokeRequest) [][]*metastorev1.BlockMeta {
	groups := make([][]*metastorev1.BlockMeta, len(request.Blocks))
	for i := range request.Blocks {
		groups[i] = []*metastorev1.BlockMeta{request.Blocks[i]}
	}
	return groups
}

func groupsOf(n int64) splitFunc {
	return func(request *querybackendv1.InvokeRequest) [][]*metastorev1.BlockMeta {
		return uniformSplit(request.Blocks, n)
	}
}

func uniformSplit[T any](s []T, max int64) [][]T {
	// Find number of parts.
	n := math.Ceil(float64(len(s)) / float64(max))
	// Find optimal part size.
	o := int(math.Ceil(float64(len(s)) / n))
	var ret [][]T // Prevent referencing the source slice.
	for i := 0; i < len(s); i += o {
		r := i + o
		if r > len(s) {
			r = len(s)
		}
		it := s[i:r]
		ret = append(ret, it)
	}
	return ret
}

func (q *QueryBackend) invoke(
	ctx context.Context,
	request *querybackendv1.InvokeRequest,
	handler QueryHandler,
	split splitFunc,
) (*querybackendv1.InvokeResponse, error) {
	blocks := request.Blocks
	parts := split(request)
	request.Blocks = nil
	defer func() {
		request.Blocks = blocks
	}()
	requests := make([]*querybackendv1.InvokeRequest, len(parts))
	for i := range requests {
		requests[i] = request.CloneVT()
		requests[i].Blocks = parts[i]
	}
	m := newMerger()
	g, ctx := errgroup.WithContext(ctx)
	// TODO: Speculative retry.
	for i := range requests {
		i := i
		g.Go(util.RecoverPanic(func() error {
			return m.mergeResponse(handler.Invoke(ctx, requests[i]))
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return m.response()
}
