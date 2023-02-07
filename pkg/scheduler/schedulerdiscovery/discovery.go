// SPDX-License-Identifier: AGPL-3.0-only

package schedulerdiscovery

import (
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/phlare/pkg/util/servicediscovery"
)

func New(cfg Config, schedulerAddress string, lookupPeriod time.Duration, component string, receiver servicediscovery.Notifications, logger log.Logger, reg prometheus.Registerer) (services.Service, error) {
	// Since this is a client for the query-schedulers ring, we append "query-scheduler-client" to the component to clearly differentiate it.
	component = component + "-query-scheduler-client"

	switch cfg.Mode {
	case ModeRing:
		return newRing(cfg, component, receiver, logger, reg)
	default:
		return servicediscovery.NewDNS(schedulerAddress, lookupPeriod, receiver)
	}
}

func newRing(cfg Config, component string, receiver servicediscovery.Notifications, logger log.Logger, reg prometheus.Registerer) (services.Service, error) {
	client, err := NewRingClient(cfg.SchedulerRing, component, logger, reg)
	if err != nil {
		return nil, err
	}

	const ringCheckPeriod = 5 * time.Second
	return servicediscovery.NewRing(client, ringCheckPeriod, cfg.MaxUsedInstances, receiver), nil
}
