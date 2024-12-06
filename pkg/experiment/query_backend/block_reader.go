package query_backend

import (
	"context"
	"fmt"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/util"
)

// Block reader reads objects from the object storage. Each block is currently
// represented by a single object.
//
// An object consists of data sets – regions within the block that include some
// tenant data. Each such dataset consists of 3 sections: profile table, TSDB,
// and symbol database.
//
// A single Invoke request typically spans multiple blocks (objects).
// Querying an object involves processing multiple datasets in parallel.
// Multiple parallel queries can be executed on the same tenant dataset.
//
// Thus, queries share the same "execution context": the object and a tenant
// dataset.
//
// object-a    dataset-a   query-a
//                         query-b
//             dataset-b   query-a
//                         query-b
// object-b    dataset-a   query-a
//                         query-b
//             dataset-b   query-a
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
	req *queryv1.InvokeRequest,
) (*queryv1.InvokeResponse, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "BlockReader.Invoke")
	defer span.Finish()
	r, err := validateRequest(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "request validation failed: %v", err)
	}
	r.setTraceTags(span)

	g, ctx := errgroup.WithContext(ctx)
	agg := newAggregator(req)

	qcs := make([]*queryContext, 0, len(req.Query)*len(req.QueryPlan.Root.Blocks))
	for _, md := range req.QueryPlan.Root.Blocks {
		object := block.NewObject(b.storage, md)
		for _, ds := range md.Datasets {
			dataset := block.NewDataset(ds, object)
			qcs = append(qcs, newQueryContext(ctx, b.log, r, agg, dataset))
		}
	}

	for _, c := range qcs {
		for _, query := range req.Query {
			q := query
			g.Go(util.RecoverPanic(func() error {
				execErr := executeQuery(c, q)
				if execErr != nil && objstore.IsNotExist(b.storage, execErr) {
					level.Warn(b.log).Log("msg", "object not found", "err", execErr)
					return nil
				}
				return execErr
			}))
		}
	}

	if err = g.Wait(); err != nil {
		return nil, err
	}
	return agg.response()
}

type request struct {
	src       *queryv1.InvokeRequest
	matchers  []*labels.Matcher
	startTime int64 // Unix nano.
	endTime   int64 // Unix nano.
}

func (r *request) setTraceTags(span opentracing.Span) {
	if r.src == nil {
		return
	}
	span.SetTag("start_time", model.Time(r.src.StartTime).Time().String())
	span.SetTag("end_time", model.Time(r.src.EndTime).Time().String())
	span.SetTag("matchers", r.src.LabelSelector)

	if len(r.src.Query) > 0 {
		queryTypes := make([]string, 0, len(r.src.Query))
		for _, q := range r.src.Query {
			queryTypes = append(queryTypes, q.QueryType.String())
		}
		span.SetTag("query_types", queryTypes)
	}
}

func validateRequest(req *queryv1.InvokeRequest) (*request, error) {
	if len(req.Query) == 0 {
		return nil, fmt.Errorf("no queries provided")
	}
	if req.QueryPlan == nil || len(req.QueryPlan.Root.Blocks) == 0 {
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
