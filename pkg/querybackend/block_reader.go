package querybackend

import (
	"context"
	"fmt"

	"github.com/go-kit/log"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/querybackend/block"
	"github.com/grafana/pyroscope/pkg/util"
)

// Block reader reads objects from the object storage. Each block is currently
// represented by a single object.
//
// An object consists of a set of "tenant services" – regions within the block
// that include data of a specific tenant service. Each such tenant service
// consists of 3 sections: profile table, TSDB, and symbol database.
//
// A single Invoke request typically spans multiple blocks (objects).
// Querying an object involves processing multiple tenant services in parallel.
// Multiple parallel queries can be executed on the same tenant service.
//
// Thus, queries share the same "execution context": the object and a tenant
// service:
//
// object-a    service-a   query-a
//                         query-b
//             service-b   query-a
//                         query-b
// object-b    service-a   query-a
//                         query-b
//             service-b   query-a
//                         query-b
//

type BlockReader struct {
	log     log.Logger
	storage objstore.Bucket

	// TODO:
	//  - Use a worker pool instead of the errgroup.
	//  - Reusable query context.
	//  - Query pipelining: currently, queries share the same context,
	//    and reuse resources, but the data is processed independently.
	//    Instead, they should share the processing pipeline, if possible.
}

func NewBlockReader(logger log.Logger, storage objstore.Bucket) *BlockReader {
	return &BlockReader{
		log:     logger,
		storage: storage,
	}
}

func (b *BlockReader) Invoke(
	ctx context.Context,
	req *querybackendv1.InvokeRequest,
) (*querybackendv1.InvokeResponse, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "BlockReader.Invoke")
	defer span.Finish()
	vr, err := validateRequest(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "request validation failed: %v", err)
	}
	g, ctx := errgroup.WithContext(ctx)
	m := newAggregator(req)
	for _, md := range req.QueryPlan.Blocks {
		obj := block.NewObject(b.storage, md)
		for _, meta := range md.TenantServices {
			c := newQueryContext(ctx, b.log, meta, vr, obj)
			for _, query := range req.Query {
				q := query
				g.Go(util.RecoverPanic(func() error {
					r, err := executeQuery(c, q)
					if err != nil {
						return err
					}
					return m.aggregateReport(r)
				}))
			}
		}
	}
	if err = g.Wait(); err != nil {
		return nil, err
	}
	return m.response()
}

type request struct {
	src       *querybackendv1.InvokeRequest
	matchers  []*labels.Matcher
	startTime int64 // Unix nano.
	endTime   int64 // Unix nano.
}

func validateRequest(req *querybackendv1.InvokeRequest) (*request, error) {
	if len(req.Query) == 0 {
		return nil, fmt.Errorf("no queries provided")
	}
	if req.QueryPlan == nil || len(req.QueryPlan.Blocks) == 0 {
		return nil, fmt.Errorf("no blocks planned")
	}
	matchers, err := parser.ParseMetricSelector(req.LabelSelector)
	if err != nil {
		return nil, fmt.Errorf("label selection is invalid: %w", err)
	}
	// TODO: Validate the rest, just in case.
	r := request{
		src:       req,
		matchers:  matchers,
		startTime: model.Time(req.StartTime).UnixNano(),
		endTime:   model.Time(req.EndTime).UnixNano(),
	}
	return &r, nil
}
