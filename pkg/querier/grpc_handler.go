package querier

import (
	"net/http"

	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	vcsv1connect "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1/vcsv1connect"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type QuerierSvc interface {
	querierv1connect.QuerierServiceHandler
	vcsv1connect.VCSServiceHandler
}

func NewGRPCHandler(svc QuerierSvc, useK6Middleware bool) connectgrpc.GRPCHandler {
	mux := http.NewServeMux()
	mux.Handle(querierv1connect.NewQuerierServiceHandler(svc, connectapi.DefaultHandlerOptions()...))
	mux.Handle(vcsv1connect.NewVCSServiceHandler(svc, connectapi.DefaultHandlerOptions()...))

	if useK6Middleware {
		httpMiddleware := httputil.K6Middleware()
		return connectgrpc.NewHandler(httpMiddleware.Wrap(mux))
	}

	return connectgrpc.NewHandler(mux)
}
