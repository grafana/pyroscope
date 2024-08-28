// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/bucketindex/index.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package bucketindex

import (
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/common/model"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/sharding"
	"github.com/grafana/pyroscope/pkg/util"
)

const (
	IndexFilename           = "bucket-index.json"
	IndexCompressedFilename = IndexFilename + ".gz"
	IndexVersion1           = 1
	IndexVersion2           = 2 // Added CompactorShardID field.
	IndexVersion3           = 3 // Added CompactionLevel field.
)

// Index contains all known blocks and markers of a tenant.
type Index struct {
	// Version of the index format.
	Version int `json:"version"`

	// List of complete blocks (partial blocks are excluded from the index).
	Blocks Blocks `json:"blocks"`

	// List of block deletion marks.
	BlockDeletionMarks BlockDeletionMarks `json:"block_deletion_marks"`

	// UpdatedAt is a unix timestamp (seconds precision) of when the index has been updated
	// (written in the storage) the last time.
	UpdatedAt int64 `json:"updated_at"`
}

func (idx *Index) GetUpdatedAt() time.Time {
	return time.Unix(idx.UpdatedAt, 0)
}

// RemoveBlock removes block and its deletion mark (if any) from index.
func (idx *Index) RemoveBlock(id ulid.ULID) {
	for i := 0; i < len(idx.Blocks); i++ {
		if idx.Blocks[i].ID == id {
			idx.Blocks = append(idx.Blocks[:i], idx.Blocks[i+1:]...)
			break
		}
	}

	for i := 0; i < len(idx.BlockDeletionMarks); i++ {
		if idx.BlockDeletionMarks[i].ID == id {
			idx.BlockDeletionMarks[i] = nil
			idx.BlockDeletionMarks = append(idx.BlockDeletionMarks[:i], idx.BlockDeletionMarks[i+1:]...)
			break
		}
	}
}

// Block holds the information about a block in the index.
type Block struct {
	// Block ID.
	ID ulid.ULID `json:"block_id"`

	// MinTime and MaxTime specify the time range all samples in the block are in (millis precision).
	MinTime model.Time `json:"min_time"`
	MaxTime model.Time `json:"max_time"`

	// UploadedAt is a unix timestamp (seconds precision) of when the block has been completed to be uploaded
	// to the storage.
	UploadedAt int64 `json:"uploaded_at"`

	// Block's compactor shard ID, copied from tsdb.CompactorShardIDExternalLabel label.
	CompactorShardID string `json:"compactor_shard_id,omitempty"`
	CompactionLevel  int    `json:"compaction_level,omitempty"`
}

// Within returns whether the block contains samples within the provided range.
// Input minT and maxT are both inclusive.
func (m *Block) Within(minT, maxT model.Time) bool {
	return block.InRange(m.MinTime, m.MaxTime, minT, maxT)
}

func (m *Block) GetUploadedAt() time.Time {
	return time.Unix(m.UploadedAt, 0)
}

func (m *Block) String() string {
	minT := util.TimeFromMillis(int64(m.MinTime)).UTC()
	maxT := util.TimeFromMillis(int64(m.MaxTime)).UTC()

	shard := m.CompactorShardID
	if shard == "" {
		shard = "none"
	}

	return fmt.Sprintf("%s (min time: %s max time: %s, compactor shard: %s)", m.ID, minT.String(), maxT.String(), shard)
}

// Meta returns a block meta based on the known information in the index.
// The returned meta doesn't include all original meta.json data but only a subset
// of it.
func (m *Block) Meta() *block.Meta {
	return &block.Meta{
		ULID:    m.ID,
		MinTime: m.MinTime,
		MaxTime: m.MaxTime,
		Labels: map[string]string{
			sharding.CompactorShardIDLabel: m.CompactorShardID,
		},
		Compaction: block.BlockMetaCompaction{
			Level: m.CompactionLevel,
		},
	}
}

func BlockFromMeta(meta block.Meta) *Block {
	return &Block{
		ID:               meta.ULID,
		MinTime:          meta.MinTime,
		MaxTime:          meta.MaxTime,
		CompactorShardID: meta.Labels[sharding.CompactorShardIDLabel],
		CompactionLevel:  meta.Compaction.Level,
	}
}

// BlockDeletionMark holds the information about a block's deletion mark in the index.
type BlockDeletionMark struct {
	// Block ID.
	ID ulid.ULID `json:"block_id"`

	// DeletionTime is a unix timestamp (seconds precision) of when the block was marked to be deleted.
	DeletionTime int64 `json:"deletion_time"`
}

func (m *BlockDeletionMark) GetDeletionTime() time.Time {
	return time.Unix(m.DeletionTime, 0)
}

// BlockDeletionMark returns the block deletion mark.
func (m *BlockDeletionMark) BlockDeletionMark() *block.DeletionMark {
	return &block.DeletionMark{
		ID:           m.ID,
		Version:      block.DeletionMarkVersion1,
		DeletionTime: m.DeletionTime,
	}
}

func DeletionMarkFromBlockMarker(mark *block.DeletionMark) *BlockDeletionMark {
	return &BlockDeletionMark{
		ID:           mark.ID,
		DeletionTime: mark.DeletionTime,
	}
}

// BlockDeletionMarks holds a set of block deletion marks in the index. No ordering guaranteed.
type BlockDeletionMarks []*BlockDeletionMark

func (s BlockDeletionMarks) GetULIDs() []ulid.ULID {
	ids := make([]ulid.ULID, len(s))
	for i, m := range s {
		ids[i] = m.ID
	}
	return ids
}

func (s BlockDeletionMarks) Clone() BlockDeletionMarks {
	clone := make(BlockDeletionMarks, len(s))
	for i, m := range s {
		v := *m
		clone[i] = &v
	}
	return clone
}

// Blocks holds a set of blocks in the index. No ordering guaranteed.
type Blocks []*Block

func (s Blocks) GetULIDs() []ulid.ULID {
	ids := make([]ulid.ULID, len(s))
	for i, m := range s {
		ids[i] = m.ID
	}
	return ids
}

func (s Blocks) String() string {
	b := strings.Builder{}

	for idx, m := range s {
		if idx > 0 {
			b.WriteString(", ")
		}
		b.WriteString(m.String())
	}

	return b.String()
}
