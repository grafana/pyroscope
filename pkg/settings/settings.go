package settings

import (
	"context"

	"github.com/grafana/dskit/services"
)

func New() (*TenantSettings, error) {
	ts := &TenantSettings{}

	ts.Service = services.NewBasicService(ts.starting, ts.running, ts.stopping)

	return ts, nil
}

type TenantSettings struct {
	services.Service
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
