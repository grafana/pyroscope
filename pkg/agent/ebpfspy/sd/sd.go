package sd

import (
	"context"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
)

type ServiceDiscovery interface {
	// Refresh called every 10s before session reset
	Refresh(ctx context.Context) error

	// GetLabels may return nil
	GetLabels(pid uint32) *spy.Labels
}

type NoopSD struct {
}

func (n NoopSD) Refresh(_ context.Context) error {
	return nil
}

func (n NoopSD) GetLabels(_ uint32) *spy.Labels {
	return nil
}
