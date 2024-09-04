package query_frontend

import (
	"context"
	"math/rand"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/tenant"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	metastoreclient "github.com/grafana/pyroscope/pkg/experiment/metastore/client"
	querybackend "github.com/grafana/pyroscope/pkg/experiment/query_backend"
	querybackendclient "github.com/grafana/pyroscope/pkg/experiment/query_backend/client"
	queryplan "github.com/grafana/pyroscope/pkg/experiment/query_backend/query_plan"
	"github.com/grafana/pyroscope/pkg/frontend"
)

var _ querierv1connect.QuerierServiceClient = (*QueryFrontend)(nil)

type QueryFrontend struct {
	logger       log.Logger
	limits       frontend.Limits
	metastore    *metastoreclient.Client
	querybackend *querybackendclient.Client
}

func NewQueryFrontend(
	logger log.Logger,
	limits frontend.Limits,
	metastore *metastoreclient.Client,
	querybackend *querybackendclient.Client,
) *QueryFrontend {
	return &QueryFrontend{
		logger:       logger,
		limits:       limits,
		metastore:    metastore,
		querybackend: querybackend,
	}
}

var xrand = rand.New(rand.NewSource(4349676827832284783))

func (q *QueryFrontend) Query(
	ctx context.Context,
	req *queryv1.QueryRequest,
) (*queryv1.QueryResponse, error) {
	// TODO(kolesnikovae):
	// This method is supposed to be the entry point of the read path
	// in the future versions. Therefore, validation, overrides, and
	// rest of the request handling should be moved here.
	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	md, err := q.metastore.QueryMetadata(ctx, &metastorev1.QueryMetadataRequest{
		TenantId:  tenants,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Query:     req.LabelSelector,
	})
	if err != nil {
		return nil, err
	}
	if len(md.Blocks) == 0 {
		return new(queryv1.QueryResponse), nil
	}

	// TODO(kolesnikovae): Implement query planning.
	// Randomize the order of blocks to avoid hotspots.
	xrand.Shuffle(len(md.Blocks), func(i, j int) {
		md.Blocks[i], md.Blocks[j] = md.Blocks[j], md.Blocks[i]
	})
	p := queryplan.Build(md.Blocks, 4, 20)

	resp, err := q.querybackend.Invoke(ctx, &queryv1.InvokeRequest{
		Tenant:        tenants,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		LabelSelector: req.LabelSelector,
		Options:       &queryv1.InvokeOptions{},
		QueryPlan:     p.Proto(),
		Query:         req.Query,
	})
	if err != nil {
		return nil, err
	}
	// TODO(kolesnikovae): Diagnostics.
	return &queryv1.QueryResponse{Reports: resp.Reports}, nil
}

// querySingle is a helper method that expects a single report
// of the appropriate type in the response; this method should
// be used to implement adapter to the old query API.
func (q *QueryFrontend) querySingle(
	ctx context.Context,
	req *queryv1.QueryRequest,
) (*queryv1.Report, error) {
	if len(req.Query) != 1 {
		// Nil report is a valid response.
		return nil, nil
	}
	t := querybackend.QueryReportType(req.Query[0].QueryType)
	resp, err := q.Query(ctx, req)
	if err != nil {
		return nil, err
	}
	return findReport(t, resp.Reports), nil
}
