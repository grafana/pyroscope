// SPDX-License-Identifier: AGPL-3.0-only

package servicediscovery

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/kv/consul"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRingServiceDiscovery_WithoutMaxUsedInstances(t *testing.T) {
	const (
		ringKey         = "test"
		ringCheckPeriod = 100 * time.Millisecond // Check very frequently to speed up the test.
	)

	// Use an in-memory KV store.
	inmem, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { _ = closer.Close() })

	// Create a ring client.
	ringCfg := ring.Config{HeartbeatTimeout: time.Minute, ReplicationFactor: 1}
	ringClient, err := ring.NewWithStoreClientAndStrategy(ringCfg, "test", ringKey, inmem, ring.NewDefaultReplicationStrategy(), nil, log.NewNopLogger())
	require.NoError(t, err)

	// Create an empty ring.
	ctx := context.Background()
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		return ring.NewDesc(), true, nil
	}))

	// Mock a receiver to keep track of all notified addresses.
	receiver := newNotificationsReceiverMock()

	sd := NewRing(ringClient, ringCheckPeriod, 0, receiver)

	// Start the service discovery.
	require.NoError(t, services.StartAndAwaitRunning(ctx, sd))
	t.Cleanup(func() {
		require.NoError(t, services.StopAndAwaitTerminated(ctx, sd))
	})

	// Wait some time, we expect no address notified because the ring is empty.
	time.Sleep(time.Second)
	require.Empty(t, receiver.getDiscoveredInstances())

	// Register some instances.
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		desc.AddIngester("instance-1", "127.0.0.1", "", nil, ring.ACTIVE, time.Now())
		desc.AddIngester("instance-2", "127.0.0.2", "", nil, ring.PENDING, time.Now())
		desc.AddIngester("instance-3", "127.0.0.3", "", nil, ring.JOINING, time.Now())
		desc.AddIngester("instance-4", "127.0.0.4", "", nil, ring.LEAVING, time.Now())
		return desc, true, nil
	}))

	test.Poll(t, time.Second, []Instance{{"127.0.0.1", true}}, func() interface{} {
		return receiver.getDiscoveredInstances()
	})

	// Register more instances.
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		desc.AddIngester("instance-5", "127.0.0.5", "", nil, ring.ACTIVE, time.Now())
		desc.AddIngester("instance-6", "127.0.0.6", "", nil, ring.ACTIVE, time.Now())
		return desc, true, nil
	}))

	test.Poll(t, time.Second, []Instance{{"127.0.0.1", true}, {"127.0.0.5", true}, {"127.0.0.6", true}}, func() interface{} {
		return receiver.getDiscoveredInstances()
	})

	// Unregister some instances.
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		desc.RemoveIngester("instance-1")
		desc.RemoveIngester("instance-6")
		return desc, true, nil
	}))

	test.Poll(t, time.Second, []Instance{{"127.0.0.5", true}}, func() interface{} {
		return receiver.getDiscoveredInstances()
	})

	// A non-active instance switches to active.
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		instance := desc.Ingesters["instance-2"]
		instance.State = ring.ACTIVE
		desc.Ingesters["instance-2"] = instance
		return desc, true, nil
	}))

	test.Poll(t, time.Second, []Instance{{"127.0.0.2", true}, {"127.0.0.5", true}}, func() interface{} {
		return receiver.getDiscoveredInstances()
	})

	// An active becomes unhealthy.
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		instance := desc.Ingesters["instance-2"]
		instance.Timestamp = time.Now().Add(-2 * ringCfg.HeartbeatTimeout).Unix()
		desc.Ingesters["instance-2"] = instance
		return desc, true, nil
	}))

	test.Poll(t, time.Second, []Instance{{"127.0.0.5", true}}, func() interface{} {
		return receiver.getDiscoveredInstances()
	})
}

func TestRingServiceDiscovery_WithMaxUsedInstances(t *testing.T) {
	const (
		ringKey          = "test"
		ringCheckPeriod  = 100 * time.Millisecond // Check very frequently to speed up the test.
		maxUsedInstances = 2
	)

	// Use an in-memory KV store.
	inmem, closer := consul.NewInMemoryClient(ring.GetCodec(), log.NewNopLogger(), nil)
	t.Cleanup(func() { _ = closer.Close() })

	// Create a ring client.
	ringCfg := ring.Config{HeartbeatTimeout: time.Minute, ReplicationFactor: 1}
	ringClient, err := ring.NewWithStoreClientAndStrategy(ringCfg, "test", ringKey, inmem, ring.NewDefaultReplicationStrategy(), nil, log.NewNopLogger())
	require.NoError(t, err)

	// Create an empty ring.
	ctx := context.Background()
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		return ring.NewDesc(), true, nil
	}))

	// Mock a receiver to keep track of all notified addresses.
	receiver := newNotificationsReceiverMock()

	sd := NewRing(ringClient, ringCheckPeriod, maxUsedInstances, receiver)

	// Start the service discovery.
	require.NoError(t, services.StartAndAwaitRunning(ctx, sd))
	t.Cleanup(func() {
		require.NoError(t, services.StopAndAwaitTerminated(ctx, sd))
	})

	// Wait some time, we expect no address notified because the ring is empty.
	time.Sleep(time.Second)
	require.Empty(t, receiver.getDiscoveredInstances())

	// Register some instances.
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		desc.AddIngester("instance-1", "127.0.0.1", "", nil, ring.ACTIVE, time.Now())
		desc.AddIngester("instance-2", "127.0.0.2", "", nil, ring.PENDING, time.Now())
		desc.AddIngester("instance-3", "127.0.0.3", "", nil, ring.JOINING, time.Now())
		desc.AddIngester("instance-4", "127.0.0.4", "", nil, ring.LEAVING, time.Now())
		return desc, true, nil
	}))

	test.Poll(t, time.Second, []Instance{{"127.0.0.1", true}}, func() interface{} {
		return receiver.getDiscoveredInstances()
	})

	// Register more instances.
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		desc.AddIngester("instance-5", "127.0.0.5", "", nil, ring.ACTIVE, time.Now())
		desc.AddIngester("instance-6", "127.0.0.6", "", nil, ring.ACTIVE, time.Now())
		return desc, true, nil
	}))

	test.Poll(t, time.Second, []Instance{{"127.0.0.1", true}, {"127.0.0.5", true}, {"127.0.0.6", false}}, func() interface{} {
		return receiver.getDiscoveredInstances()
	})

	// Unregister some instances.
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		desc.RemoveIngester("instance-1")
		desc.RemoveIngester("instance-6")
		return desc, true, nil
	}))

	test.Poll(t, time.Second, []Instance{{"127.0.0.5", true}}, func() interface{} {
		return receiver.getDiscoveredInstances()
	})

	// Some non-active instances switch to active.
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		instance := desc.Ingesters["instance-2"]
		instance.State = ring.ACTIVE
		desc.Ingesters["instance-2"] = instance

		instance = desc.Ingesters["instance-3"]
		instance.State = ring.ACTIVE
		desc.Ingesters["instance-3"] = instance

		return desc, true, nil
	}))

	test.Poll(t, time.Second, []Instance{{"127.0.0.2", true}, {"127.0.0.3", true}, {"127.0.0.5", false}}, func() interface{} {
		return receiver.getDiscoveredInstances()
	})

	// An active becomes unhealthy.
	require.NoError(t, inmem.CAS(ctx, ringKey, func(in interface{}) (out interface{}, retry bool, err error) {
		desc := in.(*ring.Desc)
		instance := desc.Ingesters["instance-2"]
		instance.Timestamp = time.Now().Add(-2 * ringCfg.HeartbeatTimeout).Unix()
		desc.Ingesters["instance-2"] = instance
		return desc, true, nil
	}))

	test.Poll(t, time.Second, []Instance{{"127.0.0.3", true}, {"127.0.0.5", true}}, func() interface{} {
		return receiver.getDiscoveredInstances()
	})
}

func TestSelectInUseInstances(t *testing.T) {
	tests := map[string]struct {
		input        []ring.InstanceDesc
		maxInstances int
		expected     []ring.InstanceDesc
	}{
		"should return the input on empty list of instances": {
			input:        nil,
			maxInstances: 3,
			expected:     nil,
		},
		"should return the input on a number of instances < max instances": {
			input:        []ring.InstanceDesc{{Addr: "1.1.1.1"}, {Addr: "3.3.3.3"}},
			maxInstances: 3,
			expected:     []ring.InstanceDesc{{Addr: "1.1.1.1"}, {Addr: "3.3.3.3"}},
		},
		"should return the input on a number of instances = max instances": {
			input:        []ring.InstanceDesc{{Addr: "1.1.1.1"}, {Addr: "3.3.3.3"}, {Addr: "2.2.2.2"}},
			maxInstances: 3,
			expected:     []ring.InstanceDesc{{Addr: "1.1.1.1"}, {Addr: "3.3.3.3"}, {Addr: "2.2.2.2"}},
		},
		"should return a subset of the input on a number of instances > max instances": {
			input:        []ring.InstanceDesc{{Addr: "1.1.1.1"}, {Addr: "3.3.3.3"}, {Addr: "2.2.2.2"}, {Addr: "4.4.4.4"}, {Addr: "5.5.5.5"}},
			maxInstances: 3,
			expected:     []ring.InstanceDesc{{Addr: "1.1.1.1"}, {Addr: "2.2.2.2"}, {Addr: "3.3.3.3"}},
		},
		"should return the input if max instances is 0": {
			input:        []ring.InstanceDesc{{Addr: "1.1.1.1"}, {Addr: "3.3.3.3"}, {Addr: "2.2.2.2"}},
			maxInstances: 0,
			expected:     []ring.InstanceDesc{{Addr: "1.1.1.1"}, {Addr: "3.3.3.3"}, {Addr: "2.2.2.2"}},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.Equal(t, testData.expected, selectInUseInstances(testData.input, testData.maxInstances))
		})
	}
}

type notificationsReceiverMock struct {
	discoveredInstancesMx sync.Mutex
	discoveredInstances   map[string]Instance
}

func newNotificationsReceiverMock() *notificationsReceiverMock {
	return &notificationsReceiverMock{
		discoveredInstances: map[string]Instance{},
	}
}

func (r *notificationsReceiverMock) InstanceAdded(instance Instance) {
	r.discoveredInstancesMx.Lock()
	defer r.discoveredInstancesMx.Unlock()

	r.discoveredInstances[instance.Address] = instance
}

func (r *notificationsReceiverMock) InstanceRemoved(instance Instance) {
	r.discoveredInstancesMx.Lock()
	defer r.discoveredInstancesMx.Unlock()

	delete(r.discoveredInstances, instance.Address)
}

func (r *notificationsReceiverMock) InstanceChanged(instance Instance) {
	r.discoveredInstancesMx.Lock()
	defer r.discoveredInstancesMx.Unlock()

	r.discoveredInstances[instance.Address] = instance
}

func (r *notificationsReceiverMock) getDiscoveredInstances() []Instance {
	r.discoveredInstancesMx.Lock()
	defer r.discoveredInstancesMx.Unlock()

	out := make([]Instance, 0, len(r.discoveredInstances))
	for _, instance := range r.discoveredInstances {
		out = append(out, instance)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Address < out[j].Address
	})

	return out
}
