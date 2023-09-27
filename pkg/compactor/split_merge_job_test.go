// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/split_merge_job_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"encoding/json"
	"testing"

	"github.com/oklog/ulid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	sharding "github.com/grafana/pyroscope/pkg/phlaredb/sharding"
)

func TestJob_conflicts(t *testing.T) {
	block1 := &block.Meta{ULID: ulid.MustNew(1, nil)}
	block2 := &block.Meta{ULID: ulid.MustNew(2, nil)}
	block3 := &block.Meta{ULID: ulid.MustNew(3, nil)}
	block4 := &block.Meta{ULID: ulid.MustNew(4, nil)}

	copyMeta := func(meta *block.Meta) *block.Meta {
		encoded, err := json.Marshal(meta)
		require.NoError(t, err)

		decoded := block.Meta{}
		require.NoError(t, json.Unmarshal(encoded, &decoded))

		return &decoded
	}

	withShardIDLabel := func(meta *block.Meta, shardID string) *block.Meta {
		meta = copyMeta(meta)
		meta.Labels = map[string]string{sharding.CompactorShardIDLabel: shardID}
		return meta
	}

	tests := map[string]struct {
		first    *job
		second   *job
		expected bool
	}{
		"should conflict if jobs compact different blocks but with overlapping time ranges and same shard": {
			first: &job{
				stage:   stageMerge,
				shardID: "1_of_2",
				blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks:     []*block.Meta{withShardIDLabel(block1, "1_of_2"), withShardIDLabel(block2, "1_of_2")},
				},
			},
			second: &job{
				stage:   stageMerge,
				shardID: "1_of_2",
				blocksGroup: blocksGroup{
					rangeStart: 15,
					rangeEnd:   25,
					blocks:     []*block.Meta{withShardIDLabel(block3, "1_of_2"), withShardIDLabel(block4, "1_of_2")},
				},
			},
			expected: true,
		},
		"should NOT conflict if jobs compact different blocks with non-overlapping time ranges and same shard": {
			first: &job{
				stage:   stageMerge,
				shardID: "1_of_2",
				blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks:     []*block.Meta{withShardIDLabel(block1, "1_of_2"), withShardIDLabel(block2, "1_of_2")},
				},
			},
			second: &job{
				stage:   stageMerge,
				shardID: "1_of_2",
				blocksGroup: blocksGroup{
					rangeStart: 21,
					rangeEnd:   30,
					blocks:     []*block.Meta{withShardIDLabel(block3, "1_of_2"), withShardIDLabel(block4, "1_of_2")},
				},
			},
			expected: false,
		},
		"should NOT conflict if jobs compact same blocks with overlapping time ranges but different shard": {
			first: &job{
				stage:   stageMerge,
				shardID: "1_of_2",
				blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks:     []*block.Meta{withShardIDLabel(block1, "1_of_2"), withShardIDLabel(block2, "1_of_2")},
				},
			},
			second: &job{
				stage:   stageMerge,
				shardID: "2_of_2",
				blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks:     []*block.Meta{withShardIDLabel(block1, "2_of_2"), withShardIDLabel(block2, "2_of_2")},
				},
			},
			expected: false,
		},
		"should conflict if jobs compact same blocks with overlapping time ranges and different shard but at a different stage": {
			first: &job{
				stage:   stageSplit,
				shardID: "1_of_2",
				blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks:     []*block.Meta{withShardIDLabel(block1, "1_of_2"), withShardIDLabel(block2, "1_of_2")},
				},
			},
			second: &job{
				stage:   stageMerge,
				shardID: "2_of_2",
				blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks:     []*block.Meta{withShardIDLabel(block1, "2_of_2"), withShardIDLabel(block2, "2_of_2")},
				},
			},
			expected: true,
		},
		"should conflict between split and merge jobs with overlapping time ranges": {
			first: &job{
				stage:   stageSplit,
				shardID: "",
				blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks:     []*block.Meta{block1, block2},
				},
			},
			second: &job{
				stage:   stageMerge,
				shardID: "1_of_2",
				blocksGroup: blocksGroup{
					rangeStart: 0,
					rangeEnd:   40,
					blocks:     []*block.Meta{withShardIDLabel(block3, "1_of_2"), withShardIDLabel(block4, "1_of_2")},
				},
			},
			expected: true,
		},
		"should NOT conflict between split and merge jobs with non-overlapping time ranges": {
			first: &job{
				stage:   stageSplit,
				shardID: "",
				blocksGroup: blocksGroup{
					rangeStart: 10,
					rangeEnd:   20,
					blocks:     []*block.Meta{block1, block2},
				},
			},
			second: &job{
				stage:   stageMerge,
				shardID: "1_of_2",
				blocksGroup: blocksGroup{
					rangeStart: 21,
					rangeEnd:   40,
					blocks:     []*block.Meta{withShardIDLabel(block3, "1_of_2"), withShardIDLabel(block4, "1_of_2")},
				},
			},
			expected: false,
		},
	}

	for testName, testCase := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.Equal(t, testCase.expected, testCase.first.conflicts(testCase.second))
			assert.Equal(t, testCase.expected, testCase.second.conflicts(testCase.first))
		})
	}
}

func TestBlocksGroup_overlaps(t *testing.T) {
	tests := []struct {
		first    blocksGroup
		second   blocksGroup
		expected bool
	}{
		{
			first:    blocksGroup{rangeStart: 10, rangeEnd: 20},
			second:   blocksGroup{rangeStart: 20, rangeEnd: 30},
			expected: false,
		}, {
			first:    blocksGroup{rangeStart: 10, rangeEnd: 20},
			second:   blocksGroup{rangeStart: 21, rangeEnd: 30},
			expected: false,
		}, {
			first:    blocksGroup{rangeStart: 10, rangeEnd: 20},
			second:   blocksGroup{rangeStart: 19, rangeEnd: 30},
			expected: true,
		}, {
			first:    blocksGroup{rangeStart: 10, rangeEnd: 21},
			second:   blocksGroup{rangeStart: 20, rangeEnd: 30},
			expected: true,
		}, {
			first:    blocksGroup{rangeStart: 10, rangeEnd: 20},
			second:   blocksGroup{rangeStart: 12, rangeEnd: 18},
			expected: true,
		},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.expected, tc.first.overlaps(tc.second))
		assert.Equal(t, tc.expected, tc.second.overlaps(tc.first))
	}
}

func TestBlocksGroup_getNonShardedBlocks(t *testing.T) {
	block1 := ulid.MustNew(1, nil)
	block2 := ulid.MustNew(2, nil)
	block3 := ulid.MustNew(3, nil)

	tests := map[string]struct {
		input    blocksGroup
		expected []*block.Meta
	}{
		"should return nil if the group is empty": {
			input:    blocksGroup{},
			expected: nil,
		},
		"should return nil if the group contains only sharded blocks": {
			input: blocksGroup{blocks: []*block.Meta{
				{ULID: block1, Labels: map[string]string{sharding.CompactorShardIDLabel: "1"}},
				{ULID: block2, Labels: map[string]string{sharding.CompactorShardIDLabel: "1"}},
			}},
			expected: nil,
		},
		"should return the list of non-sharded blocks if exist in the group": {
			input: blocksGroup{blocks: []*block.Meta{
				{ULID: block1},
				{ULID: block2, Labels: map[string]string{sharding.CompactorShardIDLabel: "1"}},
				{ULID: block3, Labels: map[string]string{"key": "value"}},
			}},
			expected: []*block.Meta{
				{ULID: block1},
				{ULID: block3, Labels: map[string]string{"key": "value"}},
			},
		},
		"should consider non-sharded a block with the shard ID label but empty value": {
			input: blocksGroup{blocks: []*block.Meta{
				{ULID: block1, Labels: map[string]string{sharding.CompactorShardIDLabel: ""}},
				{ULID: block2, Labels: map[string]string{sharding.CompactorShardIDLabel: "1"}},
				{ULID: block3, Labels: map[string]string{"key": "value"}},
			}},
			expected: []*block.Meta{
				{ULID: block1, Labels: map[string]string{sharding.CompactorShardIDLabel: ""}},
				{ULID: block3, Labels: map[string]string{"key": "value"}},
			},
		},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.expected, tc.input.getNonShardedBlocks())
	}
}
