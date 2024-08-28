package querier

import (
	"net/http"

	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	vcsv1connect "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1/vcsv1connect"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

type QuerierSvc interface {
	querierv1connect.QuerierServiceHandler
	vcsv1connect.VCSServiceHandler
}

func NewGRPCHandler(svc QuerierSvc) connectgrpc.GRPCHandler {
	mux := http.NewServeMux()
	mux.Handle(querierv1connect.NewQuerierServiceHandler(svc, connectapi.DefaultHandlerOptions()...))
	mux.Handle(vcsv1connect.NewVCSServiceHandler(svc, connectapi.DefaultHandlerOptions()...))
	return connectgrpc.NewHandler(mux)
}
