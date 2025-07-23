package api

import (
	"connectrpc.com/connect"

	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/delayhandler"
)

func connectInterceptorRecovery() connect.HandlerOption {
	return connect.WithInterceptors(util.RecoveryInterceptor)
}

func (a *API) connectInterceptorAuth() connect.HandlerOption {
	return a.cfg.GrpcAuthMiddleware
}

func (a *API) connectInterceptorLog() connect.HandlerOption {
	return connect.WithInterceptors(util.NewLogInterceptor(a.logger))
}

func (a *API) connectInterceptorDelay(limits delayhandler.Limits) connect.HandlerOption {
	return connect.WithInterceptors(delayhandler.NewConnect(limits))
}

func (a *API) connectOptionsRecovery() []connect.HandlerOption {
	return append(connectapi.DefaultHandlerOptions(), connectInterceptorRecovery())
}

func (a *API) connectOptionsAuthRecovery() []connect.HandlerOption {
	return append(connectapi.DefaultHandlerOptions(), a.connectInterceptorAuth(), connectInterceptorRecovery())
}

func (a *API) connectOptionsAuthDelayRecovery(limits delayhandler.Limits) []connect.HandlerOption {
	return append(connectapi.DefaultHandlerOptions(), a.connectInterceptorAuth(), a.connectInterceptorDelay(limits), connectInterceptorRecovery())
}

func (a *API) connectOptionsAuthLogRecovery() []connect.HandlerOption {
	return append(connectapi.DefaultHandlerOptions(), a.connectInterceptorAuth(), a.connectInterceptorLog(), connectInterceptorRecovery())
}
