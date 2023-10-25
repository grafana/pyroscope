// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/split_merge_grouper_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/sharding"
)

func TestPlanCompaction(t *testing.T) {
	const userID = "user-1"

	block1 := ulid.MustNew(1, nil)   // Hash: 283204220
	block2 := ulid.MustNew(2, nil)   // Hash: 444110359
	block3 := ulid.MustNew(3, nil)   // Hash: 3253786510
	block4 := ulid.MustNew(4, nil)   // Hash: 122298081
	block5 := ulid.MustNew(5, nil)   // Hash: 2931974232
	block6 := ulid.MustNew(6, nil)   // Hash: 3092880371
	block7 := ulid.MustNew(7, nil)   // Hash: 1607589226
	block8 := ulid.MustNew(8, nil)   // Hash: 2771068093
	block9 := ulid.MustNew(9, nil)   // Hash: 1285776948
	block10 := ulid.MustNew(10, nil) // Hash: 1446683087

	tests := map[string]struct {
		ranges      []int64
		shardCount  uint32
		splitGroups uint32
		blocks      []*block.Meta
		expected    []*job
	}{
		"no input blocks": {
			ranges:   []int64{20},
			blocks:   nil,
			expected: nil,
		},
		"should split a single block if == smallest compaction range": {
			ranges:     []int64{20, 40},
			shardCount: 1,
			blocks: []*block.Meta{
				{ULID: block1, MinTime: 0, MaxTime: 20},
			},
			expected: []*job{
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 0, MaxTime: 20},
					},
				}},
			},
		},
		"should split a single block if < smallest compaction range": {
			ranges:     []int64{20, 40},
			shardCount: 1,
			blocks: []*block.Meta{
				{ULID: block1, MinTime: 10, MaxTime: 20},
			},
			expected: []*job{
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 10, MaxTime: 20},
					},
				}},
			},
		},
		"should NOT split a single block if == smallest compaction range but configured shards = 0": {
			ranges:     []int64{20, 40},
			shardCount: 0,
			blocks: []*block.Meta{
				{ULID: block1, MinTime: 0, MaxTime: 20},
			},
			expected: []*job{},
		},
		"should merge and split multiple 1st level blocks within the same time range": {
			ranges:     []int64{10, 20},
			shardCount: 1,
			blocks: []*block.Meta{
				{ULID: block1, MinTime: 10, MaxTime: 20},
				{ULID: block2, MinTime: 10, MaxTime: 20},
			},
			expected: []*job{
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 10, MaxTime: 20},
						{ULID: block2, MinTime: 10, MaxTime: 20},
					},
				}},
			},
		},
		"should merge and split multiple 1st level blocks in different time ranges": {
			ranges:     []int64{10, 20},
			shardCount: 1,
			blocks: []*block.Meta{
				// 1st level range [0, 10]
				{ULID: block1, MinTime: 0, MaxTime: 10},
				{ULID: block2, MinTime: 0, MaxTime: 10},
				// 1st level range [10, 20]
				{ULID: block3, MinTime: 11, MaxTime: 20},
				{ULID: block4, MinTime: 11, MaxTime: 20},
			},
			expected: []*job{
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   10,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 0, MaxTime: 10},
						{ULID: block2, MinTime: 0, MaxTime: 10},
					},
				}},
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block3, MinTime: 11, MaxTime: 20},
						{ULID: block4, MinTime: 11, MaxTime: 20},
					},
				}},
			},
		},
		"should merge and split multiple 1st level blocks in different time ranges, single split group": {
			ranges:      []int64{10, 20},
			shardCount:  2,
			splitGroups: 1,
			blocks: []*block.Meta{
				// 1st level range [0, 10]
				{ULID: block1, MinTime: 0, MaxTime: 10},
				{ULID: block2, MinTime: 0, MaxTime: 10},
				// 1st level range [10, 20]
				{ULID: block3, MinTime: 11, MaxTime: 20},
				{ULID: block4, MinTime: 11, MaxTime: 20},
			},
			expected: []*job{
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   10,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 0, MaxTime: 10},
						{ULID: block2, MinTime: 0, MaxTime: 10},
					},
				}},
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block3, MinTime: 11, MaxTime: 20},
						{ULID: block4, MinTime: 11, MaxTime: 20},
					},
				}},
			},
		},
		"should merge and split multiple 1st level blocks in different time ranges, two split groups": {
			ranges:      []int64{10, 20},
			shardCount:  2,
			splitGroups: 2,
			blocks: []*block.Meta{
				// 1st level range [0, 10]
				{ULID: block1, MinTime: 0, MaxTime: 10},
				{ULID: block2, MinTime: 0, MaxTime: 10},
				// 1st level range [10, 20]
				{ULID: block3, MinTime: 11, MaxTime: 20},
				{ULID: block4, MinTime: 11, MaxTime: 20},
			},
			expected: []*job{
				{userID: userID, stage: stageSplit, shardID: "1_of_2", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   10,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 0, MaxTime: 10},
					},
				}},
				{userID: userID, stage: stageSplit, shardID: "2_of_2", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   10,
					blocks: []*block.Meta{
						{ULID: block2, MinTime: 0, MaxTime: 10},
					},
				}},
				{userID: userID, stage: stageSplit, shardID: "1_of_2", blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block3, MinTime: 11, MaxTime: 20},
					},
				}},
				{userID: userID, stage: stageSplit, shardID: "2_of_2", blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block4, MinTime: 11, MaxTime: 20},
					},
				}},
			},
		},
		"should merge but NOT split multiple 1st level blocks in different time ranges if configured shards = 0": {
			ranges:     []int64{10, 20},
			shardCount: 0,
			blocks: []*block.Meta{
				// 1st level range [0, 10]
				{ULID: block1, MinTime: 0, MaxTime: 10},
				{ULID: block2, MinTime: 0, MaxTime: 10},
				// 1st level range [10, 20]
				{ULID: block3, MinTime: 11, MaxTime: 20},
				{ULID: block4, MinTime: 11, MaxTime: 20},
			},
			expected: []*job{
				{userID: userID, stage: stageMerge, blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   10,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 0, MaxTime: 10},
						{ULID: block2, MinTime: 0, MaxTime: 10},
					},
				}},
				{userID: userID, stage: stageMerge, blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block3, MinTime: 11, MaxTime: 20},
						{ULID: block4, MinTime: 11, MaxTime: 20},
					},
				}},
			},
		},
		"should merge split blocks that can be compacted on the 2nd range only": {
			ranges:     []int64{10, 20},
			shardCount: 2,
			blocks: []*block.Meta{
				// 2nd level range [0, 20]
				{ULID: block1, MinTime: 0, MaxTime: 10, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
				{ULID: block2, MinTime: 10, MaxTime: 20, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
				{ULID: block3, MinTime: 0, MaxTime: 10, Labels: map[string]string{sharding.CompactorShardIDLabel: "2_of_2"}},
				{ULID: block4, MinTime: 10, MaxTime: 20, Labels: map[string]string{sharding.CompactorShardIDLabel: "2_of_2"}},
				// 2nd level range [20, 40]
				{ULID: block5, MinTime: 21, MaxTime: 30, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
				{ULID: block6, MinTime: 30, MaxTime: 40, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
			},
			expected: []*job{
				{userID: userID, stage: stageMerge, shardID: "1_of_2", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 0, MaxTime: 10, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
						{ULID: block2, MinTime: 10, MaxTime: 20, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
					},
				}},
				{userID: userID, stage: stageMerge, shardID: "2_of_2", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block3, MinTime: 0, MaxTime: 10, Labels: map[string]string{sharding.CompactorShardIDLabel: "2_of_2"}},
						{ULID: block4, MinTime: 10, MaxTime: 20, Labels: map[string]string{sharding.CompactorShardIDLabel: "2_of_2"}},
					},
				}},
				{userID: userID, stage: stageMerge, shardID: "1_of_2", blocksGroup: blocksGroup{
					rangeStart: 20,
					rangeEnd:   40,
					blocks: []*block.Meta{
						{ULID: block5, MinTime: 21, MaxTime: 30, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
						{ULID: block6, MinTime: 30, MaxTime: 40, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
					},
				}},
			},
		},
		"should not split non-split blocks if they're > smallest compaction range (do not split historical blocks after enabling splitting)": {
			ranges:     []int64{10, 20},
			shardCount: 2,
			blocks: []*block.Meta{
				// 2nd level range [0, 20]
				{ULID: block1, MinTime: 0, MaxTime: 10, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
				{ULID: block2, MinTime: 10, MaxTime: 20, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
				{ULID: block3, MinTime: 0, MaxTime: 10, Labels: map[string]string{sharding.CompactorShardIDLabel: "2_of_2"}},
				{ULID: block4, MinTime: 10, MaxTime: 20, Labels: map[string]string{sharding.CompactorShardIDLabel: "2_of_2"}},
				// 2nd level range [20, 40]
				{ULID: block5, MinTime: 21, MaxTime: 40},
				{ULID: block6, MinTime: 21, MaxTime: 40, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
			},
			expected: []*job{
				{userID: userID, stage: stageMerge, shardID: "1_of_2", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 0, MaxTime: 10, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
						{ULID: block2, MinTime: 10, MaxTime: 20, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
					},
				}},
				{userID: userID, stage: stageMerge, shardID: "2_of_2", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block3, MinTime: 0, MaxTime: 10, Labels: map[string]string{sharding.CompactorShardIDLabel: "2_of_2"}},
						{ULID: block4, MinTime: 10, MaxTime: 20, Labels: map[string]string{sharding.CompactorShardIDLabel: "2_of_2"}},
					},
				}},
			},
		},
		"input blocks can be compacted on a mix of 1st and 2nd ranges, guaranteeing no overlaps and giving preference to smaller ranges": {
			ranges:     []int64{10, 20},
			shardCount: 1,
			blocks: []*block.Meta{
				// To be split on 1st level range [0, 10]
				{ULID: block1, MinTime: 0, MaxTime: 10},
				{ULID: block2, MinTime: 7, MaxTime: 10},
				// Not compacted because on 2nd level because the range [0, 20]
				// has other 1st level range groups to be split first
				{ULID: block10, MinTime: 0, MaxTime: 10, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block3, MinTime: 10, MaxTime: 20, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				// To be compacted on 2nd level range [20, 40]
				{ULID: block4, MinTime: 21, MaxTime: 30, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block5, MinTime: 30, MaxTime: 40, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				// Already compacted on 2nd level range [40, 60]
				{ULID: block6, MinTime: 41, MaxTime: 60, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				// Not compacted on 2nd level because the range [60, 80]
				// has other 1st level range groups to be compacted first
				{ULID: block7, MinTime: 61, MaxTime: 70, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				// To be compacted on 1st level range [70, 80]
				{ULID: block8, MinTime: 71, MaxTime: 80, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block9, MinTime: 75, MaxTime: 80, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
			},
			expected: []*job{
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   10,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 0, MaxTime: 10},
						{ULID: block2, MinTime: 7, MaxTime: 10},
					},
				}},
				{userID: userID, stage: stageMerge, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 70,
					rangeEnd:   80,
					blocks: []*block.Meta{
						{ULID: block8, MinTime: 71, MaxTime: 80, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
						{ULID: block9, MinTime: 75, MaxTime: 80, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
					},
				}},
				{userID: userID, stage: stageMerge, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 20,
					rangeEnd:   40,
					blocks: []*block.Meta{
						{ULID: block4, MinTime: 21, MaxTime: 30, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
						{ULID: block5, MinTime: 30, MaxTime: 40, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
					},
				}},
			},
		},
		"input blocks have already been compacted with the largest range": {
			ranges:     []int64{10, 20, 40},
			shardCount: 1,
			blocks: []*block.Meta{
				{ULID: block1, MinTime: 0, MaxTime: 40, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block2, MinTime: 40, MaxTime: 70, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block3, MinTime: 80, MaxTime: 120, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
			},
			expected: nil,
		},
		"input blocks match the largest range but can be compacted because overlapping": {
			ranges:     []int64{10, 20, 40},
			shardCount: 1,
			blocks: []*block.Meta{
				{ULID: block1, MinTime: 0, MaxTime: 40, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block2, MinTime: 40, MaxTime: 70, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block3, MinTime: 81, MaxTime: 120, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block4, MinTime: 81, MaxTime: 120, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
			},
			expected: []*job{
				{userID: userID, stage: stageMerge, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 80,
					rangeEnd:   120,
					blocks: []*block.Meta{
						{ULID: block3, MinTime: 81, MaxTime: 120, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
						{ULID: block4, MinTime: 81, MaxTime: 120, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
					},
				}},
			},
		},
		"a block with time range crossing two 1st level ranges should be NOT considered for 1st level splitting": {
			ranges:     []int64{20, 40},
			shardCount: 1,
			blocks: []*block.Meta{
				{ULID: block1, MinTime: 10, MaxTime: 20},
				{ULID: block2, MinTime: 10, MaxTime: 30}, // This block spans across two 1st level ranges.
				{ULID: block3, MinTime: 21, MaxTime: 30},
				{ULID: block4, MinTime: 30, MaxTime: 40},
			},
			expected: []*job{
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 10, MaxTime: 20},
					},
				}},
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 20,
					rangeEnd:   40,
					blocks: []*block.Meta{
						{ULID: block3, MinTime: 21, MaxTime: 30},
						{ULID: block4, MinTime: 30, MaxTime: 40},
					},
				}},
			},
		},
		"a block with time range crossing two 1st level ranges should BE considered for 2nd level compaction": {
			ranges:     []int64{20, 40},
			shardCount: 1,
			blocks: []*block.Meta{
				{ULID: block1, MinTime: 0, MaxTime: 20, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block2, MinTime: 10, MaxTime: 30, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}}, // This block spans across two 1st level ranges.
				{ULID: block3, MinTime: 20, MaxTime: 40, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
			},
			expected: []*job{
				{userID: userID, stage: stageMerge, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   40,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 0, MaxTime: 20, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
						{ULID: block2, MinTime: 10, MaxTime: 30, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
						{ULID: block3, MinTime: 20, MaxTime: 40, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
					},
				}},
			},
		},
		"a block with time range larger then the largest compaction range should NOT be considered for compaction": {
			ranges:     []int64{10, 20, 40},
			shardCount: 1,
			blocks: []*block.Meta{
				{ULID: block1, MinTime: 0, MaxTime: 40, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block2, MinTime: 30, MaxTime: 150, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}}, // This block is larger then the largest compaction range.
				{ULID: block3, MinTime: 40, MaxTime: 70, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block4, MinTime: 81, MaxTime: 120, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
				{ULID: block5, MinTime: 81, MaxTime: 120, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
			},
			expected: []*job{
				{userID: userID, stage: stageMerge, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 80,
					rangeEnd:   120,
					blocks: []*block.Meta{
						{ULID: block4, MinTime: 81, MaxTime: 120, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
						{ULID: block5, MinTime: 81, MaxTime: 120, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_1"}},
					},
				}},
			},
		},
		"a range containing the most recent block shouldn't be prematurely compacted if doesn't cover the full range": {
			ranges:     []int64{10, 20, 40},
			shardCount: 1,
			blocks: []*block.Meta{
				{MinTime: 5, MaxTime: 8},
				{MinTime: 7, MaxTime: 9},
				{MinTime: 10, MaxTime: 12},
				{MinTime: 13, MaxTime: 15},
			},
			expected: []*job{
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   10,
					blocks: []*block.Meta{
						{MinTime: 5, MaxTime: 8},
						{MinTime: 7, MaxTime: 9},
					},
				}},
			},
		},
		"should not merge blocks within the same time range but with different external labels": {
			ranges:     []int64{10, 20},
			shardCount: 1,
			blocks: []*block.Meta{
				{ULID: block1, MinTime: 10, MaxTime: 20},
				{ULID: block2, MinTime: 10, MaxTime: 20, Labels: map[string]string{"another_group": "a"}},
				{ULID: block3, MinTime: 10, MaxTime: 20, Labels: map[string]string{"another_group": "a"}},
				{ULID: block4, MinTime: 10, MaxTime: 20, Labels: map[string]string{"another_group": "b"}},
			},
			expected: []*job{
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block1, MinTime: 10, MaxTime: 20},
					},
				}},
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block2, MinTime: 10, MaxTime: 20, Labels: map[string]string{"another_group": "a"}},
						{ULID: block3, MinTime: 10, MaxTime: 20, Labels: map[string]string{"another_group": "a"}},
					},
				}},
				{userID: userID, stage: stageSplit, shardID: "1_of_1", blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks: []*block.Meta{
						{ULID: block4, MinTime: 10, MaxTime: 20, Labels: map[string]string{"another_group": "b"}},
					},
				}},
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			actual := planCompaction(userID, testData.blocks, testData.ranges, testData.shardCount, testData.splitGroups)

			// Print the actual jobs (useful for debugging if tests fail).
			t.Logf("got %d jobs:", len(actual))
			for _, job := range actual {
				t.Logf("- %s", job.String())
			}

			assert.ElementsMatch(t, testData.expected, actual)
		})
	}
}

func TestPlanSplitting(t *testing.T) {
	const userID = "user-1"

	block1 := ulid.MustNew(1, nil) // Hash: 283204220
	block2 := ulid.MustNew(2, nil) // Hash: 444110359
	block3 := ulid.MustNew(3, nil) // Hash: 3253786510
	block4 := ulid.MustNew(4, nil) // Hash: 122298081
	block5 := ulid.MustNew(5, nil) // Hash: 2931974232

	tests := map[string]struct {
		blocks      blocksGroup
		splitGroups uint32
		expected    []*job
	}{
		"should return nil if the input group is empty": {
			blocks:      blocksGroup{},
			splitGroups: 2,
			expected:    nil,
		},
		"should return nil if the input group contains no non-sharded blocks": {
			blocks: blocksGroup{
				rangeStart: 10,
				rangeEnd:   20,
				blocks: []*block.Meta{
					{ULID: block1, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
					{ULID: block2, Labels: map[string]string{sharding.CompactorShardIDLabel: "2_of_2"}},
				},
			},
			splitGroups: 2,
			expected:    nil,
		},
		"should return a split job if the input group contains 1 non-sharded block": {
			blocks: blocksGroup{
				rangeStart: 10,
				rangeEnd:   20,
				blocks: []*block.Meta{
					{ULID: block1, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
					{ULID: block2},
				},
			},
			splitGroups: 2,
			expected: []*job{
				{
					blocksGroup: blocksGroup{
						rangeStart: 10,
						rangeEnd:   20,
						blocks: []*block.Meta{
							{ULID: block2},
						},
					},
					userID:  userID,
					stage:   stageSplit,
					shardID: "2_of_2",
				},
			},
		},
		"should splitGroups split jobs if the input group contains multiple non-sharded blocks": {
			blocks: blocksGroup{
				rangeStart: 10,
				rangeEnd:   20,
				blocks: []*block.Meta{
					{ULID: block1, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
					{ULID: block2},
					{ULID: block3},
					{ULID: block4},
					{ULID: block5, Labels: map[string]string{sharding.CompactorShardIDLabel: "1_of_2"}},
				},
			},
			splitGroups: 2,
			expected: []*job{
				{
					blocksGroup: blocksGroup{
						rangeStart: 10,
						rangeEnd:   20,
						blocks: []*block.Meta{
							{ULID: block3},
						},
					},
					userID:  userID,
					stage:   stageSplit,
					shardID: "1_of_2",
				}, {
					blocksGroup: blocksGroup{
						rangeStart: 10,
						rangeEnd:   20,
						blocks: []*block.Meta{
							{ULID: block2},
							{ULID: block4},
						},
					},
					userID:  userID,
					stage:   stageSplit,
					shardID: "2_of_2",
				},
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.ElementsMatch(t, testData.expected, planSplitting(userID, testData.blocks, testData.splitGroups))
		})
	}
}

func TestGroupBlocksByShardID(t *testing.T) {
	block1 := ulid.MustNew(1, nil)
	block2 := ulid.MustNew(2, nil)
	block3 := ulid.MustNew(3, nil)
	block4 := ulid.MustNew(4, nil)

	tests := map[string]struct {
		blocks   []*block.Meta
		expected map[string][]*block.Meta
	}{
		"no input blocks": {
			blocks:   nil,
			expected: map[string][]*block.Meta{},
		},
		"only 1 block in input with shard ID": {
			blocks: []*block.Meta{
				{ULID: block1, Labels: map[string]string{sharding.CompactorShardIDLabel: "1"}},
			},
			expected: map[string][]*block.Meta{
				"1": {
					{ULID: block1, Labels: map[string]string{sharding.CompactorShardIDLabel: "1"}},
				},
			},
		},
		"only 1 block in input without shard ID": {
			blocks: []*block.Meta{
				{ULID: block1},
			},
			expected: map[string][]*block.Meta{
				"": {
					{ULID: block1},
				},
			},
		},
		"multiple blocks per shard ID": {
			blocks: []*block.Meta{
				{ULID: block1, Labels: map[string]string{sharding.CompactorShardIDLabel: "1"}},
				{ULID: block2, Labels: map[string]string{sharding.CompactorShardIDLabel: "2"}},
				{ULID: block3, Labels: map[string]string{sharding.CompactorShardIDLabel: "1"}},
				{ULID: block4},
			},
			expected: map[string][]*block.Meta{
				"": {
					{ULID: block4},
				},
				"1": {
					{ULID: block1, Labels: map[string]string{sharding.CompactorShardIDLabel: "1"}},
					{ULID: block3, Labels: map[string]string{sharding.CompactorShardIDLabel: "1"}},
				},
				"2": {
					{ULID: block2, Labels: map[string]string{sharding.CompactorShardIDLabel: "2"}},
				},
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.Equal(t, testData.expected, groupBlocksByShardID(testData.blocks))
		})
	}
}

func TestGroupBlocksByRange(t *testing.T) {
	blockRange := 2 * time.Hour.Milliseconds()
	tests := map[string]struct {
		timeRange int64
		blocks    []*block.Meta
		expected  []blocksGroup
	}{
		"no input blocks": {
			timeRange: 20,
			blocks:    nil,
			expected:  nil,
		},
		"only 1 block in input": {
			timeRange: 20,
			blocks: []*block.Meta{
				{MinTime: 10, MaxTime: 20},
			},
			expected: []blocksGroup{
				{rangeStart: 0, rangeEnd: 20, blocks: []*block.Meta{
					{MinTime: 10, MaxTime: 20},
				}},
			},
		},
		"block start at the end of the range": {
			timeRange: 20,
			blocks: []*block.Meta{
				{MinTime: 10, MaxTime: 20},
				{MinTime: 20, MaxTime: 40},
			},
			expected: []blocksGroup{
				{rangeStart: 0, rangeEnd: 20, blocks: []*block.Meta{
					{MinTime: 10, MaxTime: 20},
				}},
				{rangeStart: 20, rangeEnd: 40, blocks: []*block.Meta{
					{MinTime: 20, MaxTime: 40},
				}},
			},
		},
		"only 1 block per range": {
			timeRange: 20,
			blocks: []*block.Meta{
				{MinTime: 10, MaxTime: 15},
				{MinTime: 21, MaxTime: 40},
				{MinTime: 41, MaxTime: 60},
			},
			expected: []blocksGroup{
				{rangeStart: 0, rangeEnd: 20, blocks: []*block.Meta{
					{MinTime: 10, MaxTime: 15},
				}},
				{rangeStart: 20, rangeEnd: 40, blocks: []*block.Meta{
					{MinTime: 21, MaxTime: 40},
				}},
				{rangeStart: 40, rangeEnd: 60, blocks: []*block.Meta{
					{MinTime: 41, MaxTime: 60},
				}},
			},
		},
		"multiple blocks per range": {
			timeRange: 20,
			blocks: []*block.Meta{
				{MinTime: 10, MaxTime: 15},
				{MinTime: 10, MaxTime: 20},
				{MinTime: 40, MaxTime: 60},
				{MinTime: 50, MaxTime: 55},
			},
			expected: []blocksGroup{
				{rangeStart: 0, rangeEnd: 20, blocks: []*block.Meta{
					{MinTime: 10, MaxTime: 15},
					{MinTime: 10, MaxTime: 20},
				}},
				{rangeStart: 40, rangeEnd: 60, blocks: []*block.Meta{
					{MinTime: 40, MaxTime: 60},
					{MinTime: 50, MaxTime: 55},
				}},
			},
		},
		"a block with time range larger then the range should be excluded": {
			timeRange: 20,
			blocks: []*block.Meta{
				{MinTime: 0, MaxTime: 20},
				{MinTime: 0, MaxTime: 40}, // This block is larger then the range.
				{MinTime: 10, MaxTime: 20},
				{MinTime: 21, MaxTime: 30},
			},
			expected: []blocksGroup{
				{rangeStart: 0, rangeEnd: 20, blocks: []*block.Meta{
					{MinTime: 0, MaxTime: 20},
					{MinTime: 10, MaxTime: 20},
				}},
				{rangeStart: 20, rangeEnd: 40, blocks: []*block.Meta{
					{MinTime: 21, MaxTime: 30},
				}},
			},
		},
		"blocks with different time ranges but all fitting within the input range": {
			timeRange: 40,
			blocks: []*block.Meta{
				{MinTime: 0, MaxTime: 20},
				{MinTime: 0, MaxTime: 40},
				{MinTime: 10, MaxTime: 20},
				{MinTime: 20, MaxTime: 30},
			},
			expected: []blocksGroup{
				{rangeStart: 0, rangeEnd: 40, blocks: []*block.Meta{
					{MinTime: 0, MaxTime: 20},
					{MinTime: 0, MaxTime: 40},
					{MinTime: 10, MaxTime: 20},
					{MinTime: 20, MaxTime: 30},
				}},
			},
		},
		"2 different range": {
			timeRange: 4 * blockRange,
			blocks: []*block.Meta{
				{MinTime: model.Time(blockRange), MaxTime: model.Time(2 * blockRange)},
				{MinTime: model.Time(blockRange), MaxTime: model.Time(2 * blockRange)},
				{MinTime: model.Time(4*blockRange) + 1, MaxTime: model.Time(5 * blockRange)},
				{MinTime: model.Time(7 * blockRange), MaxTime: model.Time(8 * blockRange)},
			},
			expected: []blocksGroup{
				{
					rangeStart: 0, rangeEnd: 4 * blockRange,
					blocks: []*block.Meta{
						{MinTime: model.Time(blockRange), MaxTime: model.Time(2 * blockRange)},
						{MinTime: model.Time(blockRange), MaxTime: model.Time(2 * blockRange)},
					},
				},
				{
					rangeStart: 4 * blockRange, rangeEnd: 8 * blockRange,
					blocks: []*block.Meta{
						{MinTime: model.Time(4*blockRange) + 1, MaxTime: model.Time(5 * blockRange)},
						{MinTime: model.Time(7 * blockRange), MaxTime: model.Time(8 * blockRange)},
					},
				},
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			actual := groupBlocksByRange(testData.blocks, testData.timeRange)
			assert.Equal(t, testData.expected, actual)
		})
	}
}
