package compactor

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/metastore/compaction"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

var testConfig = Config{
	Levels: []LevelConfig{
		{MaxBlocks: 3},
		{MaxBlocks: 2},
		{MaxBlocks: 2},
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
		if !c.enqueue(e) {
			t.Errorf("failed to enqueue: %v", e)
		}
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

	for i, e := range make([]compaction.BlockEntry, 3) {
		e.Index = uint64(i)
		e.ID = strconv.Itoa(i)
		c.enqueue(e)
	}

	remove(c.queue.levels[0], compactionKey{}, "0", "1", "2")

	p := &plan{compactor: c, blocks: newBlockIter()}
	assert.Nil(t, p.nextJob())
}

func TestPlan_compact_by_time(t *testing.T) {
	c := NewCompactor(Config{
		Levels: []LevelConfig{
			{MaxBlocks: 5, MaxAge: 5},
			{MaxBlocks: 5, MaxAge: 5},
		},
	}, nil, nil, nil)

	for _, e := range []compaction.BlockEntry{
		{Tenant: "A", Shard: 1, Level: 0, Index: 1, AppendedAt: 10, ID: "1"},
		{Tenant: "B", Shard: 0, Level: 0, Index: 2, AppendedAt: 20, ID: "2"},
		{Tenant: "A", Shard: 1, Level: 0, Index: 3, AppendedAt: 30, ID: "3"},
	} {
		c.enqueue(e)
	}

	// Third block remains in the queue as
	// we need another push to evict it.
	expected := []*jobPlan{
		{
			compactionKey: compactionKey{tenant: "A", shard: 1, level: 0},
			name:          "b7b41276360564d4-TA-S1-L0",
			blocks:        []string{"1"},
		},
		{
			compactionKey: compactionKey{tenant: "B", shard: 0, level: 0},
			name:          "6021b5621680598b-TB-S0-L0",
			blocks:        []string{"2"},
		},
	}

	p := &plan{
		compactor: c,
		blocks:    newBlockIter(),
		now:       40,
	}

	planned := make([]*jobPlan, 0, len(expected))
	for j := p.nextJob(); j != nil; j = p.nextJob() {
		planned = append(planned, j)
	}

	assert.Equal(t, expected, planned)
}

func TestPlan_time_split(t *testing.T) {
	s := DefaultConfig()
	// To skip tombstones for simplicity.
	s.CleanupBatchSize = 0
	c := NewCompactor(s, nil, nil, nil)
	now := test.Time("2024-09-23T00:00:00Z")

	for i := 0; i < 10; i++ {
		now = now.Add(15 * time.Second)
		e := compaction.BlockEntry{
			Index:      uint64(i),
			AppendedAt: now.UnixNano(),
			Tenant:     "A",
			Shard:      1,
			Level:      0,
			ID:         test.ULID(now.Format(time.RFC3339)),
		}
		c.enqueue(e)
	}

	now = now.Add(time.Hour * 6)
	for i := 0; i < 5; i++ {
		now = now.Add(15 * time.Second)
		e := compaction.BlockEntry{
			Index:      uint64(i),
			AppendedAt: now.UnixNano(),
			Tenant:     "A",
			Shard:      1,
			Level:      0,
			ID:         test.ULID(now.Format(time.RFC3339)),
		}
		c.enqueue(e)
	}

	p := &plan{
		compactor: c,
		blocks:    newBlockIter(),
		now:       now.UnixNano(),
	}

	var i int
	var n int
	for j := p.nextJob(); j != nil; j = p.nextJob() {
		i++
		n += len(j.blocks)
	}

	assert.Equal(t, 2, i)
	assert.Equal(t, 15, n)
}

func TestJobPlan_isComplete(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		level    uint32
		blocks   int
		bytes    uint64
		expected bool
	}{
		{
			name:   "below both thresholds",
			config: Config{Levels: []LevelConfig{{}, {MaxBlocks: 10}}, MaxJobBytes: 100},
			level:  1, blocks: 3, bytes: 10, expected: false,
		},
		{
			name:   "count threshold reached, bytes below",
			config: Config{Levels: []LevelConfig{{}, {MaxBlocks: 5}}, MaxJobBytes: 1000},
			level:  1, blocks: 5, bytes: 10, expected: true,
		},
		{
			name:   "bytes threshold reached, count below",
			config: Config{Levels: []LevelConfig{{}, {MaxBlocks: 100}}, MaxJobBytes: 100},
			level:  1, blocks: 2, bytes: 150, expected: true,
		},
		{
			name:   "byte cap disabled at level 0 regardless of MaxJobBytes",
			config: Config{Levels: []LevelConfig{{MaxBlocks: 100}}, MaxJobBytes: 10},
			blocks: 1, bytes: 1_000_000, expected: false,
		},
		{
			name:   "byte cap disabled globally when MaxJobBytes is 0",
			config: Config{Levels: []LevelConfig{{}, {MaxBlocks: 100}}, MaxJobBytes: 0},
			level:  1, blocks: 1, bytes: 1_000_000, expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &jobPlan{config: &tt.config, blocks: make([]string, tt.blocks), bytes: tt.bytes}
			job.level = tt.level
			require.Equal(t, tt.expected, job.isComplete())
		})
	}
}

// TestPlan_compact_by_size enqueues 4 blocks sharing a compaction key, with
// MaxBlocks set to the batch size (4) so the whole batch is flushed to the
// queue atomically (batches only become visible to the planner once
// flushed - see (*Config).exceedsMaxSize). The byte cap (100) is
// deliberately reached (at 120 bytes) before the block-count limit (4) is,
// so completing after 3 blocks is attributable to the byte cap alone, not
// to MaxBlocks.
func TestPlan_compact_by_size(t *testing.T) {
	c := NewCompactor(Config{
		Levels:      []LevelConfig{{MaxBlocks: 10}, {MaxBlocks: 4}}, // L0 (unused), L1
		MaxJobBytes: 100,
	}, nil, nil, nil)

	for _, e := range []compaction.BlockEntry{
		{Tenant: "A", Shard: 1, Level: 1, Index: 1, ID: "1", Size: 40},
		{Tenant: "A", Shard: 1, Level: 1, Index: 2, ID: "2", Size: 40},
		{Tenant: "A", Shard: 1, Level: 1, Index: 3, ID: "3", Size: 40}, // 120 >= 100: completes here.
		{Tenant: "A", Shard: 1, Level: 1, Index: 4, ID: "4", Size: 10}, // remains in the batch, alone.
	} {
		c.enqueue(e)
	}

	p := &plan{compactor: c, blocks: newBlockIter()}
	job := p.nextJob()
	require.NotNil(t, job)
	assert.Equal(t, []string{"1", "2", "3"}, job.blocks)
	require.Nil(t, p.nextJob()) // "4" alone hasn't completed the next job yet.
}

// TestPlan_compact_by_size_oversized_block enqueues 2 blocks (MaxBlocks: 2,
// so the batch flushes once both have arrived), the first of which alone
// already exceeds MaxJobBytes. isComplete() must trip immediately after
// this single block is added (len(blocks) == 1 < MaxBlocks == 2, so this is
// attributable to the byte cap, not the block count), proving a single
// oversized block always forms a valid job on its own.
func TestPlan_compact_by_size_oversized_block(t *testing.T) {
	c := NewCompactor(Config{
		Levels:      []LevelConfig{{MaxBlocks: 10}, {MaxBlocks: 2}},
		MaxJobBytes: 100,
	}, nil, nil, nil)

	c.enqueue(compaction.BlockEntry{Tenant: "A", Shard: 1, Level: 1, Index: 1, ID: "1", Size: 500})
	c.enqueue(compaction.BlockEntry{Tenant: "A", Shard: 1, Level: 1, Index: 2, ID: "2", Size: 10})

	p := &plan{compactor: c, blocks: newBlockIter()}
	job := p.nextJob()
	require.NotNil(t, job, "a single oversized block must still form a valid job, never a stuck/empty plan")
	assert.Equal(t, []string{"1"}, job.blocks)
	require.Nil(t, p.nextJob())
}

// TestPlan_compact_by_size_excludes_level_zero enqueues 2 L0 blocks
// (MaxBlocks: 2, so the batch flushes once both arrive) whose combined size
// vastly exceeds MaxJobBytes. If the byte cap applied at level 0, the job
// would complete after just the first block (500 >= 10); instead both
// blocks must be included, proving the byte cap is exempted at level 0.
func TestPlan_compact_by_size_excludes_level_zero(t *testing.T) {
	c := NewCompactor(Config{
		Levels:      []LevelConfig{{MaxBlocks: 2}},
		MaxJobBytes: 10, // would trip after a single block at any other level.
	}, nil, nil, nil)

	for _, e := range []compaction.BlockEntry{
		{Tenant: "A", Shard: 1, Level: 0, Index: 1, ID: "1", Size: 1000},
		{Tenant: "A", Shard: 1, Level: 0, Index: 2, ID: "2", Size: 1000},
	} {
		c.enqueue(e)
	}

	p := &plan{compactor: c, blocks: newBlockIter()}
	job := p.nextJob()
	require.NotNil(t, job)
	assert.Equal(t, []string{"1", "2"}, job.blocks)
}

// TestPlan_compact_by_size_mixed_with_legacy_zero_size_block enqueues a
// Size:0 block (as a queue snapshot restored before size tracking existed
// would produce, see README.md's "Job Planner" section) alongside sized
// blocks in the same job. MaxBlocks (6) is kept above the number of blocks
// needed to trip the byte cap (4), so completion after 4 blocks is
// attributable to the byte cap, not the block count. The zero-size block
// must be included in the job like any other block, contributing nothing
// to the byte accumulator (not causing early completion, and not being
// skipped or excluded).
func TestPlan_compact_by_size_mixed_with_legacy_zero_size_block(t *testing.T) {
	c := NewCompactor(Config{
		Levels:      []LevelConfig{{MaxBlocks: 10}, {MaxBlocks: 6}}, // L0 (unused), L1
		MaxJobBytes: 100,
	}, nil, nil, nil)

	for _, e := range []compaction.BlockEntry{
		{Tenant: "A", Shard: 1, Level: 1, Index: 1, ID: "1", Size: 0}, // legacy-restored: no recorded size.
		{Tenant: "A", Shard: 1, Level: 1, Index: 2, ID: "2", Size: 40},
		{Tenant: "A", Shard: 1, Level: 1, Index: 3, ID: "3", Size: 40},
		{Tenant: "A", Shard: 1, Level: 1, Index: 4, ID: "4", Size: 40}, // 120 >= 100: completes here (4 < MaxBlocks 6).
		{Tenant: "A", Shard: 1, Level: 1, Index: 5, ID: "5", Size: 10}, // remains in the batch.
		{Tenant: "A", Shard: 1, Level: 1, Index: 6, ID: "6", Size: 10}, // flushes the batch (size == MaxBlocks).
	} {
		c.enqueue(e)
	}

	p := &plan{compactor: c, blocks: newBlockIter()}
	job := p.nextJob()
	require.NotNil(t, job)
	assert.Equal(t, []string{"1", "2", "3", "4"}, job.blocks,
		"the zero-size block is included in the job like any other block")
	require.Nil(t, p.nextJob(), "the remaining two blocks (20 bytes, 2 < MaxBlocks) haven't completed a job yet")
}

func TestPlan_remove_staged_batch_corrupts_queue(t *testing.T) {
	c := NewCompactor(testConfig, nil, nil, nil)

	for i := 0; i < 3; i++ {
		e := compaction.BlockEntry{
			Index:  uint64(i),
			ID:     fmt.Sprint(i),
			Tenant: "baseline",
			Shard:  1,
			Level:  0,
		}
		c.enqueue(e)
	}

	p1 := &plan{compactor: c, blocks: newBlockIter()}
	require.NotNil(t, p1.nextJob())
	require.Nil(t, p1.nextJob())

	// Add and remove blocks before they got to the compaction
	// queue, triggering removal of the staged batch.
	for i := 10; i < 12; i++ {
		e := compaction.BlockEntry{
			Index:  uint64(i + 10),
			ID:     fmt.Sprint(i),
			Tenant: "temp",
			Shard:  1,
			Level:  0,
		}
		c.enqueue(e)
	}

	level0 := c.queue.levels[0]
	tempKey := compactionKey{tenant: "temp", shard: 1, level: 0}
	if tempStaged, exists := level0.staged[tempKey]; exists {
		tempStaged.delete("10")
		tempStaged.delete("11")
	} else {
		t.Fatal("Compaction queue not found")
	}

	p2 := &plan{compactor: c, blocks: newBlockIter()}
	if job := p2.nextJob(); job == nil {
		t.Fatal("🐛🐛🐛: Corrupted compaction queue")
	}

	require.Nil(t, p1.nextJob(), "A single job is expected.")
}
