package compactor

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
)

var testConfig = Config{
	Strategy: Strategy{
		MaxBlocksPerLevel: []uint{3, 2, 2},
		MaxBlocksDefault:  2,
		MaxBatchAge:       0,
		MaxLevel:          3,
	},
}

func TestPlan_same_level(t *testing.T) {
	c := NewCompactor(testConfig, nil, nil, nil)

	var i int // The index is used outside the loop.
	for _, e := range []compaction.BlockEntry{
		{Tenant: "A", Shard: 0, Level: 0},
		{Tenant: "B", Shard: 2, Level: 0},
		{Tenant: "A", Shard: 1, Level: 0},
		{Tenant: "A", Shard: 1, Level: 0},
		{Tenant: "B", Shard: 2, Level: 0},
		{Tenant: "A", Shard: 1, Level: 0}, // TA-S1-L0 is ready
		{Tenant: "B", Shard: 2, Level: 0}, // TB-S2-L0
		{Tenant: "A", Shard: 0, Level: 0},
		{Tenant: "A", Shard: 1, Level: 0},
		{Tenant: "A", Shard: 0, Level: 0}, // TA-S0-L0
		{Tenant: "B", Shard: 2, Level: 0},
		{Tenant: "A", Shard: 1, Level: 0},
	} {
		e.Index = uint64(i)
		e.ID = strconv.Itoa(i)
		c.enqueue(e)
		i++
	}

	expected := []*jobPlan{
		{
			compactionKey: compactionKey{tenant: "A", shard: 1, level: 0},
			name:          "ffba6b12acb007e6-TA-S1-L0",
			blocks:        []string{"2", "3", "5"},
		},
		{
			compactionKey: compactionKey{tenant: "B", shard: 2, level: 0},
			name:          "3860b3ec2cf5bfa3-TB-S2-L0",
			blocks:        []string{"1", "4", "6"},
		},
		{
			compactionKey: compactionKey{tenant: "A", shard: 0, level: 0},
			name:          "6a1fee35d1568267-TA-S0-L0",
			blocks:        []string{"0", "7", "9"},
		},
	}

	p := &plan{compactor: c, blocks: newBlockIter()}
	planned := make([]*jobPlan, 0, len(expected))
	for j := p.nextJob(); j != nil; j = p.nextJob() {
		planned = append(planned, j)
	}
	assert.Equal(t, expected, planned)

	// Now we're adding some more blocks to produce more jobs,
	// using the same queue. We expect all the previously planned
	// jobs and new ones.
	expected = append(expected, []*jobPlan{
		{
			compactionKey: compactionKey{tenant: "A", shard: 1, level: 0},
			name:          "34d4246acbf55d05-TA-S1-L0",
			blocks:        []string{"8", "11", "13"},
		},
		{
			compactionKey: compactionKey{tenant: "B", shard: 2, level: 0},
			name:          "5567ff0cdb349aaf-TB-S2-L0",
			blocks:        []string{"10", "12", "14"},
		},
	}...)

	for _, e := range []compaction.BlockEntry{
		{Tenant: "B", Shard: 2, Level: 0},
		{Tenant: "A", Shard: 1, Level: 0}, // TA-S1-L0 is ready
		{Tenant: "B", Shard: 2, Level: 0}, // TB-S2-L0
	} {
		e.Index = uint64(i)
		e.ID = strconv.Itoa(i)
		c.enqueue(e)
		i++
	}

	p = &plan{compactor: c, blocks: newBlockIter()}
	planned = planned[:0] // Old jobs should be re-planned.
	for j := p.nextJob(); j != nil; j = p.nextJob() {
		planned = append(planned, j)
	}
	assert.Equal(t, expected, planned)
}

func TestPlan_level_priority(t *testing.T) {
	c := NewCompactor(testConfig, nil, nil, nil)

	// Lower level job should be planned first despite the arrival order.
	var i int
	for _, e := range []compaction.BlockEntry{
		{Tenant: "B", Shard: 2, Level: 1},
		{Tenant: "A", Shard: 1, Level: 0},
		{Tenant: "A", Shard: 1, Level: 0},
		{Tenant: "B", Shard: 2, Level: 1}, // TB-S2-L1 is ready
		{Tenant: "A", Shard: 1, Level: 0}, // TA-S1-L0
	} {
		e.Index = uint64(i)
		e.ID = strconv.Itoa(i)
		c.enqueue(e)
		i++
	}

	expected := []*jobPlan{
		{
			compactionKey: compactionKey{tenant: "A", shard: 1, level: 0},
			name:          "3567f9a8f34203a9-TA-S1-L0",
			blocks:        []string{"1", "2", "4"},
		},
		{
			compactionKey: compactionKey{tenant: "B", shard: 2, level: 1},
			name:          "3254788b90b8fafc-TB-S2-L1",
			blocks:        []string{"0", "3"},
		},
	}

	p := &plan{compactor: c, blocks: newBlockIter()}
	planned := make([]*jobPlan, 0, len(expected))
	for j := p.nextJob(); j != nil; j = p.nextJob() {
		planned = append(planned, j)
	}

	assert.Equal(t, expected, planned)
}

func TestPlan_empty_queue(t *testing.T) {
	c := NewCompactor(testConfig, nil, nil, nil)

	p := &plan{compactor: c, blocks: newBlockIter()}
	assert.Nil(t, p.nextJob())

	c.enqueue(compaction.BlockEntry{
		Index:  0,
		ID:     "0",
		Tenant: "A",
		Shard:  1,
		Level:  1,
	})

	// L0 queue is empty.
	// L1 queue has one block.
	p = &plan{compactor: c, blocks: newBlockIter()}
	assert.Nil(t, p.nextJob())

	c.enqueue(compaction.BlockEntry{
		Index:  1,
		ID:     "1",
		Tenant: "A",
		Shard:  1,
		Level:  1,
	})

	// L0 queue is empty.
	// L2 has blocks for a job.
	p = &plan{compactor: c, blocks: newBlockIter()}
	assert.NotNil(t, p.nextJob())
}

func TestPlan_deleted_blocks(t *testing.T) {
	c := NewCompactor(testConfig, nil, nil, nil)

	var i int // The index is used outside the loop.
	for _, e := range []compaction.BlockEntry{
		{Tenant: "A", Shard: 1, Level: 0},
		{Tenant: "B", Shard: 2, Level: 0},
		{Tenant: "A", Shard: 1, Level: 0},
		{Tenant: "B", Shard: 2, Level: 0},
		{Tenant: "A", Shard: 1, Level: 0}, // TA-S1-L0 is ready
		{Tenant: "B", Shard: 2, Level: 0}, // TB-S2-L0
	} {
		e.Index = uint64(i)
		e.ID = strconv.Itoa(i)
		c.enqueue(e)
		i++
	}

	// Invalidate TA-S1-L0 plan by removing some blocks.
	remove(c.queue.levels[0], compactionKey{
		tenant: "A",
		shard:  1,
		level:  0,
	}, "0", "4")

	// "0" - - -
	// "1" {Tenant: "B", Shard: 2, Level: 0},
	// "2" {Tenant: "A", Shard: 1, Level: 0},
	// "3" {Tenant: "B", Shard: 2, Level: 0},
	// "4" - - -                              // TA-S1-L0 would be created here.
	// "5" {Tenant: "B", Shard: 2, Level: 0}, // TB-S2-L0 is ready
	expected := []*jobPlan{
		{
			compactionKey: compactionKey{tenant: "B", shard: 2, level: 0},
			name:          "5668d093d5b7cc2f-TB-S2-L0",
			blocks:        []string{"1", "3", "5"},
		},
	}

	p := &plan{compactor: c, blocks: newBlockIter()}
	planned := make([]*jobPlan, 0, len(expected))
	for j := p.nextJob(); j != nil; j = p.nextJob() {
		planned = append(planned, j)
	}
	assert.Equal(t, expected, planned)

	// Now we add some more blocks to make sure that the
	// invalidated queue can still be compacted.
	for _, e := range []compaction.BlockEntry{
		{Tenant: "A", Shard: 1, Level: 0},
		{Tenant: "A", Shard: 1, Level: 0},
		{Tenant: "A", Shard: 1, Level: 0},
	} {
		e.Index = uint64(i)
		e.ID = strconv.Itoa(i)
		c.enqueue(e)
		i++
	}

	expected = append([]*jobPlan{
		{
			compactionKey: compactionKey{tenant: "A", shard: 1, level: 0},
			name:          "69cebc117138be9-TA-S1-L0",
			blocks:        []string{"2", "6", "7"},
		},
	}, expected...)

	p = &plan{compactor: c, blocks: newBlockIter()}
	planned = planned[:0]
	for j := p.nextJob(); j != nil; j = p.nextJob() {
		planned = append(planned, j)
	}
	assert.Equal(t, expected, planned)
}

func TestPlan_deleted_batch(t *testing.T) {
	c := NewCompactor(testConfig, nil, nil, nil)

	for i, e := range []compaction.BlockEntry{{}, {}, {}} {
		e.Index = uint64(i)
		e.ID = strconv.Itoa(i)
		c.enqueue(e)
	}

	remove(c.queue.levels[0], compactionKey{}, "0", "1", "2")

	p := &plan{compactor: c, blocks: newBlockIter()}
	assert.Nil(t, p.nextJob())
}
