package settings

import (
	"context"

	connect "github.com/bufbuild/connect-go"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/settings/v1/settingsv1connect"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

func New(store Store) (settingsv1connect.SettingsServiceClient, error) {
	svc := &settingsService{
		store: store,
	}
	return svc, nil
}

type settingsService struct {
	store Store
}

func (s *settingsService) All(ctx context.Context, req *connect.Request[settingsv1.AllSettingsRequest]) (*connect.Response[settingsv1.AllSettingsResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Settings All")
	defer sp.Finish()

	ctx = connectgrpc.WithProcedure(ctx, settingsv1connect.SettingsServiceAllProcedure)

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	settings, err := s.store.All(ctx, tenantIDs...)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&settingsv1.AllSettingsResponse{
		Settings: settings,
	}), nil
}

func (s *settingsService) Set(ctx context.Context, req *connect.Request[settingsv1.SetSettingsRequest]) (*connect.Response[settingsv1.SetSettingsResponse], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "Settings Set")
	defer sp.Finish()

	ctx = connectgrpc.WithProcedure(ctx, settingsv1connect.SettingsServiceSetProcedure)

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	setting, err := s.store.Set(ctx, req.Msg.Setting, tenantIDs...)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&settingsv1.SetSettingsResponse{
		Setting: setting,
	}), nil
}
