package api

import (
	"net/http"

	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/segmentwriter/v1/segmentwriterv1connect"
	segmentwriter "github.com/grafana/pyroscope/pkg/experiment/ingester"
	"github.com/grafana/pyroscope/pkg/experiment/metastore"
	"github.com/grafana/pyroscope/pkg/experiment/querybackend"
)

func (a *API) RegisterSegmentWriter(svc *segmentwriter.SegmentWriterService) {
	// TODO: Remove connect, add gRPC.
	// segmentwriterv1.RegisterSegmentWriterServiceServer(a.server.GRPC, svc)
	segmentwriterv1connect.RegisterSegmentWriterServiceHandler(a.server.HTTP, svc, a.connectOptionsAuthRecovery()...)
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
	compactorv1.RegisterCompactionPlannerServer(a.server.GRPC, svc)
}

func (a *API) RegisterQueryBackend(svc *querybackend.QueryBackend) {
	querybackendv1.RegisterQueryBackendServiceServer(a.server.GRPC, svc)
}
