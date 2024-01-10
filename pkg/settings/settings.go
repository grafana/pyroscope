package settings

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/tenant"
	"github.com/pkg/errors"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

func New(store Store) (*TenantSettings, error) {
	ts := &TenantSettings{
		store: store,
	}

	ts.Service = services.NewBasicService(ts.starting, ts.running, ts.stopping)

	return ts, nil
}

type TenantSettings struct {
	services.Service

	store Store
}

func (ts *TenantSettings) starting(ctx context.Context) error {
	return nil
}

func (ts *TenantSettings) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (ts *TenantSettings) stopping(_ error) error {
	return nil
}

func (ts *TenantSettings) Get(ctx context.Context, req *connect.Request[settingsv1.GetSettingsRequest]) (*connect.Response[settingsv1.GetSettingsResponse], error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	settings, err := ts.store.Get(ctx, tenantID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&settingsv1.GetSettingsResponse{
		Settings: settings,
	}), nil
}

func (ts *TenantSettings) Set(ctx context.Context, req *connect.Request[settingsv1.SetSettingsRequest]) (*connect.Response[settingsv1.SetSettingsResponse], error) {
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	setting, err := ts.store.Set(ctx, tenantID, req.Msg.Setting)
	if err != nil {
		if errors.Is(err, oldSettingErr) {
			return nil, connect.NewError(connect.CodeAlreadyExists, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&settingsv1.SetSettingsResponse{
		Setting: setting,
	}), nil
}
