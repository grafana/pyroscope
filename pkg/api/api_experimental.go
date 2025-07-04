package api

import (
	"net/http"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	metastoreadmin "github.com/grafana/pyroscope/pkg/metastore/admin"
	"github.com/grafana/pyroscope/pkg/querybackend"
	"github.com/grafana/pyroscope/pkg/segmentwriter"
)

// TODO(kolesnikovae): Recovery interceptor.

func (a *API) RegisterSegmentWriter(svc *segmentwriter.SegmentWriterService) {
	segmentwriterv1.RegisterSegmentWriterServiceServer(a.server.GRPC, svc)
}

// RegisterSegmentWriterRing registers the ring UI page associated with the distributor for writes.
func (a *API) RegisterSegmentWriterRing(r http.Handler) {
	a.RegisterRoute("/ring-segment-writer", r, a.registerOptionsRingPage()...)
	a.indexPage.AddLinks(defaultWeight, "Segment Writer", []IndexPageLink{
		{Desc: "Ring status", Path: "/ring-segment-writer"},
	})
}

func (a *API) RegisterQueryBackend(svc *querybackend.QueryBackend) {
	queryv1.RegisterQueryBackendServiceServer(a.server.GRPC, svc)
}

func (a *API) RegisterMetastoreAdmin(adm *metastoreadmin.Admin) {
	a.RegisterRoute("/metastore-nodes", adm.NodeListHandler(), a.registerOptionsRingPage()...)
	a.RegisterRoute("/metastore-client-test", adm.ClientTestHandler(), a.registerOptionsRingPage()...)
	a.indexPage.AddLinks(defaultWeight, "Metastore", []IndexPageLink{
		{Desc: "Nodes", Path: "/metastore-nodes"},
		{Desc: "Client Test", Path: "/metastore-client-test"},
	})
}
