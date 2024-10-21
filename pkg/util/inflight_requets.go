package util

import (
	"sync"
)

// InflightRequests utility emerged due to the need to handle request draining
// at the service level.
//
// Ideally, this should be the responsibility of the server using the service.
// However, since the server is a dependency of the service and is only shut
// down after the service is stopped, requests may still arrive after the Stop
// call. This issue arises from how we initialize modules.
//
// In other scenarios, request draining could be managed at a higher level,
// such as in a load balancer or service discovery mechanism. The goal would
// be to stop routing requests to an instance that is about to shut down.
//
// In our case, service instances that are not directly exposed to the outside
// world but are discoverable via e.g, ring, kubernetes, or DNS. There's no a
// _reliable_ mechanism to ensure that all the clients are aware of fact that
// the instance is leaving, so requests may continue to arrive within a short
// period of time. InflightRequests ensure that such requests will be rejected.
type InflightRequests struct {
	mu      sync.RWMutex
	wg      sync.WaitGroup
	allowed bool
}

// Open allows new requests.
func (r *InflightRequests) Open() {
	r.mu.Lock()
	r.allowed = true
	r.mu.Unlock()
}

// Add adds a new request if allowed.
func (r *InflightRequests) Add() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.allowed {
		return false
	}
	r.wg.Add(1)
	return true
}

// Done completes a request.
func (r *InflightRequests) Done() {
	r.wg.Done()
}

// Drain prevents new requests from being accepted
// and waits for all ongoing requests to complete.
func (r *InflightRequests) Drain() {
	r.mu.Lock()
	r.allowed = false
	r.mu.Unlock()
	r.wg.Wait()
}
