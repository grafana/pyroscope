package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/operations"
	"github.com/grafana/pyroscope/pkg/querybackend/queryplan"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type QueryBackend interface {
	Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)
}

type MetastoreClient interface {
	metastorev1.MetadataQueryServiceClient
	metastorev1.TenantServiceClient
}

type Admin struct {
	service services.Service
	logger  log.Logger

	metastoreClient MetastoreClient
	queryBackend    QueryBackend
}

func New(
	logger log.Logger,
	metastoreClient MetastoreClient,
	queryBackend QueryBackend,
) *Admin {
	adm := &Admin{
		logger:          logger,
		metastoreClient: metastoreClient,
		queryBackend:    queryBackend,
	}
	adm.service = services.NewIdleService(adm.starting, adm.stopping)
	return adm
}

func (a *Admin) Service() services.Service {
	return a.service
}

func (a *Admin) starting(context.Context) error { return nil }
func (a *Admin) stopping(error) error           { return nil }

func (a *Admin) DiagnosticsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := diagnosticsPageContent{
			Now:       time.Now().UTC(),
			QueryType: "QUERY_PPROF",
			StartTime: "now-30m",
			EndTime:   "now",
		}

		// Fetch tenants for autocomplete
		if tenants, err := a.fetchTenants(r.Context()); err == nil {
			content.Tenants = tenants
		}

		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				content.Error = fmt.Sprintf("failed to parse form: %v", err)
				a.renderDiagnosticsPage(w, content)
				return
			}

			content.TenantID = r.FormValue("tenant_id")
			content.StartTime = r.FormValue("start_time")
			content.EndTime = r.FormValue("end_time")
			content.QueryType = r.FormValue("query_type")
			content.LabelSelector = r.FormValue("label_selector")
			content.PlanJSON = r.FormValue("plan_json")

			action := r.FormValue("action")
			switch action {
			case "create_plan":
				a.handleCreatePlan(r.Context(), &content)
			case "execute_query":
				a.handleExecuteQuery(r.Context(), &content)
			}
		}

		a.renderDiagnosticsPage(w, content)
	})
}

func (a *Admin) renderDiagnosticsPage(w http.ResponseWriter, content diagnosticsPageContent) {
	if err := pageTemplates.diagnosticsTemplate.Execute(w, content); err != nil {
		httputil.Error(w, err)
	}
}

func (a *Admin) fetchTenants(ctx context.Context) ([]string, error) {
	resp, err := a.metastoreClient.GetTenants(ctx, &metastorev1.GetTenantsRequest{})
	if err != nil {
		level.Warn(a.logger).Log("msg", "failed to fetch tenants for autocomplete", "err", err)
		return nil, err
	}
	return resp.TenantIds, nil
}

func (a *Admin) handleCreatePlan(ctx context.Context, content *diagnosticsPageContent) {
	if content.TenantID == "" {
		content.Error = "tenant ID is required"
		return
	}
	if content.StartTime == "" || content.EndTime == "" {
		content.Error = "start time and end time are required"
		return
	}

	startTime, err := operations.ParseTime(content.StartTime)
	if err != nil {
		content.Error = fmt.Sprintf("invalid start time: %v", err)
		return
	}
	endTime, err := operations.ParseTime(content.EndTime)
	if err != nil {
		content.Error = fmt.Sprintf("invalid end time: %v", err)
		return
	}

	start := time.Now()
	blocks, err := a.queryMetadata(ctx, content.TenantID, startTime.UnixMilli(), endTime.UnixMilli(), content.LabelSelector)
	content.MetadataQueryTime = time.Since(start)

	if err != nil {
		content.Error = fmt.Sprintf("failed to query metadata: %v", err)
		return
	}

	content.MetadataStats = buildMetadataStats(blocks, startTime, endTime)

	if len(blocks) == 0 {
		content.PlanJSON = "{}"
		return
	}

	plan := queryplan.Build(blocks, 4, 20)
	planJSON, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		content.Error = fmt.Sprintf("failed to marshal plan: %v", err)
		return
	}

	content.PlanJSON = string(planJSON)
	content.PlanTree = convertQueryPlanToTree(plan)
}

func buildMetadataStats(blocks []*metastorev1.BlockMeta, startTime, endTime time.Time) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Blocks found: %d\n", len(blocks))
	fmt.Fprintf(&sb, "Time range: %s to %s", startTime.UTC().Format(time.RFC3339), endTime.UTC().Format(time.RFC3339))

	if len(blocks) == 0 {
		return sb.String()
	}

	var totalBlockSize uint64
	var totalDatasetSize uint64
	var totalDatasets int

	var largestBlock, smallestBlock *metastorev1.BlockMeta
	var largestDataset, smallestDataset *metastorev1.Dataset
	var largestDatasetBlock, smallestDatasetBlock *metastorev1.BlockMeta

	for _, block := range blocks {
		totalBlockSize += block.Size

		if largestBlock == nil || block.Size > largestBlock.Size {
			largestBlock = block
		}
		if smallestBlock == nil || block.Size < smallestBlock.Size {
			smallestBlock = block
		}

		for _, ds := range block.Datasets {
			totalDatasetSize += ds.Size
			totalDatasets++

			if largestDataset == nil || ds.Size > largestDataset.Size {
				largestDataset = ds
				largestDatasetBlock = block
			}
			if smallestDataset == nil || ds.Size < smallestDataset.Size {
				smallestDataset = ds
				smallestDatasetBlock = block
			}
		}
	}

	// Block statistics
	fmt.Fprintf(&sb, "\n\nBlock Statistics:\n")
	fmt.Fprintf(&sb, "  Total size: %s\n", humanize.Bytes(totalBlockSize))
	if len(blocks) > 0 {
		avgBlockSize := totalBlockSize / uint64(len(blocks))
		fmt.Fprintf(&sb, "  Average size: %s\n", humanize.Bytes(avgBlockSize))
	}
	if largestBlock != nil {
		fmt.Fprintf(&sb, "  Largest: %s (%s, shard %d, L%d)\n", humanize.Bytes(largestBlock.Size), largestBlock.Id, largestBlock.Shard, largestBlock.CompactionLevel)
	}
	if smallestBlock != nil {
		fmt.Fprintf(&sb, "  Smallest: %s (%s, shard %d, L%d)", humanize.Bytes(smallestBlock.Size), smallestBlock.Id, smallestBlock.Shard, smallestBlock.CompactionLevel)
	}

	// Dataset statistics
	if totalDatasets > 0 {
		fmt.Fprintf(&sb, "\n\nDataset Statistics:\n")
		fmt.Fprintf(&sb, "  Total datasets: %d\n", totalDatasets)
		fmt.Fprintf(&sb, "  Total size: %s\n", humanize.Bytes(totalDatasetSize))
		avgDatasetSize := totalDatasetSize / uint64(totalDatasets)
		fmt.Fprintf(&sb, "  Average size: %s\n", humanize.Bytes(avgDatasetSize))
		if largestDataset != nil && largestDatasetBlock != nil {
			dsName := getDatasetName(largestDataset, largestDatasetBlock)
			fmt.Fprintf(&sb, "  Largest: %s (%s in %s, shard %d, L%d)\n", humanize.Bytes(largestDataset.Size), dsName, largestDatasetBlock.Id, largestDatasetBlock.Shard, largestDatasetBlock.CompactionLevel)
		}
		if smallestDataset != nil && smallestDatasetBlock != nil {
			dsName := getDatasetName(smallestDataset, smallestDatasetBlock)
			fmt.Fprintf(&sb, "  Smallest: %s (%s in %s, shard %d, L%d)", humanize.Bytes(smallestDataset.Size), dsName, smallestDatasetBlock.Id, smallestDatasetBlock.Shard, smallestDatasetBlock.CompactionLevel)
		}
	}

	return sb.String()
}

func getDatasetName(ds *metastorev1.Dataset, block *metastorev1.BlockMeta) string {
	if ds.Name >= 0 && int(ds.Name) < len(block.StringTable) {
		return block.StringTable[ds.Name]
	}
	return fmt.Sprintf("dataset-%d", ds.Name)
}

func extractBlocksFromPlan(plan *queryv1.QueryPlan) []*metastorev1.BlockMeta {
	if plan == nil || plan.Root == nil {
		return nil
	}
	var blocks []*metastorev1.BlockMeta
	extractBlocksFromNode(plan.Root, &blocks)
	return blocks
}

func extractBlocksFromNode(node *queryv1.QueryNode, blocks *[]*metastorev1.BlockMeta) {
	if node == nil {
		return
	}
	if node.Type == queryv1.QueryNode_READ {
		*blocks = append(*blocks, node.Blocks...)
	}
	for _, child := range node.Children {
		extractBlocksFromNode(child, blocks)
	}
}

func convertQueryPlanToTree(plan *queryv1.QueryPlan) *PlanTreeNode {
	if plan == nil || plan.Root == nil {
		return nil
	}
	return convertQueryNodeToTree(plan.Root)
}

func convertQueryNodeToTree(node *queryv1.QueryNode) *PlanTreeNode {
	if node == nil {
		return nil
	}

	treeNode := &PlanTreeNode{}

	switch node.Type {
	case queryv1.QueryNode_MERGE:
		treeNode.Type = "MERGE"
		treeNode.Children = make([]*PlanTreeNode, 0, len(node.Children))
		for _, child := range node.Children {
			childNode := convertQueryNodeToTree(child)
			if childNode != nil {
				treeNode.Children = append(treeNode.Children, childNode)
				treeNode.TotalBlocks += childNode.TotalBlocks
			}
		}
	case queryv1.QueryNode_READ:
		treeNode.Type = "READ"
		treeNode.BlockCount = len(node.Blocks)
		treeNode.TotalBlocks = len(node.Blocks)
		// Limit blocks shown for display
		maxBlocks := 5
		for i, block := range node.Blocks {
			if i >= maxBlocks {
				break
			}
			treeNode.Blocks = append(treeNode.Blocks, PlanTreeBlock{
				ID:              block.Id,
				Shard:           block.Shard,
				Size:            humanize.Bytes(block.Size),
				CompactionLevel: block.CompactionLevel,
			})
		}
	default:
		treeNode.Type = "UNKNOWN"
	}

	return treeNode
}

func (a *Admin) queryMetadata(
	ctx context.Context,
	tenantID string,
	startTime, endTime int64,
	labelSelector string,
) ([]*metastorev1.BlockMeta, error) {
	query := &metastorev1.QueryMetadataRequest{
		TenantId:  []string{tenantID},
		StartTime: startTime,
		EndTime:   endTime,
		Query:     labelSelector,
	}

	level.Debug(a.logger).Log(
		"msg", "querying metadata",
		"tenant_id", tenantID,
		"start_time", startTime,
		"end_time", endTime,
		"label_selector", labelSelector,
	)

	resp, err := a.metastoreClient.QueryMetadata(ctx, query)
	if err != nil {
		return nil, err
	}

	return resp.Blocks, nil
}

func (a *Admin) handleExecuteQuery(ctx context.Context, content *diagnosticsPageContent) {
	if content.PlanJSON == "" || content.PlanJSON == "{}" {
		content.Error = "query plan is required; create a plan first"
		return
	}
	if content.TenantID == "" {
		content.Error = "tenant ID is required"
		return
	}

	startTime, err := operations.ParseTime(content.StartTime)
	if err != nil {
		content.Error = fmt.Sprintf("invalid start time: %v", err)
		return
	}
	endTime, err := operations.ParseTime(content.EndTime)
	if err != nil {
		content.Error = fmt.Sprintf("invalid end time: %v", err)
		return
	}

	var plan queryv1.QueryPlan
	if err := json.Unmarshal([]byte(content.PlanJSON), &plan); err != nil {
		content.Error = fmt.Sprintf("failed to parse plan JSON: %v", err)
		return
	}

	// Rebuild metadata stats and plan tree from the plan
	blocks := extractBlocksFromPlan(&plan)
	content.MetadataStats = buildMetadataStats(blocks, startTime, endTime)
	content.PlanTree = convertQueryPlanToTree(&plan)

	queryType := a.parseQueryType(content.QueryType)
	query := &queryv1.Query{
		QueryType: queryType,
	}

	switch queryType {
	case queryv1.QueryType_QUERY_PPROF:
		query.Pprof = &queryv1.PprofQuery{}
	case queryv1.QueryType_QUERY_TREE:
		query.Tree = &queryv1.TreeQuery{MaxNodes: 16384}
	case queryv1.QueryType_QUERY_TIME_SERIES:
		query.TimeSeries = &queryv1.TimeSeriesQuery{}
	case queryv1.QueryType_QUERY_LABEL_NAMES:
		query.LabelNames = &queryv1.LabelNamesQuery{}
	case queryv1.QueryType_QUERY_LABEL_VALUES:
		query.LabelValues = &queryv1.LabelValuesQuery{}
	case queryv1.QueryType_QUERY_SERIES_LABELS:
		query.SeriesLabels = &queryv1.SeriesLabelsQuery{}
	}

	req := &queryv1.InvokeRequest{
		Tenant:        []string{content.TenantID},
		StartTime:     startTime.UnixMilli(),
		EndTime:       endTime.UnixMilli(),
		LabelSelector: content.LabelSelector,
		QueryPlan:     &plan,
		Query:         []*queryv1.Query{query},
	}

	level.Debug(a.logger).Log(
		"msg", "executing query",
		"tenant_id", content.TenantID,
		"query_type", content.QueryType,
	)

	start := time.Now()
	resp, err := a.queryBackend.Invoke(ctx, req)
	content.QueryResponseTime = time.Since(start)

	if err != nil {
		content.Error = fmt.Sprintf("query execution failed: %v", err)
		return
	}

	content.ReportStats = a.buildReportStats(resp)
}

func (a *Admin) parseQueryType(queryTypeStr string) queryv1.QueryType {
	switch queryTypeStr {
	case "QUERY_PPROF":
		return queryv1.QueryType_QUERY_PPROF
	case "QUERY_TREE":
		return queryv1.QueryType_QUERY_TREE
	case "QUERY_TIME_SERIES":
		return queryv1.QueryType_QUERY_TIME_SERIES
	case "QUERY_LABEL_NAMES":
		return queryv1.QueryType_QUERY_LABEL_NAMES
	case "QUERY_LABEL_VALUES":
		return queryv1.QueryType_QUERY_LABEL_VALUES
	case "QUERY_SERIES_LABELS":
		return queryv1.QueryType_QUERY_SERIES_LABELS
	default:
		return queryv1.QueryType_QUERY_PPROF
	}
}

func (a *Admin) buildReportStats(resp *queryv1.InvokeResponse) string {
	if resp == nil || len(resp.Reports) == 0 {
		return "No reports returned"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Reports: %d\n", len(resp.Reports))
	for i, report := range resp.Reports {
		fmt.Fprintf(&sb, "\nReport %d:\n", i+1)
		fmt.Fprintf(&sb, "  Type: %s\n", report.ReportType.String())

		switch {
		case report.Pprof != nil && report.Pprof.Pprof != nil:
			fmt.Fprintf(&sb, "  Pprof size: %s\n", humanize.Bytes(uint64(len(report.Pprof.Pprof))))
		case report.Tree != nil && report.Tree.Tree != nil:
			fmt.Fprintf(&sb, "  Tree size: %s\n", humanize.Bytes(uint64(len(report.Tree.Tree))))
		case report.TimeSeries != nil:
			fmt.Fprintf(&sb, "  Time series count: %d\n", len(report.TimeSeries.TimeSeries))
		case report.LabelNames != nil:
			fmt.Fprintf(&sb, "  Label names count: %d\n", len(report.LabelNames.LabelNames))
		case report.LabelValues != nil:
			fmt.Fprintf(&sb, "  Label values count: %d\n", len(report.LabelValues.LabelValues))
		case report.SeriesLabels != nil:
			fmt.Fprintf(&sb, "  Series count: %d\n", len(report.SeriesLabels.SeriesLabels))
		}
	}

	return sb.String()
}
