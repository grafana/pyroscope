package query_frontend

import (
	"context"
	"math/rand"
	"slices"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/tenant"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block/metadata"
	querybackendclient "github.com/grafana/pyroscope/pkg/experiment/query_backend/client"
	queryplan "github.com/grafana/pyroscope/pkg/experiment/query_backend/query_plan"
	"github.com/grafana/pyroscope/pkg/frontend"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

var _ querierv1connect.QuerierServiceClient = (*QueryFrontend)(nil)

type QueryFrontend struct {
	logger log.Logger
	limits frontend.Limits

	metadataQueryClient metastorev1.MetadataQueryServiceClient
	tenantServiceClient metastorev1.TenantServiceClient
	querybackendClient  *querybackendclient.Client
}

func NewQueryFrontend(
	logger log.Logger,
	limits frontend.Limits,
	metadataQueryClient metastorev1.MetadataQueryServiceClient,
	tenantServiceClient metastorev1.TenantServiceClient,
	querybackendClient *querybackendclient.Client,
) *QueryFrontend {
	return &QueryFrontend{
		logger:              logger,
		limits:              limits,
		metadataQueryClient: metadataQueryClient,
		tenantServiceClient: tenantServiceClient,
		querybackendClient:  querybackendClient,
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
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	blocks, err := q.QueryMetadata(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return new(queryv1.QueryResponse), nil
	}

	// Randomize the order of blocks to avoid hotspots.
	xrand.Shuffle(len(blocks), func(i, j int) {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	})
	p := queryplan.Build(blocks, 4, 20)

	resp, err := q.querybackendClient.Invoke(ctx, &queryv1.InvokeRequest{
		Tenant:        tenants,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		LabelSelector: req.LabelSelector,
		Options:       &queryv1.InvokeOptions{},
		QueryPlan:     p,
		Query:         req.Query,
	})
	if err != nil {
		return nil, err
	}
	// TODO(kolesnikovae): Extend diagnostics
	if resp.Diagnostics == nil {
		resp.Diagnostics = new(queryv1.Diagnostics)
	}
	resp.Diagnostics.QueryPlan = p
	return &queryv1.QueryResponse{Reports: resp.Reports}, nil
}

func (q *QueryFrontend) QueryMetadata(
	ctx context.Context,
	req *queryv1.QueryRequest,
) ([]*metastorev1.BlockMeta, error) {
	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	matchers, err := parser.ParseMetricSelector(req.LabelSelector)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	matchers = slices.DeleteFunc(matchers, func(m *labels.Matcher) bool {
		return m.Name != phlaremodel.LabelNameServiceName
	})
	if len(matchers) == 0 {
		matchers = []*labels.Matcher{{
			Name:  metadata.LabelNameTenantDataset,
			Value: metadata.LabelValueDatasetIndex,
			Type:  labels.MatchEqual,
		}}
	}
	query := matchersToLabelSelector(matchers)
	md, err := q.metadataQueryClient.QueryMetadata(ctx, &metastorev1.QueryMetadataRequest{
		TenantId:  tenants,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
		Query:     query,
	})
	if err != nil {
		return nil, err
	}
	return md.Blocks, nil
}
