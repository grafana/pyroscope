package compaction

import (
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func TestNewBlockEntry(t *testing.T) {
	tests := []struct {
		name string
		md   *metastorev1.BlockMeta
		want BlockEntry
	}{
		{
			name: "populates size, tenant, shard, level from BlockMeta",
			md: &metastorev1.BlockMeta{
				Id:              "block-1",
				Shard:           3,
				CompactionLevel: 2,
				Size:            123456,
				Tenant:          1,
				StringTable:     []string{"", "tenant-a"},
			},
			want: BlockEntry{ID: "block-1", Tenant: "tenant-a", Shard: 3, Level: 2, Size: 123456},
		},
		{
			name: "zero size is preserved, not treated as missing",
			md:   &metastorev1.BlockMeta{Id: "block-2", Size: 0},
			want: BlockEntry{ID: "block-2", Size: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &raft.Log{Index: 7, AppendedAt: time.Unix(0, 1000)}
			got := NewBlockEntry(cmd, tt.md)
			require.Equal(t, tt.want.ID, got.ID)
			require.Equal(t, tt.want.Tenant, got.Tenant)
			require.Equal(t, tt.want.Shard, got.Shard)
			require.Equal(t, tt.want.Level, got.Level)
			require.Equal(t, tt.want.Size, got.Size)
			require.Equal(t, cmd.Index, got.Index)
			require.Equal(t, cmd.AppendedAt.UnixNano(), got.AppendedAt)
		})
	}
}
