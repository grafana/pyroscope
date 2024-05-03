package querier

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

type queryScope struct {
	*querierv1.QueryScope

	blockIds []string
}

func (q *Querier) AnalyzeQuery(ctx context.Context, req *connect.Request[querierv1.AnalyzeQueryRequest]) (*connect.Response[querierv1.AnalyzeQueryResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "AnalyzeQuery")
	defer sp.Finish()

	plan, err := q.blockSelect(ctx, model.Time(req.Msg.Start), model.Time(req.Msg.End))
	ingesterQueryScope, storeGatewayQueryScope, deduplicationNeeded := getDataFromPlan(plan)

	blockStatsFromReplicas, err := q.getBlockStatsFromIngesters(ctx, plan, ingesterQueryScope.blockIds)
	if err != nil {
		return nil, err
	}
	addBlockStatsToQueryScope(blockStatsFromReplicas, ingesterQueryScope)

	blockStatsFromReplicas, err = q.getBlockStatsFromStoreGateways(ctx, plan, storeGatewayQueryScope.blockIds)
	if err != nil {
		return nil, err
	}
	addBlockStatsToQueryScope(blockStatsFromReplicas, storeGatewayQueryScope)

	queriedSeries, err := q.getQueriedSeriesCount(ctx, req.Msg)
	if err != nil {
		return nil, err
	}

	res := createResponse(ingesterQueryScope, storeGatewayQueryScope)
	res.QueryImpact.DeduplicationNeeded = deduplicationNeeded
	res.QueryImpact.TotalQueriedSeries = queriedSeries

	return connect.NewResponse(res), nil
}

func getDataFromPlan(plan blockPlan) (ingesterQueryScope *queryScope, storeGatewayQueryScope *queryScope, deduplicationNeeded bool) {
	ingesterQueryScope = &queryScope{
		QueryScope: &querierv1.QueryScope{
			ComponentType: "Short term storage",
		},
	}
	storeGatewayQueryScope = &queryScope{
		QueryScope: &querierv1.QueryScope{
			ComponentType: "Long term storage",
		},
	}
	deduplicationNeeded = false
	for _, planEntry := range plan {
		deduplicationNeeded = deduplicationNeeded || planEntry.Deduplication
		if planEntry.InstanceType == ingesterInstance {
			ingesterQueryScope.ComponentCount += 1
			ingesterQueryScope.NumBlocks += uint64(len(planEntry.Ulids))
			ingesterQueryScope.blockIds = append(ingesterQueryScope.blockIds, planEntry.Ulids...)
		} else {
			storeGatewayQueryScope.ComponentCount += 1
			storeGatewayQueryScope.NumBlocks += uint64(len(planEntry.Ulids))
			storeGatewayQueryScope.blockIds = append(storeGatewayQueryScope.blockIds, planEntry.Ulids...)
		}
	}
	return ingesterQueryScope, storeGatewayQueryScope, deduplicationNeeded
}

func (q *Querier) getBlockStatsFromIngesters(ctx context.Context, plan blockPlan, ingesterBlockIds []string) ([]ResponseFromReplica[*ingestv1.GetBlockStatsResponse], error) {
	var blockStatsFromReplicas []ResponseFromReplica[*ingestv1.GetBlockStatsResponse]
	blockStatsFromReplicas, err := forAllPlannedIngesters(ctx, q.ingesterQuerier, plan, func(ctx context.Context, iq IngesterQueryClient, hint *ingestv1.Hints) (*ingestv1.GetBlockStatsResponse, error) {
		stats, err := iq.GetBlockStats(ctx, connect.NewRequest(&ingestv1.GetBlockStatsRequest{Ulids: ingesterBlockIds}))
		if err != nil {
			return nil, err
		}
		return stats.Msg, err
	})
	return blockStatsFromReplicas, err
}

func (q *Querier) getBlockStatsFromStoreGateways(ctx context.Context, plan blockPlan, storeGatewayBlockIds []string) ([]ResponseFromReplica[*ingestv1.GetBlockStatsResponse], error) {
	tenantId, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, err
	}
	blockStatsFromReplicas, err := forAllPlannedStoreGateways(ctx, tenantId, q.storeGatewayQuerier, plan, func(ctx context.Context, sq StoreGatewayQueryClient, hint *ingestv1.Hints) (*ingestv1.GetBlockStatsResponse, error) {
		stats, err := sq.GetBlockStats(ctx, connect.NewRequest(&ingestv1.GetBlockStatsRequest{Ulids: storeGatewayBlockIds}))
		if err != nil {
			return nil, err
		}
		return stats.Msg, err
	})
	return blockStatsFromReplicas, nil
}

func addBlockStatsToQueryScope(blockStatsFromReplicas []ResponseFromReplica[*ingestv1.GetBlockStatsResponse], queryScope *queryScope) {
	for _, r := range blockStatsFromReplicas {
		for _, stats := range r.response.BlockStats {
			queryScope.NumSeries += stats.NumSeries
			queryScope.NumProfiles += stats.NumProfiles
			queryScope.NumSamples += stats.NumSamples
			queryScope.IndexBytes += stats.IndexBytes
			queryScope.ProfileBytes += stats.ProfilesBytes
			queryScope.SymbolBytes += stats.SymbolsBytes
		}
	}
}

func (q *Querier) getQueriedSeriesCount(ctx context.Context, req *querierv1.AnalyzeQueryRequest) (uint64, error) {
	matchers, err := createMatchersFromQuery(req.Query)
	if err != nil {
		return 0, err
	}
	resSeries, err := q.Series(ctx, connect.NewRequest(&querierv1.SeriesRequest{
		Matchers: matchers,
		Start:    req.Start,
		End:      req.End,
	}))
	if err != nil {
		return 0, err
	}
	return uint64(len(resSeries.Msg.LabelsSet)), nil
}

func createMatchersFromQuery(query string) ([]string, error) {
	var matchers []*labels.Matcher
	var err error
	if query != "" {
		matchers, err = parser.ParseMetricSelector(query)
		if err != nil {
			return nil, err
		}
	}
	for _, matcher := range matchers {
		if matcher.Name == labels.MetricName {
			matcher.Name = phlaremodel.LabelNameProfileType
		}
	}
	return []string{convertMatchersToString(matchers)}, nil
}

func createResponse(ingesterQueryScope *queryScope, storeGatewayQueryScope *queryScope) *querierv1.AnalyzeQueryResponse {
	totalBytes := ingesterQueryScope.IndexBytes +
		ingesterQueryScope.ProfileBytes +
		ingesterQueryScope.SymbolBytes +
		storeGatewayQueryScope.IndexBytes +
		storeGatewayQueryScope.ProfileBytes +
		storeGatewayQueryScope.SymbolBytes

	return &querierv1.AnalyzeQueryResponse{
		QueryScopes: []*querierv1.QueryScope{ingesterQueryScope.QueryScope, storeGatewayQueryScope.QueryScope},
		QueryImpact: &querierv1.QueryImpact{
			TotalBytesInTimeRange: totalBytes,
		},
	}
}
