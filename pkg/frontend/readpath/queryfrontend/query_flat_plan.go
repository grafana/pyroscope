package queryfrontend

import (
	"context"
	"fmt"
	"strings"

	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
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
	sp, _ := opentracing.StartSpanFromContext(ctx, "QueryFrontend.queryFlatPlan")

	sp.SetTag("start_time", req.StartTime)
	sp.SetTag("end_time", req.EndTime)
	sp.SetTag(
		"label_selector",
		req.LabelSelector,
	)
	defer sp.Finish()

	if len(req.Query) != 1 {
		// Nil report is a valid response.
		return nil, nil
	}
	queryType := querybackend.QueryReportType(req.Query[0].QueryType)
	sp.SetTag("query_type", queryType)

	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	sp.SetTag("tenant_ids", strings.Join(tenants, ","))

	blocks, err := q.QueryMetadata(ctx, req)
	if err != nil {
		return nil, err
	}
	sp.SetTag("block_count", len(blocks))
	if len(blocks) == 0 {
		return nil, nil
	}

	// Only check for symbolization if all tenants have it enabled
	shouldSymbolize := q.shouldSymbolize(tenants, blocks)
	sp.SetTag("should_symbolize", shouldSymbolize)

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
			datasetSize := uint64(0)
			if len(block.Datasets) > 0 {
				datasetSize = block.Datasets[0].Size
			}
			sp.LogFields(
				log.String("msg", "querying block"),
				log.String("block_id", block.Id),
				log.String("block_size", fmt.Sprint(block.Size)),
				log.String("dataset_count", fmt.Sprint(len(block.Datasets))),
				log.String("first_dataset_size", fmt.Sprint(datasetSize)),
			)
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
				if report.ReportType == queryType {
					sp.LogFields(
						log.String("msg", "got report"),
						log.String("report_type", queryType.String()),
						log.String("msg_size", fmt.Sprint(resp.SizeVT())),
					)
					return reportAggregator.aggregateReport(report)
				}
			}
			return fmt.Errorf("no report of type %s", queryType)
		})
	}

	err = g.Wait()
	if err != nil {
		return nil, err
	}

	aggregated := reportAggregator.response()

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
