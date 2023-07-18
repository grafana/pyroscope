// SPDX-License-Identifier: AGPL-3.0-only

package servicediscovery

import (
	"context"
	"sort"
	"time"

	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/pkg/errors"
)

var (
	// Ring operation used to get healthy active instances in the ring.
	activeRingOp = ring.NewOp([]ring.InstanceState{ring.ACTIVE}, nil)
)

type ringServiceDiscovery struct {
	services.Service

	ringClient         *ring.Ring
	ringCheckPeriod    time.Duration
	maxUsedInstances   int
	subservicesWatcher *services.FailureWatcher
	receiver           Notifications

	// Keep track of the instances that have been discovered and notified so far.
	notifiedByAddress map[string]Instance
}

func NewRing(ringClient *ring.Ring, ringCheckPeriod time.Duration, maxUsedInstances int, receiver Notifications) services.Service {
	r := &ringServiceDiscovery{
		ringClient:         ringClient,
		ringCheckPeriod:    ringCheckPeriod,
		maxUsedInstances:   maxUsedInstances,
		subservicesWatcher: services.NewFailureWatcher(),
		notifiedByAddress:  make(map[string]Instance),
		receiver:           receiver,
	}

	r.Service = services.NewBasicService(r.starting, r.running, r.stopping)
	return r
}

func (r *ringServiceDiscovery) starting(ctx context.Context) error {
	r.subservicesWatcher.WatchService(r.ringClient)

	return errors.Wrap(services.StartAndAwaitRunning(ctx, r.ringClient), "failed to start ring client")
}

func (r *ringServiceDiscovery) stopping(_ error) error {
	return errors.Wrap(services.StopAndAwaitTerminated(context.Background(), r.ringClient), "failed to stop ring client")
}

func (r *ringServiceDiscovery) running(ctx context.Context) error {
	ringTicker := time.NewTicker(r.ringCheckPeriod)
	defer ringTicker.Stop()

	// Notifies the initial state.
	all, _ := r.ringClient.GetAllHealthy(activeRingOp) // nolint:errcheck
	r.notifyChanges(all)

	for {
		select {
		case <-ringTicker.C:
			all, _ := r.ringClient.GetAllHealthy(activeRingOp) // nolint:errcheck
			r.notifyChanges(all)
		case <-ctx.Done():
			return nil
		case err := <-r.subservicesWatcher.Chan():
			return errors.Wrap(err, "a subservice of ring-based service discovery has failed")
		}
	}
}

// notifyChanges is not concurrency safe. The input all and inUse ring.ReplicationSet may be the same object.
func (r *ringServiceDiscovery) notifyChanges(all ring.ReplicationSet) {
	// Build a map with the discovered instances.
	instancesByAddress := make(map[string]Instance, len(all.Instances))
	for _, instance := range selectInUseInstances(all.Instances, r.maxUsedInstances) {
		instancesByAddress[instance.Addr] = Instance{Address: instance.Addr, InUse: true}
	}
	for _, instance := range all.Instances {
		if _, ok := instancesByAddress[instance.Addr]; !ok {
			instancesByAddress[instance.Addr] = Instance{Address: instance.Addr, InUse: false}
		}
	}

	// Notify new instances.
	for addr, instance := range instancesByAddress {
		if _, ok := r.notifiedByAddress[addr]; !ok {
			r.receiver.InstanceAdded(instance)
		}
	}

	// Notify changed instances.
	for addr, instance := range instancesByAddress {
		if n, ok := r.notifiedByAddress[addr]; ok && !n.Equal(instance) {
			r.receiver.InstanceChanged(instance)
		}
	}

	// Notify removed instances.
	for addr, instance := range r.notifiedByAddress {
		if _, ok := instancesByAddress[addr]; !ok {
			r.receiver.InstanceRemoved(instance)
		}
	}

	r.notifiedByAddress = instancesByAddress
}

func selectInUseInstances(instances []ring.InstanceDesc, maxInstances int) []ring.InstanceDesc {
	if maxInstances <= 0 || len(instances) <= maxInstances {
		return instances
	}

	// Select the first N instances (sorted by address) to be used.
	sort.Sort(ring.ByAddr(instances))
	return instances[:maxInstances]
}
