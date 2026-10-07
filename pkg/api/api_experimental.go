package api

import (
	"net/http"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	metastoreadmin "github.com/grafana/pyroscope/v2/pkg/metastore/admin"
	"github.com/grafana/pyroscope/v2/pkg/operations/v2/querydiagnostics"
	"github.com/grafana/pyroscope/v2/pkg/querybackend"
	"github.com/grafana/pyroscope/v2/pkg/segmentwriter"
)

// TODO(kolesnikovae): Recovery interceptor.

func (a *API) RegisterSegmentWriter(svc *segmentwriter.SegmentWriterService) {
	segmentwriterv1.RegisterSegmentWriterServiceServer(a.server.GRPC, svc)
}

// RegisterSegmentWriterRing registers the ring UI page associated with the distributor for writes.
func (a *API) RegisterSegmentWriterRing(r http.Handler) {
	a.registerAdminRoute("/ring-segment-writer", r, a.registerOptionsRingPage()...)
	a.addOperationalLinks(defaultWeight, "Segment Writer", []IndexPageLink{
		{Desc: "Ring status", Path: "/ring-segment-writer"},
	})
}

func (a *API) RegisterQueryBackend(svc *querybackend.QueryBackend) {
	queryv1.RegisterQueryBackendServiceServer(a.server.GRPC, svc)
}

func (a *API) RegisterMetastoreAdmin(adm *metastoreadmin.Admin) {
	a.registerAdminRoute("/metastore-nodes", adm.NodeListHandler(), a.registerOptionsRingPage()...)
	a.registerAdminRoute("/metastore-client-test", adm.ClientTestHandler(), a.registerOptionsRingPage()...)
	a.addOperationalLinks(defaultWeight, "Metastore", []IndexPageLink{
		{Desc: "Nodes", Path: "/metastore-nodes"},
		{Desc: "Client Test", Path: "/metastore-client-test"},
	})
}

func (a *API) RegisterQueryDiagnosticsAdmin(adm *querydiagnostics.Admin) {
	a.registerAdminRoute("/query-diagnostics", adm.DiagnosticsHandler(), a.registerOptionsRingPage()...)
	a.registerAdminRoute("/query-diagnostics/list", adm.DiagnosticsListHandler(), a.registerOptionsRingPage()...)

	// JSON API endpoints for React frontend
	a.registerAdminRoute("/query-diagnostics/api/tenants", adm.TenantsAPIHandler(), a.registerOptionsRingPage()...)
	a.registerAdminRoute("/query-diagnostics/api/diagnostics", adm.DiagnosticsListAPIHandler(), a.registerOptionsRingPage()...)
	a.registerAdminRoute("/query-diagnostics/api/diagnostics/", adm.DiagnosticsGetAPIHandler(), WithGzipMiddleware(), WithMethod("GET"), WithPrefix())
	a.registerAdminRoute("/query-diagnostics/api/export/", adm.DiagnosticsExportAPIHandler(), WithMethod("GET"), WithPrefix())
	a.registerAdminRoute("/query-diagnostics/api/import", adm.DiagnosticsImportAPIHandler(), WithMethod("POST"))

	a.addOperationalLinks(defaultWeight, "Query Diagnostics", []IndexPageLink{
		{Desc: "Collect Diagnostics", Path: "/query-diagnostics"},
		{Desc: "View Stored Diagnostics", Path: "/query-diagnostics/list"},
	})
}
