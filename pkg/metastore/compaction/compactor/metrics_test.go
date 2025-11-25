package compactor

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/metastore/compaction"
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

func TestBlockQueueAggregatedMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCompactor(testConfig, nil, nil, reg)

	entries := []compaction.BlockEntry{
		{ID: "block1", Tenant: "A", Shard: 0, Level: 0},
		{ID: "block2", Tenant: "A", Shard: 0, Level: 0},
		{ID: "block3", Tenant: "A", Shard: 0, Level: 0},
		{ID: "block4", Tenant: "A", Shard: 1, Level: 0},
		{ID: "block5", Tenant: "B", Shard: 0, Level: 1},
		{ID: "block6", Tenant: "B", Shard: 0, Level: 1},
		{ID: "block7", Tenant: "B", Shard: 0, Level: 1},
	}

	for _, e := range entries {
		c.enqueue(e)
	}

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	var blocksTotal, queuesTotal, batchesTotal float64
	var foundBlocks, foundQueues, foundBatches bool

	for _, mf := range metrics {
		if mf.GetName() == "compaction_global_queue_blocks_current" {
			for _, m := range mf.GetMetric() {
				blocksTotal += m.GetGauge().GetValue()
				foundBlocks = true
			}
		}
		if mf.GetName() == "compaction_global_queue_queues_current" {
			for _, m := range mf.GetMetric() {
				queuesTotal += m.GetGauge().GetValue()
				foundQueues = true
			}
		}

		if mf.GetName() == "compaction_global_queue_batches_current" {
			for _, m := range mf.GetMetric() {
				batchesTotal += m.GetGauge().GetValue()
				foundBatches = true
			}
		}
	}

	if !foundBlocks {
		t.Fatal("compaction_global_queue_blocks metric not found")
	}
	if !foundQueues {
		t.Fatal("compaction_global_queue_queues metric not found")
	}
	if !foundBatches {
		t.Fatal("compaction_global_queue_batches_current metric not found")
	}

	if blocksTotal != 7 {
		t.Errorf("expected 7 total blocks, got %v", blocksTotal)
	}

	if queuesTotal != 3 {
		t.Errorf("expected 3 total queues, got %v", queuesTotal)
	}

	// testConfig.Levels[0].MaxBlocks = 3
	// testConfig.Levels[1].MaxBlocks = 2
	// (A,0): 3 blocks → 3/3 = 1 batch
	// (A,1): 1 block → 1/2 = 0 batches
	// (B,1): 3 blocks → 3/2 = 1 batch
	// Total = 2 batches
	if batchesTotal != 2 {
		t.Errorf("expected 2 total batches, got %v", batchesTotal)
	}
}
