package querydiagnostics

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/block/metadata"
	"github.com/grafana/pyroscope/pkg/frontend/readpath/queryfrontend/diagnostics"
	"github.com/grafana/pyroscope/pkg/objstore"
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
	bucket           objstore.Bucket
	indexService     metastorev1.IndexServiceClient
}

func New(
	logger log.Logger,
	tenantService metastorev1.TenantServiceClient,
	diagnosticsStore DiagnosticsStore,
	bucket objstore.Bucket,
	indexService metastorev1.IndexServiceClient,
) *Admin {
	adm := &Admin{
		logger:           logger,
		tenantService:    tenantService,
		diagnosticsStore: diagnosticsStore,
		bucket:           bucket,
		indexService:     indexService,
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

// ExportBlocksAPIHandler returns a handler for exporting anonymized L3 blocks
// referenced in a diagnostic's query plan as a tar.gz archive.
func (a *Admin) ExportBlocksAPIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.diagnosticsStore == nil {
			http.Error(w, `{"error":"diagnostics store not configured"}`, http.StatusServiceUnavailable)
			return
		}
		if a.bucket == nil {
			http.Error(w, `{"error":"storage bucket not configured"}`, http.StatusServiceUnavailable)
			return
		}

		tenant := r.URL.Query().Get("tenant")
		if tenant == "" {
			http.Error(w, `{"error":"tenant parameter required"}`, http.StatusBadRequest)
			return
		}

		path := r.URL.Path
		prefix := "/query-diagnostics/api/export-blocks/"
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

		blocks := extractL3Blocks(stored.Plan)
		if len(blocks) == 0 {
			http.Error(w, `{"error":"no L3 blocks found in query plan"}`, http.StatusNotFound)
			return
		}

		a.exportBlocksBuffered(r.Context(), w, blocks, tenant, id)
	})
}

func (a *Admin) exportBlocksBuffered(
	ctx context.Context,
	w http.ResponseWriter,
	blocks []*metastorev1.BlockMeta,
	tenant, id string,
) {
	type result struct {
		data []byte
		err  error
	}

	// Anonymize all blocks in parallel.
	anonymizer := NewBlockAnonymizer(a.bucket)
	results := make([]result, len(blocks))
	var wg sync.WaitGroup
	for i, blockMeta := range blocks {
		wg.Add(1)
		go func(i int, meta *metastorev1.BlockMeta) {
			defer wg.Done()
			data, err := anonymizer.AnonymizeBlock(ctx, meta)
			results[i] = result{data: data, err: err}
		}(i, blockMeta)
	}
	wg.Wait()

	// Write results to tar sequentially.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	var exported int

	for i, r := range results {
		if r.err != nil {
			level.Error(a.logger).Log("msg", "failed to anonymize block", "block_id", blocks[i].Id, "err", r.err)
			continue
		}
		if err := writeTarEntry(tw, blocks[i], r.data); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to write block: %s"}`, err), http.StatusInternalServerError)
			return
		}
		exported++
		level.Info(a.logger).Log("msg", "anonymized block", "block_id", blocks[i].Id, "exported", exported, "total", len(blocks))
	}

	if err := tw.Close(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to finalize tar: %s"}`, err), http.StatusInternalServerError)
		return
	}
	if err := gw.Close(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to finalize gzip: %s"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"blocks-%s-%s.tar.gz\"", tenant, id))
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Write(buf.Bytes())
}

func writeTarEntry(tw *tar.Writer, blockMeta *metastorev1.BlockMeta, data []byte) error {
	blockTenant := metadata.Tenant(blockMeta)
	tarPath := fmt.Sprintf("blocks/%d/%s/%s/%s",
		blockMeta.Shard,
		blockTenant,
		blockMeta.Id,
		block.FileNameDataObject,
	)
	header := &tar.Header{
		Name:    tarPath,
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// ImportBlocksHandler returns an HTTP handler for the import blocks page shell.
func (a *Admin) ImportBlocksHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := pageContent{
			Now: time.Now().UTC(),
		}
		if err := pageTemplates.importBlocksTemplate.Execute(w, content); err != nil {
			httputil.Error(w, err)
		}
	})
}

// ImportBlockAPIHandler returns a handler for importing a single block.
// The block data is sent as the raw request body (application/octet-stream).
func (a *Admin) ImportBlockAPIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if a.bucket == nil {
			http.Error(w, `{"error":"storage bucket not configured"}`, http.StatusServiceUnavailable)
			return
		}
		if a.indexService == nil {
			http.Error(w, `{"error":"index service not configured"}`, http.StatusServiceUnavailable)
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

		blockData, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to read request body: %s"}`, err), http.StatusBadRequest)
			return
		}

		var meta metastorev1.BlockMeta
		if err := metadata.Decode(blockData, &meta); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to decode block metadata: %s"}`, err), http.StatusBadRequest)
			return
		}

		// Rewrite tenant in the metadata string table.
		oldTenant := metadata.Tenant(&meta)
		if meta.Tenant > 0 && int(meta.Tenant) < len(meta.StringTable) {
			meta.StringTable[meta.Tenant] = tenant
		}
		for _, ds := range meta.Datasets {
			if ds.Tenant > 0 && int(ds.Tenant) < len(meta.StringTable) && meta.StringTable[ds.Tenant] == oldTenant {
				meta.StringTable[ds.Tenant] = tenant
			}
		}

		// Re-encode the block: original data up to metadata offset, then new metadata footer.
		var buf bytes.Buffer
		buf.Write(blockData[:meta.MetadataOffset])
		if err := metadata.Encode(&buf, &meta); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to encode block metadata: %s"}`, err), http.StatusInternalServerError)
			return
		}
		newBlockData := buf.Bytes()
		meta.Size = uint64(len(newBlockData))

		objectPath := block.ObjectPath(&meta)
		if err := a.bucket.Upload(r.Context(), objectPath, bytes.NewReader(newBlockData)); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to upload block: %s"}`, err), http.StatusInternalServerError)
			return
		}

		if _, err := a.indexService.AddBlock(r.Context(), &metastorev1.AddBlockRequest{Block: &meta}); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to register block: %s"}`, err), http.StatusInternalServerError)
			return
		}

		level.Info(a.logger).Log("msg", "imported block", "block_id", meta.Id, "tenant", tenant, "path", objectPath, "size", len(newBlockData))

		response := map[string]string{"block_id": meta.Id, "path": objectPath}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			httputil.Error(w, err)
		}
	})
}
