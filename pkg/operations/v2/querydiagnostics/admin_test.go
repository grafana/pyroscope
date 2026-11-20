package querydiagnostics

import (
	"archive/zip"
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/frontend/readpath/queryfrontend/diagnostics"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/filesystem"
)

func importBody(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	for _, name := range []string{"metadata.json", "request.json"} {
		f, err := zw.Create(name)
		require.NoError(t, err)
		_, err = f.Write([]byte(`{}`))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "diagnostics.zip")
	require.NoError(t, err)
	_, err = part.Write(zipBuf.Bytes())
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	return &body, mw.FormDataContentType()
}

func TestTenantParamValidation(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	bucket, err := filesystem.NewBucket(root)
	require.NoError(t, err)
	adm := New(log.NewNopLogger(), nil, diagnostics.NewStore(log.NewNopLogger(), bucket))

	const id = "00000000-0000-0000-0000-000000000000"
	handlers := map[string]struct {
		handler http.Handler
		method  string
		path    string
	}{
		"list":   {adm.DiagnosticsListAPIHandler(), http.MethodGet, "/query-diagnostics/api/diagnostics"},
		"get":    {adm.DiagnosticsGetAPIHandler(), http.MethodGet, "/query-diagnostics/api/diagnostics/" + id},
		"export": {adm.DiagnosticsExportAPIHandler(), http.MethodGet, "/query-diagnostics/api/export/" + id},
		"import": {adm.DiagnosticsImportAPIHandler(), http.MethodPost, "/query-diagnostics/api/import"},
	}

	for name, h := range handlers {
		for _, tenant := range []string{"../../escaped", "..", "a/b", ""} {
			t.Run(name+"/"+tenant, func(t *testing.T) {
				body, contentType := importBody(t)
				req := httptest.NewRequest(h.method, h.path+"?tenant="+url.QueryEscape(tenant), body)
				req.Header.Set("Content-Type", contentType)
				rec := httptest.NewRecorder()
				h.handler.ServeHTTP(rec, req)
				require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			})
		}
	}

	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	for _, e := range entries {
		require.Equal(t, "root", e.Name())
	}
}

func TestImportValidTenant(t *testing.T) {
	bucket, err := filesystem.NewBucket(filepath.Join(t.TempDir(), "root"))
	require.NoError(t, err)
	adm := New(log.NewNopLogger(), nil, diagnostics.NewStore(log.NewNopLogger(), bucket))

	body, contentType := importBody(t)
	req := httptest.NewRequest(http.MethodPost, "/query-diagnostics/api/import?tenant=tenant-1", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	adm.DiagnosticsImportAPIHandler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
