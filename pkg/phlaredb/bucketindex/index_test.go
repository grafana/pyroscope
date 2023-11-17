// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/bucketindex/index_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package bucketindex

import (
	"testing"

	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/sharding"
)

func TestIndex_RemoveBlock(t *testing.T) {
	block1 := ulid.MustNew(1, nil)
	block2 := ulid.MustNew(2, nil)
	block3 := ulid.MustNew(3, nil)
	idx := &Index{
		Blocks:             Blocks{{ID: block1}, {ID: block2}, {ID: block3}},
		BlockDeletionMarks: BlockDeletionMarks{{ID: block2}, {ID: block3}},
	}

	idx.RemoveBlock(block2)
	assert.ElementsMatch(t, []ulid.ULID{block1, block3}, idx.Blocks.GetULIDs())
	assert.ElementsMatch(t, []ulid.ULID{block3}, idx.BlockDeletionMarks.GetULIDs())
}

func TestBlockFromMeta(t *testing.T) {
	blockID := ulid.MustNew(1, nil)

	tests := map[string]struct {
		meta     block.Meta
		expected Block
	}{
		"meta.json": {
			meta: block.Meta{
				ULID:    blockID,
				MinTime: model.Time(10),
				MaxTime: model.Time(20),
				Labels: map[string]string{
					sharding.CompactorShardIDLabel: "1_of_8",
				},
			},
			expected: Block{
				ID:               blockID,
				MinTime:          model.Time(10),
				MaxTime:          model.Time(20),
				CompactorShardID: "1_of_8",
				CompactionLevel:  0,
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.Equal(t, testData.expected, *BlockFromMeta(testData.meta))
		})
	}
}

func TestBlock_Within(t *testing.T) {
	tests := []struct {
		block    *Block
		minT     int64
		maxT     int64
		expected bool
	}{
		{
			block:    &Block{MinTime: 10, MaxTime: 20},
			minT:     5,
			maxT:     9,
			expected: false,
		}, {
			block:    &Block{MinTime: 10, MaxTime: 20},
			minT:     5,
			maxT:     10,
			expected: true,
		}, {
			block:    &Block{MinTime: 10, MaxTime: 20},
			minT:     5,
			maxT:     10,
			expected: true,
		}, {
			block:    &Block{MinTime: 10, MaxTime: 20},
			minT:     11,
			maxT:     13,
			expected: true,
		}, {
			block:    &Block{MinTime: 10, MaxTime: 20},
			minT:     19,
			maxT:     21,
			expected: true,
		}, {
			block:    &Block{MinTime: 10, MaxTime: 20},
			minT:     20,
			maxT:     21,
			expected: true,
		},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.expected, tc.block.Within(model.Time(tc.minT), model.Time(tc.maxT)))
	}
}

func TestBlockDeletionMark_DeletionMark(t *testing.T) {
	block1 := ulid.MustNew(1, nil)
	mark := &BlockDeletionMark{ID: block1, DeletionTime: 1}

	assert.Equal(t, &block.DeletionMark{
		ID:           block1,
		Version:      block.DeletionMarkVersion1,
		DeletionTime: 1,
	}, mark.BlockDeletionMark())
}

func TestBlockDeletionMarks_Clone(t *testing.T) {
	block1 := ulid.MustNew(1, nil)
	block2 := ulid.MustNew(2, nil)
	orig := BlockDeletionMarks{{ID: block1, DeletionTime: 1}, {ID: block2, DeletionTime: 2}}

	// The clone must be identical.
	clone := orig.Clone()
	assert.Equal(t, orig, clone)

	// Changes to the original shouldn't be reflected to the clone.
	orig[0].DeletionTime = -1
	assert.Equal(t, int64(1), clone[0].DeletionTime)
}
