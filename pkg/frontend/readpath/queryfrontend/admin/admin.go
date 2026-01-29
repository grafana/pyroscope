package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/frontend/readpath/queryfrontend/diagnostics"
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

	metastoreClient  MetastoreClient
	queryBackend     QueryBackend
	diagnosticsStore *diagnostics.Store
}

func New(
	logger log.Logger,
	metastoreClient MetastoreClient,
	queryBackend QueryBackend,
	diagnosticsStore *diagnostics.Store,
) *Admin {
	adm := &Admin{
		logger:           logger,
		metastoreClient:  metastoreClient,
		queryBackend:     queryBackend,
		diagnosticsStore: diagnosticsStore,
	}
	adm.service = services.NewIdleService(adm.starting, adm.stopping)
	return adm
}

func (a *Admin) Service() services.Service {
	return a.service
}

func (a *Admin) starting(context.Context) error { return nil }
func (a *Admin) stopping(error) error           { return nil }

// DiagnosticsStore returns the diagnostics store for API registration.
func (a *Admin) DiagnosticsStore() *diagnostics.Store {
	return a.diagnosticsStore
}

// DiagnosticsListHandler returns an HTTP handler for listing stored diagnostics.
func (a *Admin) DiagnosticsListHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := diagnosticsListPageContent{
			Now: time.Now().UTC(),
		}

		if a.diagnosticsStore == nil {
			content.Error = "diagnostics store is not configured"
			a.renderDiagnosticsListPage(w, content)
			return
		}

		tenants, err := a.diagnosticsStore.ListTenants(r.Context())
		if err != nil {
			content.Error = fmt.Sprintf("failed to list tenants: %v", err)
			a.renderDiagnosticsListPage(w, content)
			return
		}
		content.Tenants = tenants

		selectedTenant := r.URL.Query().Get("tenant")
		if selectedTenant != "" {
			content.SelectedTenant = selectedTenant
			diagnosticsList, err := a.diagnosticsStore.ListByTenant(r.Context(), selectedTenant)
			if err != nil {
				content.Error = fmt.Sprintf("failed to list diagnostics: %v", err)
			} else {
				content.Diagnostics = diagnosticsList
			}
		}

		a.renderDiagnosticsListPage(w, content)
	})
}

func (a *Admin) renderDiagnosticsListPage(w http.ResponseWriter, content diagnosticsListPageContent) {
	if err := pageTemplates.diagnosticsListTemplate.Execute(w, content); err != nil {
		httputil.Error(w, err)
	}
}

func (a *Admin) DiagnosticsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := diagnosticsPageContent{
			Now:       time.Now().UTC(),
			QueryType: "QUERY_PPROF",
			StartTime: "now-30m",
			EndTime:   "now",
		}

		if tenants, err := a.fetchTenants(r.Context()); err == nil {
			content.Tenants = tenants
		}

		if loadID := r.URL.Query().Get("load"); loadID != "" && a.diagnosticsStore != nil {
			tenant := r.URL.Query().Get("tenant")
			if tenant == "" {
				content.Error = "tenant is required to load a stored diagnostic"
				a.renderDiagnosticsPage(w, content)
				return
			}
			a.loadStoredDiagnostic(r.Context(), tenant, loadID, &content)
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

			content.MaxNodes = r.FormValue("max_nodes")
			content.LabelName = r.FormValue("label_name")
			content.SeriesLabelNames = r.FormValue("series_label_names")
			content.Step = r.FormValue("step")
			content.GroupBy = r.FormValue("group_by")
			content.Limit = r.FormValue("limit")

			action := r.FormValue("action")
			switch action {
			case "create_plan":
				a.handleCreatePlan(r.Context(), &content)
			case "execute_query":
				a.handleExecuteQuery(r.Context(), &content)
			case "create_and_execute":
				a.handleCreatePlan(r.Context(), &content)
				if content.Error == "" && content.PlanJSON != "" {
					a.handleExecuteQuery(r.Context(), &content)
				}
			}

			// Redirect to load URL after successful query execution (POST-Redirect-GET pattern)
			if content.DiagnosticsID != "" && content.Error == "" {
				redirectURL := fmt.Sprintf("%s?load=%s&tenant=%s", r.URL.Path, content.DiagnosticsID, content.TenantID)
				http.Redirect(w, r, redirectURL, http.StatusSeeOther)
				return
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

func (a *Admin) loadStoredDiagnostic(ctx context.Context, tenant string, id string, content *diagnosticsPageContent) {
	stored, err := a.diagnosticsStore.Get(ctx, tenant, id)
	if err != nil {
		content.Error = fmt.Sprintf("failed to load diagnostic: %v", err)
		return
	}

	content.TenantID = stored.TenantID
	content.DiagnosticsID = stored.ID

	if stored.ResponseTimeMs > 0 {
		content.QueryResponseTime = time.Duration(stored.ResponseTimeMs) * time.Millisecond
	}

	if stored.Request != nil {
		content.StartTime = time.UnixMilli(stored.Request.StartTime).UTC().Format(time.RFC3339)
		content.EndTime = time.UnixMilli(stored.Request.EndTime).UTC().Format(time.RFC3339)
		content.LabelSelector = stored.Request.LabelSelector

		if len(stored.Request.Query) > 0 {
			q := stored.Request.Query[0]
			content.QueryType = q.QueryType.String()

			switch q.QueryType {
			case queryv1.QueryType_QUERY_TREE:
				if q.Tree != nil && q.Tree.MaxNodes > 0 {
					content.MaxNodes = fmt.Sprintf("%d", q.Tree.MaxNodes)
				}
			case queryv1.QueryType_QUERY_PPROF:
				if q.Pprof != nil && q.Pprof.MaxNodes > 0 {
					content.MaxNodes = fmt.Sprintf("%d", q.Pprof.MaxNodes)
				}
			case queryv1.QueryType_QUERY_TIME_SERIES:
				if q.TimeSeries != nil {
					if q.TimeSeries.Step > 0 {
						content.Step = fmt.Sprintf("%g", q.TimeSeries.Step)
					}
					if len(q.TimeSeries.GroupBy) > 0 {
						content.GroupBy = strings.Join(q.TimeSeries.GroupBy, ", ")
					}
					if q.TimeSeries.Limit > 0 {
						content.Limit = fmt.Sprintf("%d", q.TimeSeries.Limit)
					}
				}
			case queryv1.QueryType_QUERY_HEATMAP:
				if q.Heatmap != nil {
					if q.Heatmap.Step > 0 {
						content.Step = fmt.Sprintf("%g", q.Heatmap.Step)
					}
					if len(q.Heatmap.GroupBy) > 0 {
						content.GroupBy = strings.Join(q.Heatmap.GroupBy, ", ")
					}
					if q.Heatmap.GetLimit() > 0 {
						content.Limit = fmt.Sprintf("%d", q.Heatmap.GetLimit())
					}
				}
			case queryv1.QueryType_QUERY_LABEL_VALUES:
				if q.LabelValues != nil {
					content.LabelName = q.LabelValues.LabelName
				}
			case queryv1.QueryType_QUERY_SERIES_LABELS:
				if q.SeriesLabels != nil && len(q.SeriesLabels.LabelNames) > 0 {
					content.SeriesLabelNames = strings.Join(q.SeriesLabels.LabelNames, ", ")
				}
			}
		}
	}

	if stored.Plan != nil {
		planJSON, err := json.MarshalIndent(stored.Plan, "", "  ")
		if err == nil {
			content.PlanJSON = string(planJSON)
		}
		content.PlanTree = convertQueryPlanToTree(stored.Plan)

		// Rebuild metadata stats from the plan
		blocks := extractBlocksFromPlan(stored.Plan)
		if stored.Request != nil {
			startTime := time.UnixMilli(stored.Request.StartTime)
			endTime := time.UnixMilli(stored.Request.EndTime)
			content.MetadataStats = buildMetadataStats(blocks, startTime, endTime)
		}
	}

	if stored.Execution != nil {
		content.ExecutionTree = convertExecutionNodeToTree(stored.Execution)
	}
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
		for _, block := range node.Blocks {
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

	// Parse common parameters
	maxNodes := parseIntOrDefault(content.MaxNodes, 16384)
	limit := parseIntOrDefault(content.Limit, 0)

	switch queryType {
	case queryv1.QueryType_QUERY_PPROF:
		query.Pprof = &queryv1.PprofQuery{
			MaxNodes: maxNodes,
		}
	case queryv1.QueryType_QUERY_TREE:
		query.Tree = &queryv1.TreeQuery{
			MaxNodes: maxNodes,
		}
	case queryv1.QueryType_QUERY_TIME_SERIES:
		// Parse step or calculate a sensible default (~100 data points)
		stepSeconds := parseFloatOrDefault(content.Step, 0)
		if stepSeconds <= 0 {
			rangeMs := endTime.UnixMilli() - startTime.UnixMilli()
			stepSeconds = float64(rangeMs) / 1000.0 / 100.0
			if stepSeconds < 15 {
				stepSeconds = 15 // minimum 15 second step
			}
		}

		var groupBy []string
		if content.GroupBy != "" {
			groupBy = splitAndTrim(content.GroupBy)
		}

		query.TimeSeries = &queryv1.TimeSeriesQuery{
			Step:    stepSeconds,
			GroupBy: groupBy,
			Limit:   limit,
		}
	case queryv1.QueryType_QUERY_HEATMAP:
		stepSeconds := parseFloatOrDefault(content.Step, 0)
		if stepSeconds <= 0 {
			rangeMs := endTime.UnixMilli() - startTime.UnixMilli()
			stepSeconds = float64(rangeMs) / 1000.0 / 100.0
			if stepSeconds < 15 {
				stepSeconds = 15
			}
		}

		var groupBy []string
		if content.GroupBy != "" {
			groupBy = splitAndTrim(content.GroupBy)
		}

		query.Heatmap = &queryv1.HeatmapQuery{
			Step:      stepSeconds,
			GroupBy:   groupBy,
			Limit:     limit,
			QueryType: querierv1.HeatmapQueryType_HEATMAP_QUERY_TYPE_INDIVIDUAL,
		}
	case queryv1.QueryType_QUERY_LABEL_NAMES:
		query.LabelNames = &queryv1.LabelNamesQuery{}
	case queryv1.QueryType_QUERY_LABEL_VALUES:
		query.LabelValues = &queryv1.LabelValuesQuery{
			LabelName: content.LabelName,
		}
	case queryv1.QueryType_QUERY_SERIES_LABELS:
		var labelNames []string
		if content.SeriesLabelNames != "" {
			labelNames = splitAndTrim(content.SeriesLabelNames)
		}
		query.SeriesLabels = &queryv1.SeriesLabelsQuery{
			LabelNames: labelNames,
		}
	}

	req := &queryv1.InvokeRequest{
		Tenant:        []string{content.TenantID},
		StartTime:     startTime.UnixMilli(),
		EndTime:       endTime.UnixMilli(),
		LabelSelector: content.LabelSelector,
		QueryPlan:     &plan,
		Query:         []*queryv1.Query{query},
		Options: &queryv1.InvokeOptions{
			CollectDiagnostics: true,
		},
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

	// Build QueryRequest from the form data for storage
	request := &queryv1.QueryRequest{
		StartTime:     startTime.UnixMilli(),
		EndTime:       endTime.UnixMilli(),
		LabelSelector: content.LabelSelector,
		Query:         []*queryv1.Query{query},
	}

	var execution *queryv1.ExecutionNode
	if resp.Diagnostics != nil && resp.Diagnostics.ExecutionNode != nil {
		execution = resp.Diagnostics.ExecutionNode
		content.ExecutionTree = convertExecutionNodeToTree(execution)
	}

	id, err := a.diagnosticsStore.SaveDirect(ctx, content.TenantID, content.QueryResponseTime.Milliseconds(), request, &plan, execution)
	if err != nil {
		level.Warn(a.logger).Log("msg", "failed to save diagnostics", "err", err)
	} else {
		content.DiagnosticsID = id
	}

}

func (a *Admin) parseQueryType(queryTypeStr string) queryv1.QueryType {
	switch queryTypeStr {
	case "QUERY_PPROF":
		return queryv1.QueryType_QUERY_PPROF
	case "QUERY_TREE":
		return queryv1.QueryType_QUERY_TREE
	case "QUERY_TIME_SERIES":
		return queryv1.QueryType_QUERY_TIME_SERIES
	case "QUERY_HEATMAP":
		return queryv1.QueryType_QUERY_HEATMAP
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

func convertExecutionNodeToTree(node *queryv1.ExecutionNode) *ExecutionTreeNode {
	if node == nil {
		return nil
	}

	// Find the earliest start time across all nodes to use as query start reference
	queryStartNs := findEarliestStartTime(node)

	return convertExecutionNodeToTreeWithBase(node, queryStartNs)
}

func findEarliestStartTime(node *queryv1.ExecutionNode) int64 {
	if node == nil {
		return 0
	}

	earliest := node.StartTimeNs

	// Check block executions
	if node.Stats != nil {
		for _, blockExec := range node.Stats.BlockExecutions {
			if blockExec.StartTimeNs < earliest {
				earliest = blockExec.StartTimeNs
			}
		}
	}

	// Check children
	for _, child := range node.Children {
		childEarliest := findEarliestStartTime(child)
		if childEarliest > 0 && childEarliest < earliest {
			earliest = childEarliest
		}
	}

	return earliest
}

func convertExecutionNodeToTreeWithBase(node *queryv1.ExecutionNode, queryStartNs int64) *ExecutionTreeNode {
	if node == nil {
		return nil
	}

	duration := time.Duration(node.EndTimeNs - node.StartTimeNs)
	relativeStart := time.Duration(node.StartTimeNs - queryStartNs)

	tree := &ExecutionTreeNode{
		Type:             node.Type.String(),
		Executor:         node.Executor,
		Duration:         duration,
		DurationStr:      formatDurationShort(duration),
		RelativeStart:    relativeStart,
		RelativeStartStr: formatDurationShort(relativeStart),
		Error:            node.Error,
	}

	if node.Stats != nil {
		tree.Stats = &ExecutionTreeStats{
			BlocksRead:        node.Stats.BlocksRead,
			DatasetsProcessed: node.Stats.DatasetsProcessed,
		}

		for _, blockExec := range node.Stats.BlockExecutions {
			blockDuration := time.Duration(blockExec.EndTimeNs - blockExec.StartTimeNs)
			blockRelStart := time.Duration(blockExec.StartTimeNs - queryStartNs)
			blockRelEnd := time.Duration(blockExec.EndTimeNs - queryStartNs)

			tree.Stats.BlockExecutions = append(tree.Stats.BlockExecutions, &BlockExecutionInfo{
				BlockID:           blockExec.BlockId,
				Duration:          blockDuration,
				DurationStr:       formatDurationShort(blockDuration),
				RelativeStart:     blockRelStart,
				RelativeStartStr:  formatDurationShort(blockRelStart),
				RelativeEnd:       blockRelEnd,
				RelativeEndStr:    formatDurationShort(blockRelEnd),
				DatasetsProcessed: blockExec.DatasetsProcessed,
				Size:              humanize.Bytes(blockExec.Size),
				Shard:             blockExec.Shard,
				CompactionLevel:   blockExec.CompactionLevel,
			})
		}
	}

	for _, child := range node.Children {
		if childTree := convertExecutionNodeToTreeWithBase(child, queryStartNs); childTree != nil {
			tree.Children = append(tree.Children, childTree)
		}
	}

	return tree
}

// Helper functions for parsing form values

func parseIntOrDefault(s string, defaultVal int64) int64 {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return defaultVal
	}
	return v
}

func parseFloatOrDefault(s string, defaultVal float64) float64 {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return defaultVal
	}
	return v
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
