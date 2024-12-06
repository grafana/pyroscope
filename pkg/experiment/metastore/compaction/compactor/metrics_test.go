package compactor

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
)

func TestCollectorRegistration(t *testing.T) {
	reg := prometheus.NewRegistry()
	for i := 0; i < 2; i++ {
		entries := []compaction.BlockEntry{
			{Tenant: "A", Shard: 0, Level: 0},
			{Tenant: "A", Shard: 0, Level: 1},
			{Tenant: "A", Shard: 0, Level: 1},
			{Tenant: "A", Shard: 1, Level: 0},
			{Tenant: "B", Shard: 0, Level: 0},
		}
		c := NewCompactor(testConfig, nil, nil, reg)
		for _, e := range entries {
			c.enqueue(e)
		}
		c.queue.reset()
		for _, e := range entries {
			c.enqueue(e)
		}
	}
}
