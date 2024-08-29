package singlereplica

import (
	"time"

	"github.com/grafana/dskit/ring"
)

// The replication strategy that returns all the instances, regardless
// of their health and placement to allow the caller to decide which
// instances to use on its own.

type replicationStrategy struct{}

func (replicationStrategy) Filter(
	instances []ring.InstanceDesc,
	_ ring.Operation,
	_ int,
	_ time.Duration,
	_ bool,
) ([]ring.InstanceDesc, int, error) {
	return instances, 0, nil
}

func NewReplicationStrategy() ring.ReplicationStrategy { return replicationStrategy{} }
