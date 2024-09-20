package api

import (
	"net/http"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/metastorev1connect"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	segmentwriterv1 "github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1"
	segmentwriter "github.com/grafana/pyroscope/pkg/experiment/ingester"
	"github.com/grafana/pyroscope/pkg/experiment/metastore"
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

func (a *API) RegisterMetastore(svc *metastore.Metastore) {
	metastorev1.RegisterMetastoreServiceServer(a.server.GRPC, svc)
	metastorev1connect.RegisterOperatorServiceHandler(a.server.HTTP, svc)
	compactorv1.RegisterCompactionPlannerServer(a.server.GRPC, svc)
}

func (a *API) RegisterQueryBackend(svc *querybackend.QueryBackend) {
	queryv1.RegisterQueryBackendServiceServer(a.server.GRPC, svc)
}
