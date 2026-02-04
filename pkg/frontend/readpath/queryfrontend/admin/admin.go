package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/frontend/readpath/queryfrontend/diagnostics"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type DiagnosticsStore interface {
	ListByTenant(ctx context.Context, tenant string) ([]*diagnostics.DiagnosticSummary, error)
	Get(ctx context.Context, tenant string, id string) (*diagnostics.StoredDiagnostics, error)
	Export(ctx context.Context, tenant string, id string) ([]byte, error)
	Import(ctx context.Context, tenant string, id string, data []byte) (string, error)
}

type Admin struct {
	logger log.Logger

	tenantService    metastorev1.TenantServiceClient
	diagnosticsStore DiagnosticsStore
}

func New(
	logger log.Logger,
	tenantService metastorev1.TenantServiceClient,
	diagnosticsStore DiagnosticsStore,
) *Admin {
	adm := &Admin{
		logger:           logger,
		tenantService:    tenantService,
		diagnosticsStore: diagnosticsStore,
	}
	return adm
}

// TenantsAPIHandler returns a JSON API handler for listing tenants.
func (a *Admin) TenantsAPIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var tenants []string
		if a.tenantService != nil {
			resp, err := a.tenantService.GetTenants(r.Context(), &metastorev1.GetTenantsRequest{})
			if err != nil {
				level.Debug(a.logger).Log("msg", "failed to fetch tenants from tenant service", "err", err)
			} else {
				tenants = resp.TenantIds
			}
		}

		if tenants == nil {
			tenants = []string{}
		}
		if err := json.NewEncoder(w).Encode(tenants); err != nil {
			httputil.Error(w, err)
		}
	})
}

// DiagnosticsListAPIHandler returns a JSON API handler for listing diagnostics by tenant.
func (a *Admin) DiagnosticsListAPIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if a.diagnosticsStore == nil {
			http.Error(w, `{"error":"diagnostics store not configured"}`, http.StatusServiceUnavailable)
			return
		}

		tenant := r.URL.Query().Get("tenant")
		if tenant == "" {
			http.Error(w, `{"error":"tenant parameter required"}`, http.StatusBadRequest)
			return
		}

		diagnosticsList, err := a.diagnosticsStore.ListByTenant(r.Context(), tenant)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to list diagnostics: %s"}`, err), http.StatusInternalServerError)
			return
		}

		if diagnosticsList == nil {
			diagnosticsList = []*diagnostics.DiagnosticSummary{}
		}
		if err := json.NewEncoder(w).Encode(diagnosticsList); err != nil {
			httputil.Error(w, err)
		}
	})
}

// DiagnosticsGetAPIHandler returns a JSON API handler for getting a single diagnostic.
func (a *Admin) DiagnosticsGetAPIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if a.diagnosticsStore == nil {
			http.Error(w, `{"error":"diagnostics store not configured"}`, http.StatusServiceUnavailable)
			return
		}

		tenant := r.URL.Query().Get("tenant")
		if tenant == "" {
			http.Error(w, `{"error":"tenant parameter required"}`, http.StatusBadRequest)
			return
		}

		// Extract ID from path: /query-diagnostics/api/diagnostics/{id}
		path := r.URL.Path
		prefix := "/query-diagnostics/api/diagnostics/"
		if !strings.HasPrefix(path, prefix) {
			http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
			return
		}
		id := strings.TrimPrefix(path, prefix)
		if id == "" {
			http.Error(w, `{"error":"id parameter required"}`, http.StatusBadRequest)
			return
		}

		stored, err := a.diagnosticsStore.Get(r.Context(), tenant, id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, fmt.Sprintf(`{"error":"diagnostic not found: %s"}`, id), http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf(`{"error":"failed to get diagnostic: %s"}`, err), http.StatusInternalServerError)
			return
		}

		if err := json.NewEncoder(w).Encode(stored); err != nil {
			httputil.Error(w, err)
		}
	})
}

// DiagnosticsExportAPIHandler returns a handler for exporting a diagnostic as a zip file.
func (a *Admin) DiagnosticsExportAPIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.diagnosticsStore == nil {
			http.Error(w, `{"error":"diagnostics store not configured"}`, http.StatusServiceUnavailable)
			return
		}

		tenant := r.URL.Query().Get("tenant")
		if tenant == "" {
			http.Error(w, `{"error":"tenant parameter required"}`, http.StatusBadRequest)
			return
		}

		// Extract ID from path: /query-diagnostics/api/export/{id}
		path := r.URL.Path
		prefix := "/query-diagnostics/api/export/"
		if !strings.HasPrefix(path, prefix) {
			http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
			return
		}
		id := strings.TrimPrefix(path, prefix)
		if id == "" {
			http.Error(w, `{"error":"id parameter required"}`, http.StatusBadRequest)
			return
		}

		zipData, err := a.diagnosticsStore.Export(r.Context(), tenant, id)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, fmt.Sprintf(`{"error":"diagnostic not found: %s"}`, id), http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf(`{"error":"failed to export diagnostic: %s"}`, err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"diagnostic-%s-%s.zip\"", tenant, id))
		_, _ = w.Write(zipData)
	})
}

// DiagnosticsImportAPIHandler returns a handler for importing a diagnostic from a zip file.
func (a *Admin) DiagnosticsImportAPIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if a.diagnosticsStore == nil {
			http.Error(w, `{"error":"diagnostics store not configured"}`, http.StatusServiceUnavailable)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		tenant := r.URL.Query().Get("tenant")
		if tenant == "" {
			http.Error(w, `{"error":"tenant parameter required"}`, http.StatusBadRequest)
			return
		}

		// Limit upload size to 100MB
		r.Body = http.MaxBytesReader(w, r.Body, 100<<20)

		// Parse multipart form
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to parse form: %s"}`, err), http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to get file: %s"}`, err), http.StatusBadRequest)
			return
		}
		defer file.Close()

		zipData, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to read file: %s"}`, err), http.StatusInternalServerError)
			return
		}

		newID, err := a.diagnosticsStore.Import(r.Context(), tenant, "", zipData)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to import diagnostic: %s"}`, err), http.StatusInternalServerError)
			return
		}

		level.Info(a.logger).Log("msg", "imported diagnostic", "id", newID, "tenant", tenant)

		response := map[string]string{"id": newID}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			httputil.Error(w, err)
		}
	})
}

// DiagnosticsListHandler returns an HTTP handler for the stored diagnostics page shell.
func (a *Admin) DiagnosticsListHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := pageContent{
			Now: time.Now().UTC(),
		}
		if err := pageTemplates.diagnosticsListTemplate.Execute(w, content); err != nil {
			httputil.Error(w, err)
		}
	})
}

// DiagnosticsHandler returns an HTTP handler for the query diagnostics page shell.
func (a *Admin) DiagnosticsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := pageContent{
			Now: time.Now().UTC(),
		}
		if err := pageTemplates.diagnosticsTemplate.Execute(w, content); err != nil {
			httputil.Error(w, err)
		}
	})
}
