package compactionworker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockobjstore"
)

func TestWorker_DeleteAllDatasets(t *testing.T) {
	bucket := mockobjstore.NewMockBucket(t)
	client := &MetastoreClientMock{
		MockCompactionServiceClient: mockmetastorev1.NewMockCompactionServiceClient(t),
	}
	id := test.ULID("2024-01-01T10:00:00Z")
	b := &metastorev1.BlockMeta{Id: id, Tenant: 1, Shard: 1, CompactionLevel: 2,
		StringTable: []string{"", "tenant-a", "svc-a", "svc-b"},
		Datasets:    []*metastorev1.Dataset{{Tenant: 1, Name: 2}, {Tenant: 1, Name: 3}},
	}
	// Run the real compactor. The mock bucket only serves the metadata read:
	// a full deletion must neither read the old object nor upload a replacement.
	expectBlockObjects(t, bucket, b)
	w := createTestWorker(t, client, block.Compact, bucket)
	job := &compactionJob{ctx: context.Background(), CompactionJob: &metastorev1.CompactionJob{
		Name: "delete-all", Tenant: "tenant-a", Shard: 1, CompactionLevel: 2, SourceBlocks: []string{id},
		DeleteFilter: &metastorev1.DeleteFilter{ServiceNameRegex: "svc-a|svc-b"},
	}}
	w.runCompaction(job)
	require.NotNil(t, job.compacted)
	require.Empty(t, job.compacted.NewBlocks)
	require.Equal(t, []string{id}, job.compacted.SourceBlocks.Blocks)
	require.Equal(t, "tenant-a", job.compacted.SourceBlocks.Tenant)
	require.EqualValues(t, 1, job.compacted.SourceBlocks.Shard)
}
