package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/services"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/frontend/readpath/queryfrontend/diagnostics"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

// Available query methods for the dropdown.
var queryMethods = []string{
	"SelectMergeStacktraces",
	"SelectMergeProfile",
	"SelectMergeSpanProfile",
	"SelectSeries",
	"SelectHeatmap",
	"Diff",
	"LabelNames",
	"LabelValues",
	"Series",
	"ProfileTypes",
}

type Admin struct {
	service services.Service
	logger  log.Logger

	tenantService    metastorev1.TenantServiceClient
	queryFrontend    querierv1connect.QuerierServiceClient
	diagnosticsStore *diagnostics.Store
}

func New(
	logger log.Logger,
	tenantService metastorev1.TenantServiceClient,
	queryFrontend querierv1connect.QuerierServiceClient,
	diagnosticsStore *diagnostics.Store,
) *Admin {
	adm := &Admin{
		logger:           logger,
		tenantService:    tenantService,
		queryFrontend:    queryFrontend,
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
			Now:          time.Now().UTC(),
			QueryMethods: queryMethods,
			Method:       "SelectMergeStacktraces",
			StartTime:    "now-1h",
			EndTime:      "now",
		}

		// Fetch available tenants
		a.fetchTenants(r.Context(), &content)

		if loadID := r.URL.Query().Get("load"); loadID != "" && a.diagnosticsStore != nil {
			tenantID := r.URL.Query().Get("tenant")
			if tenantID == "" {
				content.Error = "tenant is required to load a stored diagnostic"
				a.renderDiagnosticsPage(w, content)
				return
			}
			a.loadStoredDiagnostic(r.Context(), tenantID, loadID, &content)
		}

		a.renderDiagnosticsPage(w, content)
	})
}

func (a *Admin) renderDiagnosticsPage(w http.ResponseWriter, content diagnosticsPageContent) {
	if err := pageTemplates.diagnosticsTemplate.Execute(w, content); err != nil {
		httputil.Error(w, err)
	}
}

func (a *Admin) fetchTenants(ctx context.Context, content *diagnosticsPageContent) {
	if a.tenantService == nil {
		return
	}
	resp, err := a.tenantService.GetTenants(ctx, &metastorev1.GetTenantsRequest{})
	if err != nil {
		level.Debug(a.logger).Log("msg", "failed to fetch tenants", "err", err)
		return
	}
	content.Tenants = resp.TenantIds
}

func (a *Admin) loadStoredDiagnostic(ctx context.Context, tenantID string, id string, content *diagnosticsPageContent) {
	stored, err := a.diagnosticsStore.Get(ctx, tenantID, id)
	if err != nil {
		content.Error = fmt.Sprintf("failed to load diagnostic: %v", err)
		return
	}

	content.TenantID = stored.TenantID
	content.DiagnosticsID = stored.ID
	content.Method = stored.Method

	if stored.ResponseTimeMs > 0 {
		content.QueryResponseTime = time.Duration(stored.ResponseTimeMs) * time.Millisecond
	}

	// Deserialize request based on method to populate form fields
	var startTimeMs, endTimeMs int64
	if stored.Request != nil && stored.Method != "" {
		startTimeMs, endTimeMs = a.deserializeRequestToForm(stored.Method, stored.Request, content)
	}

	if stored.Plan != nil {
		planJSON, err := json.MarshalIndent(stored.Plan, "", "  ")
		if err == nil {
			content.PlanJSON = string(planJSON)
		}
		content.PlanTree = convertQueryPlanToTree(stored.Plan)

		// Rebuild metadata stats from the plan
		blocks := extractBlocksFromPlan(stored.Plan)
		if startTimeMs != 0 || endTimeMs != 0 {
			startTime := time.UnixMilli(startTimeMs)
			endTime := time.UnixMilli(endTimeMs)
			content.MetadataStats = buildMetadataStats(blocks, startTime, endTime)
		}
	}

	if stored.Execution != nil {
		content.ExecutionTree = convertExecutionNodeToTree(stored.Execution)
	}
}

// deserializeRequestToForm deserializes the stored request JSON based on method
// and populates the form fields. Returns start/end time in milliseconds.
func (a *Admin) deserializeRequestToForm(method string, requestJSON []byte, content *diagnosticsPageContent) (startTimeMs, endTimeMs int64) {
	switch method {
	case "SelectMergeStacktraces":
		var req querierv1.SelectMergeStacktracesRequest
		if json.Unmarshal(requestJSON, &req) == nil {
			startTimeMs, endTimeMs = req.Start, req.End
			content.StartTime = time.UnixMilli(req.Start).UTC().Format(time.RFC3339)
			content.EndTime = time.UnixMilli(req.End).UTC().Format(time.RFC3339)
			content.LabelSelector = req.LabelSelector
			content.ProfileTypeID = req.ProfileTypeID
			if req.MaxNodes != nil {
				content.MaxNodes = fmt.Sprintf("%d", *req.MaxNodes)
			}
			if req.Format != querierv1.ProfileFormat_PROFILE_FORMAT_UNSPECIFIED {
				content.Format = req.Format.String()
			}
		}
	case "SelectMergeProfile":
		var req querierv1.SelectMergeProfileRequest
		if json.Unmarshal(requestJSON, &req) == nil {
			startTimeMs, endTimeMs = req.Start, req.End
			content.StartTime = time.UnixMilli(req.Start).UTC().Format(time.RFC3339)
			content.EndTime = time.UnixMilli(req.End).UTC().Format(time.RFC3339)
			content.LabelSelector = req.LabelSelector
			content.ProfileTypeID = req.ProfileTypeID
			if req.MaxNodes != nil {
				content.MaxNodes = fmt.Sprintf("%d", *req.MaxNodes)
			}
		}
	case "SelectMergeSpanProfile":
		var req querierv1.SelectMergeSpanProfileRequest
		if json.Unmarshal(requestJSON, &req) == nil {
			startTimeMs, endTimeMs = req.Start, req.End
			content.StartTime = time.UnixMilli(req.Start).UTC().Format(time.RFC3339)
			content.EndTime = time.UnixMilli(req.End).UTC().Format(time.RFC3339)
			content.LabelSelector = req.LabelSelector
			content.ProfileTypeID = req.ProfileTypeID
			if req.MaxNodes != nil {
				content.MaxNodes = fmt.Sprintf("%d", *req.MaxNodes)
			}
			if req.Format != querierv1.ProfileFormat_PROFILE_FORMAT_UNSPECIFIED {
				content.Format = req.Format.String()
			}
			if len(req.SpanSelector) > 0 {
				content.SpanSelector = strings.Join(req.SpanSelector, ", ")
			}
		}
	case "SelectSeries":
		var req querierv1.SelectSeriesRequest
		if json.Unmarshal(requestJSON, &req) == nil {
			startTimeMs, endTimeMs = req.Start, req.End
			content.StartTime = time.UnixMilli(req.Start).UTC().Format(time.RFC3339)
			content.EndTime = time.UnixMilli(req.End).UTC().Format(time.RFC3339)
			content.LabelSelector = req.LabelSelector
			content.ProfileTypeID = req.ProfileTypeID
			if req.Step != 0 {
				content.Step = fmt.Sprintf("%d", int(req.Step))
			}
			if len(req.GroupBy) > 0 {
				content.GroupBy = strings.Join(req.GroupBy, ", ")
			}
			if req.Aggregation != nil {
				content.Aggregation = req.Aggregation.String()
			}
			if req.Limit != nil {
				content.Limit = fmt.Sprintf("%d", *req.Limit)
			}
			if req.ExemplarType != typesv1.ExemplarType_EXEMPLAR_TYPE_UNSPECIFIED && req.ExemplarType != typesv1.ExemplarType_EXEMPLAR_TYPE_NONE {
				content.ExemplarType = req.ExemplarType.String()
			}
		}
	case "SelectHeatmap":
		var req querierv1.SelectHeatmapRequest
		if json.Unmarshal(requestJSON, &req) == nil {
			startTimeMs, endTimeMs = req.Start, req.End
			content.StartTime = time.UnixMilli(req.Start).UTC().Format(time.RFC3339)
			content.EndTime = time.UnixMilli(req.End).UTC().Format(time.RFC3339)
			content.LabelSelector = req.LabelSelector
			content.ProfileTypeID = req.ProfileTypeID
			if req.Step != 0 {
				content.Step = fmt.Sprintf("%d", int(req.Step))
			}
			if len(req.GroupBy) > 0 {
				content.GroupBy = strings.Join(req.GroupBy, ", ")
			}
			if req.Limit != nil {
				content.Limit = fmt.Sprintf("%d", *req.Limit)
			}
			if req.QueryType != querierv1.HeatmapQueryType_HEATMAP_QUERY_TYPE_UNSPECIFIED {
				content.HeatmapQueryType = req.QueryType.String()
			}
			if req.ExemplarType != typesv1.ExemplarType_EXEMPLAR_TYPE_UNSPECIFIED && req.ExemplarType != typesv1.ExemplarType_EXEMPLAR_TYPE_NONE {
				content.ExemplarType = req.ExemplarType.String()
			}
		}
	case "Diff":
		var req querierv1.DiffRequest
		if json.Unmarshal(requestJSON, &req) == nil {
			if req.Left != nil {
				content.DiffLeftSelector = req.Left.LabelSelector
				content.DiffLeftProfileType = req.Left.ProfileTypeID
				content.DiffLeftStart = time.UnixMilli(req.Left.Start).UTC().Format(time.RFC3339)
				content.DiffLeftEnd = time.UnixMilli(req.Left.End).UTC().Format(time.RFC3339)
			}
			if req.Right != nil {
				content.DiffRightSelector = req.Right.LabelSelector
				content.DiffRightProfileType = req.Right.ProfileTypeID
				content.DiffRightStart = time.UnixMilli(req.Right.Start).UTC().Format(time.RFC3339)
				content.DiffRightEnd = time.UnixMilli(req.Right.End).UTC().Format(time.RFC3339)
			}
		}
	case "LabelNames":
		var req typesv1.LabelNamesRequest
		if json.Unmarshal(requestJSON, &req) == nil {
			startTimeMs, endTimeMs = req.Start, req.End
			content.StartTime = time.UnixMilli(req.Start).UTC().Format(time.RFC3339)
			content.EndTime = time.UnixMilli(req.End).UTC().Format(time.RFC3339)
			if len(req.Matchers) > 0 {
				content.LabelSelector = strings.Join(req.Matchers, ", ")
			}
		}
	case "LabelValues":
		var req typesv1.LabelValuesRequest
		if json.Unmarshal(requestJSON, &req) == nil {
			startTimeMs, endTimeMs = req.Start, req.End
			content.StartTime = time.UnixMilli(req.Start).UTC().Format(time.RFC3339)
			content.EndTime = time.UnixMilli(req.End).UTC().Format(time.RFC3339)
			content.LabelName = req.Name
			if len(req.Matchers) > 0 {
				content.LabelSelector = strings.Join(req.Matchers, ", ")
			}
		}
	case "Series":
		var req querierv1.SeriesRequest
		if json.Unmarshal(requestJSON, &req) == nil {
			startTimeMs, endTimeMs = req.Start, req.End
			content.StartTime = time.UnixMilli(req.Start).UTC().Format(time.RFC3339)
			content.EndTime = time.UnixMilli(req.End).UTC().Format(time.RFC3339)
			if len(req.Matchers) > 0 {
				content.LabelSelector = strings.Join(req.Matchers, ", ")
			}
			if len(req.LabelNames) > 0 {
				content.LabelNames = strings.Join(req.LabelNames, ", ")
			}
		}
	case "ProfileTypes":
		var req querierv1.ProfileTypesRequest
		if json.Unmarshal(requestJSON, &req) == nil {
			startTimeMs, endTimeMs = req.Start, req.End
			content.StartTime = time.UnixMilli(req.Start).UTC().Format(time.RFC3339)
			content.EndTime = time.UnixMilli(req.End).UTC().Format(time.RFC3339)
		}
	}
	return startTimeMs, endTimeMs
}
