// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/util/dns_watcher.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package servicediscovery

import (
	"context"
	"fmt"
	"time"

	"github.com/grafana/dskit/grpcutil"
	"github.com/grafana/dskit/services"
	"github.com/pkg/errors"

	util_log "github.com/grafana/phlare/pkg/util"
)

// Instance notified by the service discovery.
type Instance struct {
	Address string

	// InUse is true if this instance should be actively used. For example, if a service discovery
	// implementation enforced a max number of instances to be used, this flag will be set to true
	// only on a number of instances up to the configured max.
	InUse bool
}

func (i Instance) Equal(other Instance) bool {
	return i.Address == other.Address && i.InUse == other.InUse
}

// Notifications about address resolution. All notifications are sent on the same goroutine.
type Notifications interface {
	// InstanceAdded is called each time a new instance has been discovered.
	InstanceAdded(instance Instance)

	// InstanceRemoved is called each time an instance that was previously notified by AddressAdded()
	// is no longer available.
	InstanceRemoved(instance Instance)

	// InstanceChanged is called each time an instance that was previously notified by AddressAdded()
	// has changed its InUse value.
	InstanceChanged(instance Instance)
}

type dnsServiceDiscovery struct {
	watcher  grpcutil.Watcher
	receiver Notifications
}

// NewDNS creates a new DNS-based service discovery.
func NewDNS(address string, dnsLookupPeriod time.Duration, notifications Notifications) (services.Service, error) {
	resolver, err := grpcutil.NewDNSResolverWithFreq(dnsLookupPeriod, util_log.Logger)
	if err != nil {
		return nil, err
	}

	// Pass empty string for service argument, since we don't intend to lookup any SRV record
	watcher, err := resolver.Resolve(address, "")
	if err != nil {
		return nil, err
	}

	w := &dnsServiceDiscovery{
		watcher:  watcher,
		receiver: notifications,
	}
	return services.NewBasicService(nil, w.watchDNSLoop, nil), nil
}

// watchDNSLoop watches for changes in DNS and sends notifications.
func (w *dnsServiceDiscovery) watchDNSLoop(servCtx context.Context) error {
	go func() {
		// Close the watcher, when this service is asked to stop.
		// Closing the watcher makes watchDNSLoop exit, since it only iterates on watcher updates, and has no other
		// way to stop. We cannot close the watcher in `stopping` method, because it is only called *after*
		// watchDNSLoop exits.
		<-servCtx.Done()
		w.watcher.Close()
	}()

	for {
		updates, err := w.watcher.Next()
		if err != nil {
			// watcher.Next returns error when Close is called, but we call Close when our context is done.
			// we don't want to report error in that case.
			if servCtx.Err() != nil {
				return nil
			}
			return errors.Wrapf(err, "error from DNS service discovery")
		}

		for _, update := range updates {
			switch update.Op {
			case grpcutil.Add:
				w.receiver.InstanceAdded(Instance{Address: update.Addr, InUse: true})

			case grpcutil.Delete:
				w.receiver.InstanceRemoved(Instance{Address: update.Addr, InUse: true})

			default:
				return fmt.Errorf("unknown op: %v", update.Op)
			}
		}
	}
}
