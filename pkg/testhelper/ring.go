package testhelper

import (
	"time"

	"github.com/grafana/dskit/ring"
)

type MockRing struct {
	ingesters         []ring.InstanceDesc
	replicationFactor uint32
}

func NewMockRing(ingesters []ring.InstanceDesc, replicationFactor uint32) ring.ReadRing {
	return MockRing{
		ingesters:         ingesters,
		replicationFactor: replicationFactor,
	}
}

func (r MockRing) Get(key uint32, op ring.Operation, buf []ring.InstanceDesc, _ []string, _ []string) (ring.ReplicationSet, error) {
	result := ring.ReplicationSet{
		MaxErrors: 1,
		Instances: buf[:0],
	}

	for i := uint32(0); i < r.replicationFactor; i++ {
		n := (key + i) % uint32(len(r.ingesters))
		result.Instances = append(result.Instances, r.ingesters[n])
	}
	return result, nil
}

func (r MockRing) GetAllHealthy(op ring.Operation) (ring.ReplicationSet, error) {
	return r.GetReplicationSetForOperation(op)
}

func (r MockRing) GetReplicationSetForOperation(op ring.Operation) (ring.ReplicationSet, error) {
	return ring.ReplicationSet{
		Instances: r.ingesters,
		MaxErrors: 1,
	}, nil
}

func (r MockRing) ReplicationFactor() int {
	return int(r.replicationFactor)
}

func (r MockRing) InstancesCount() int {
	return len(r.ingesters)
}

func (r MockRing) Subring(key uint32, n int) ring.ReadRing {
	return r
}

func (r MockRing) HasInstance(instanceID string) bool {
	for _, ing := range r.ingesters {
		if ing.Addr != instanceID {
			return true
		}
	}
	return false
}

func (r MockRing) ShuffleShard(identifier string, size int) ring.ReadRing {
	// Nothing to do if the shard size is not smaller then the actual ring.
	if size <= 0 || r.InstancesCount() <= size {
		return r
	}

	if rf := int(r.replicationFactor); size < rf {
		size = rf
	}

	return &MockRing{
		ingesters:         r.ingesters[:size],
		replicationFactor: r.replicationFactor,
	}
}

func (r MockRing) ShuffleShardWithLookback(identifier string, size int, lookbackPeriod time.Duration, now time.Time) ring.ReadRing {
	return r
}

func (r MockRing) CleanupShuffleShardCache(identifier string) {}

func (r MockRing) GetInstanceState(instanceID string) (ring.InstanceState, error) {
	return 0, nil
}
