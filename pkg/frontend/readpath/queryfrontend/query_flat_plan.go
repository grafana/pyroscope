package queryfrontend

import (
	"context"
	"fmt"

	"github.com/grafana/dskit/tenant"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/querybackend"
)

func (q *QueryFrontend) queryFlatPlan(
	ctx context.Context,
	req *queryv1.QueryRequest,
) (*queryv1.Report, error) {
	if len(req.Query) != 1 {
		// Nil report is a valid response.
		return nil, nil
	}
	reportType := querybackend.QueryReportType(req.Query[0].QueryType)

	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	blocks, err := q.QueryMetadata(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return nil, nil
	}

	// Only check for symbolization if all tenants have it enabled
	shouldSymbolize := q.shouldSymbolize(tenants, blocks)

	modifiedQueries := make([]*queryv1.Query, len(req.Query))
	for i, originalQuery := range req.Query {
		modifiedQueries[i] = originalQuery.CloneVT()

		// If we need symbolization and this is a TREE query, convert it to PPROF
		if shouldSymbolize && originalQuery.QueryType == queryv1.QueryType_QUERY_TREE {
			modifiedQueries[i].QueryType = queryv1.QueryType_QUERY_PPROF
			modifiedQueries[i].Pprof = &queryv1.PprofQuery{
				MaxNodes: 0,
			}
			modifiedQueries[i].Tree = nil
		}
	}

	g, ctx := errgroup.WithContext(ctx)
	reportAggregator := newAggregator(req)
	for _, block := range blocks {
		g.Go(func() error {
			plan := &queryv1.QueryPlan{
				Root: &queryv1.QueryNode{
					Type:   queryv1.QueryNode_READ,
					Blocks: []*metastorev1.BlockMeta{block},
				},
			}
			resp, err := q.querybackend.Invoke(ctx, &queryv1.InvokeRequest{
				Tenant:        tenants,
				StartTime:     req.StartTime,
				EndTime:       req.EndTime,
				LabelSelector: req.LabelSelector,
				Options: &queryv1.InvokeOptions{
					SanitizeOnMerge: q.limits.QuerySanitizeOnMerge(tenants[0]),
				},
				QueryPlan: plan,
				Query:     modifiedQueries,
			})
			if err != nil {
				return err
			}
			for _, report := range resp.Reports {
				if report.ReportType == reportType {
					return reportAggregator.aggregateReport(report)
				}
			}
			return fmt.Errorf("no report of type %s", reportType)
		})
	}

	err = g.Wait()
	if err != nil {
		return nil, err
	}

	aggregated, err := reportAggregator.response()
	if err != nil {
		return nil, err
	}

	if aggregated == nil || len(aggregated.Reports) == 0 {
		return nil, nil
	}

	if shouldSymbolize {
		err = q.processAndSymbolizeProfiles(ctx, aggregated.Reports, req.Query)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("symbolizing profiles: %v", err))
		}
	}

	return aggregated.Reports[0], nil
}
