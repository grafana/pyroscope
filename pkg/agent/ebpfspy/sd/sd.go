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

type NoopServiceDiscovery struct {
}

func (NoopServiceDiscovery) Refresh(_ context.Context) error {
	return nil
}

func (NoopServiceDiscovery) GetLabels(_ uint32) *spy.Labels {
	return nil
}
