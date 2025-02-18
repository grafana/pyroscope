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
	queryplan "github.com/grafana/pyroscope/pkg/experiment/query_backend/query_plan"
	"github.com/grafana/pyroscope/pkg/frontend"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

var _ querierv1connect.QuerierServiceClient = (*QueryFrontend)(nil)

type QueryBackend interface {
	Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)
}

type QueryFrontend struct {
	logger log.Logger
	limits frontend.Limits

	metadataQueryClient metastorev1.MetadataQueryServiceClient
	tenantServiceClient metastorev1.TenantServiceClient
	querybackend        QueryBackend
}

func NewQueryFrontend(
	logger log.Logger,
	limits frontend.Limits,
	metadataQueryClient metastorev1.MetadataQueryServiceClient,
	tenantServiceClient metastorev1.TenantServiceClient,
	querybackendClient QueryBackend,
) *QueryFrontend {
	return &QueryFrontend{
		logger:              logger,
		limits:              limits,
		metadataQueryClient: metadataQueryClient,
		tenantServiceClient: tenantServiceClient,
		querybackend:        querybackendClient,
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
	// TODO(kolesnikovae): Should be dynamic.
	p := queryplan.Build(blocks, 4, 20)

	resp, err := q.querybackend.Invoke(ctx, &queryv1.InvokeRequest{
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
	query := &metastorev1.QueryMetadataRequest{
		TenantId:  tenants,
		StartTime: req.StartTime,
		EndTime:   req.EndTime,
	}

	// Delete all matchers but service_name with strict match. If no matchers
	// left, request the dataset index for query backend to lookup block datasets
	// locally.
	matchers = slices.DeleteFunc(matchers, func(m *labels.Matcher) bool {
		return !(m.Name == phlaremodel.LabelNameServiceName && m.Type == labels.MatchEqual)
	})
	if len(matchers) == 0 {
		// We preserve the __tenant_dataset__= label: this is needed for the
		// query backend to identify that the dataset is the tenant-wide index,
		// and a dataset lookup is needed.
		query.Labels = []string{metadata.LabelNameTenantDataset}
		matchers = []*labels.Matcher{{
			Name:  metadata.LabelNameTenantDataset,
			Value: metadata.LabelValueDatasetTSDBIndex,
			Type:  labels.MatchEqual,
		}}
	}

	query.Query = matchersToLabelSelector(matchers)
	md, err := q.metadataQueryClient.QueryMetadata(ctx, query)
	if err != nil {
		return nil, err
	}

	return md.Blocks, nil
}
