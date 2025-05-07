package api

import (
	"net/http"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	segmentwriter "github.com/grafana/pyroscope/pkg/experiment/ingester"
	metastoreadmin "github.com/grafana/pyroscope/pkg/experiment/metastore/admin"
	querybackend "github.com/grafana/pyroscope/pkg/experiment/query_backend"
)

// TODO(kolesnikovae): Recovery interceptor.

func (a *API) RegisterSegmentWriter(svc *segmentwriter.SegmentWriterService) {
	segmentwriterv1.RegisterSegmentWriterServiceServer(a.server.GRPC, svc)
}

// RegisterSegmentWriterRing registers the ring UI page associated with the distributor for writes.
func (a *API) RegisterSegmentWriterRing(r http.Handler) {
	a.RegisterRoute("/ring-segment-writer", r, false, true, "GET", "POST")
	a.indexPage.AddLinks(defaultWeight, "Segment Writer", []IndexPageLink{
		{Desc: "Ring status", Path: "/ring-segment-writer"},
	})
}

func (a *API) RegisterQueryBackend(svc *querybackend.QueryBackend) {
	queryv1.RegisterQueryBackendServiceServer(a.server.GRPC, svc)
}

func (a *API) RegisterMetastoreAdmin(adm *metastoreadmin.Admin) {
	a.RegisterRoute("/metastore-nodes", adm.NodeListHandler(), false, true, "GET", "POST")
	a.RegisterRoute("/metastore-client-test", adm.ClientTestHandler(), false, true, "GET", "POST")
	a.indexPage.AddLinks(defaultWeight, "Metastore", []IndexPageLink{
		{Desc: "Nodes", Path: "/metastore-nodes"},
		{Desc: "Client Test", Path: "/metastore-client-test"},
	})
}
