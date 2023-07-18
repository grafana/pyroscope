// SPDX-License-Identifier: AGPL-3.0-only

package distributor

import (
	"testing"
	"time"

	"github.com/grafana/dskit/ring"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

type nopDelegate struct{}

func (n nopDelegate) OnRingInstanceRegister(lifecycler *ring.BasicLifecycler, ringDesc ring.Desc, instanceExists bool, instanceID string, instanceDesc ring.InstanceDesc) (ring.InstanceState, ring.Tokens) {
	return instanceDesc.State, instanceDesc.GetTokens()
}

func (n nopDelegate) OnRingInstanceTokens(lifecycler *ring.BasicLifecycler, tokens ring.Tokens) {
}

func (n nopDelegate) OnRingInstanceStopping(lifecycler *ring.BasicLifecycler) {
}

func (n nopDelegate) OnRingInstanceHeartbeat(lifecycler *ring.BasicLifecycler, ringDesc *ring.Desc, instanceDesc *ring.InstanceDesc) {
}

func TestHealthyInstanceDelegate_OnRingInstanceHeartbeat(t *testing.T) {
	// addInstance registers a new instance with the given ring and sets its last heartbeat timestamp
	addInstance := func(desc *ring.Desc, id string, state ring.InstanceState, timestamp int64) {
		instance := desc.AddIngester(id, "127.0.0.1", "", []uint32{1}, state, time.Now())
		instance.Timestamp = timestamp
		desc.Ingesters[id] = instance
	}

	tests := map[string]struct {
		ringSetup        func(desc *ring.Desc)
		heartbeatTimeout time.Duration
		expectedCount    uint32
	}{
		"all instances healthy and active": {
			ringSetup: func(desc *ring.Desc) {
				now := time.Now()
				addInstance(desc, "distributor-1", ring.ACTIVE, now.Unix())
				addInstance(desc, "distributor-2", ring.ACTIVE, now.Unix())
				addInstance(desc, "distributor-3", ring.ACTIVE, now.Unix())
			},
			heartbeatTimeout: time.Minute,
			expectedCount:    3,
		},

		"all instances healthy not all instances active": {
			ringSetup: func(desc *ring.Desc) {
				now := time.Now()
				addInstance(desc, "distributor-1", ring.ACTIVE, now.Unix())
				addInstance(desc, "distributor-2", ring.LEAVING, now.Unix())
				addInstance(desc, "distributor-3", ring.ACTIVE, now.Unix())
			},
			heartbeatTimeout: time.Minute,
			expectedCount:    2,
		},

		"some instances healthy all instances active": {
			ringSetup: func(desc *ring.Desc) {
				now := time.Now()
				addInstance(desc, "distributor-1", ring.ACTIVE, now.Unix())
				addInstance(desc, "distributor-2", ring.ACTIVE, now.Unix())
				addInstance(desc, "distributor-3", ring.ACTIVE, now.Add(-5*time.Minute).Unix())
			},
			heartbeatTimeout: time.Minute,
			expectedCount:    2,
		},

		"some instances healthy but timeout disabled all instances active": {
			ringSetup: func(desc *ring.Desc) {
				now := time.Now()
				addInstance(desc, "distributor-1", ring.ACTIVE, now.Unix())
				addInstance(desc, "distributor-2", ring.ACTIVE, now.Unix())
				addInstance(desc, "distributor-3", ring.ACTIVE, now.Add(-5*time.Minute).Unix())
			},
			heartbeatTimeout: 0,
			expectedCount:    3,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			count := atomic.NewUint32(0)
			ringDesc := ring.NewDesc()

			testData.ringSetup(ringDesc)
			instance := ringDesc.Ingesters["distributor-1"]

			delegate := newHealthyInstanceDelegate(count, testData.heartbeatTimeout, &nopDelegate{})
			delegate.OnRingInstanceHeartbeat(&ring.BasicLifecycler{}, ringDesc, &instance)

			assert.Equal(t, testData.expectedCount, count.Load())
		})
	}
}
