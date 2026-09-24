package compactor

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
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

func gatherSum(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var total float64
	for _, mf := range metrics {
		if mf.GetName() == name {
			for _, m := range mf.GetMetric() {
				total += m.GetGauge().GetValue()
			}
		}
	}
	return total
}

func TestResetClearsGlobalStats(t *testing.T) {
	reg := prometheus.NewRegistry()
	entries := []compaction.BlockEntry{
		{ID: "b1", Tenant: "A", Shard: 0, Level: 0},
		{ID: "b2", Tenant: "A", Shard: 0, Level: 0},
		{ID: "b3", Tenant: "A", Shard: 1, Level: 0},
	}
	c := NewCompactor(testConfig, nil, nil, reg)
	for _, e := range entries {
		c.enqueue(e)
	}
	c.queue.reset()
	for _, e := range entries {
		c.enqueue(e)
	}

	blocks := gatherSum(t, reg, "compaction_global_queue_blocks_current")
	if blocks != 3 {
		t.Errorf("expected 3 blocks after reset+re-enqueue, got %v (counter leak detected)", blocks)
	}
}

func TestUpdatePlanUnknownKeyDoesNotLeakQueues(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewCompactor(testConfig, nil, nil, reg)
	c.enqueue(compaction.BlockEntry{ID: "b1", Tenant: "A", Shard: 0, Level: 0})

	before := gatherSum(t, reg, "compaction_global_queue_queues_current")

	_ = c.UpdatePlan(nil, &raft_log.CompactionPlanUpdate{
		NewJobs: []*raft_log.NewCompactionJob{
			{Plan: &raft_log.CompactionJobPlan{
				Tenant:          "Z",
				Shard:           99,
				CompactionLevel: 0,
				SourceBlocks:    []string{"no-such-block"},
			}},
		},
	})

	after := gatherSum(t, reg, "compaction_global_queue_queues_current")
	if after != before {
		t.Errorf("queues_current changed: before=%v after=%v (secondary leak detected)", before, after)
	}
}
