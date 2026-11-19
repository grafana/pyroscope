package api

import (
	"context"

	"connectrpc.com/connect"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	"github.com/grafana/pyroscope/v2/pkg/profiledump"
	"github.com/grafana/pyroscope/v2/pkg/tenant"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

// registerPusher wraps only the external service. Legacy ingestion retains its
// direct distributor reference, so converted profiles cannot enter this hook.
func (a *API) registerPusher(svc pushv1connect.PusherServiceHandler, limits *validation.Overrides, recorder *profiledump.Recorder) {
	if recorder != nil {
		svc = &capturePusher{next: svc, recorder: recorder}
	}
	pushv1connect.RegisterPusherServiceHandler(a.server.HTTP, svc, a.connectOptionsAuthDelayRecovery(limits)...)
}

type capturePusher struct {
	next     pushv1connect.PusherServiceHandler
	recorder *profiledump.Recorder
}

func (p *capturePusher) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	if tenantID, err := tenant.ExtractTenantIDFromContext(ctx); err == nil {
		for _, series := range req.Msg.Series {
			p.recorder.CaptureSeries(ctx, tenantID, series)
		}
	}
	// Capture synchronously owns accepted bytes before downstream can reuse them.
	return p.next.Push(ctx, req)
}
