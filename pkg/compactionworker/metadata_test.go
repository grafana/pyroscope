package compactionworker

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	thanosstore "github.com/thanos-io/objstore"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/block/metadata"
	"github.com/grafana/pyroscope/v2/pkg/objstore"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockmetastorev1"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockobjstore"
)

// newStorageMetadataWorker creates a worker that reads block metadata from
// object storage. The metastore mocks carry no expectations: any metastore
// call on the metadata path fails the test.
func newStorageMetadataWorker(t *testing.T, bucket objstore.Bucket) *Worker {
	client := &MetastoreClientMock{
		MockCompactionServiceClient: mockmetastorev1.NewMockCompactionServiceClient(t),
		MockIndexServiceClient:      mockmetastorev1.NewMockIndexServiceClient(t),
	}
	w := createTestWorker(t, client, nil, bucket)
	w.config.MetadataSource = MetadataSourceObjectStorage
	return w
}

func testBlockMeta(id string, shard uint32, level uint32, tenant string) *metastorev1.BlockMeta {
	st := metadata.NewStringTable()
	md := &metastorev1.BlockMeta{
		FormatVersion:   1,
		Id:              id,
		Shard:           shard,
		CompactionLevel: level,
		Tenant:          st.Put(tenant),
		CreatedBy:       st.Put("segment-writer-1"),
		MinTime:         1,
		MaxTime:         2,
	}
	md.StringTable = st.Strings
	return md
}

// writeBlockObject uploads an object of the block layout relevant here:
// arbitrary data followed by the footer-encoded metadata.
func writeBlockObject(t *testing.T, bucket objstore.Bucket, md *metastorev1.BlockMeta) string {
	t.Helper()
	var body bytes.Buffer
	body.WriteString(strings.Repeat("x", 64))
	md.MetadataOffset = uint64(body.Len())
	require.NoError(t, metadata.Encode(&body, md))
	path := block.ObjectPath(md)
	require.NoError(t, bucket.Upload(context.Background(), path, bytes.NewReader(body.Bytes())))
	return path
}

func testCompactionJob(tenant string, shard uint32, level uint32, blocks ...string) *compactionJob {
	return &compactionJob{
		CompactionJob: &metastorev1.CompactionJob{
			Name:            "test-job",
			Tenant:          tenant,
			Shard:           shard,
			CompactionLevel: level,
			SourceBlocks:    blocks,
		},
		ctx: context.Background(),
	}
}

func TestWorker_MetadataFromStorage(t *testing.T) {
	bucket := objstore.NewBucket(thanosstore.NewInMemBucket())
	ids := []string{
		"01J000000000000000000000A1",
		"01J000000000000000000000A2",
		"01J000000000000000000000A3",
	}
	for _, id := range ids {
		writeBlockObject(t, bucket, testBlockMeta(id, 3, 1, "tenant-a"))
	}

	w := newStorageMetadataWorker(t, bucket)
	job := testCompactionJob("tenant-a", 3, 1, ids...)
	require.NoError(t, w.getBlockMetadata(log.NewNopLogger(), job))

	require.Len(t, job.blocks, 3)
	for i, md := range job.blocks {
		require.Equal(t, ids[i], md.Id)
		require.Equal(t, "tenant-a", metadata.Tenant(md))
		require.NotZero(t, md.Size, "metadata read from storage must carry the object size")
	}
	require.Equal(t, ids, job.SourceBlocks)
}

func TestWorker_MetadataFromStorage_MissingBlocksDropped(t *testing.T) {
	bucket := objstore.NewBucket(thanosstore.NewInMemBucket())
	present := []string{
		"01J000000000000000000000B1",
		"01J000000000000000000000B3",
	}
	for _, id := range present {
		writeBlockObject(t, bucket, testBlockMeta(id, 1, 2, "tenant-a"))
	}

	w := newStorageMetadataWorker(t, bucket)
	job := testCompactionJob("tenant-a", 1, 2,
		"01J000000000000000000000B1",
		"01J000000000000000000000B2", // no such object
		"01J000000000000000000000B3",
	)
	require.NoError(t, w.getBlockMetadata(log.NewNopLogger(), job))

	// The missing block is dropped and the job plan is updated,
	// mirroring the metastore lookup semantics.
	require.Equal(t, present, job.SourceBlocks)
	require.Len(t, job.blocks, 2)
}

func TestWorker_MetadataFromStorage_AllMissing(t *testing.T) {
	bucket := objstore.NewBucket(thanosstore.NewInMemBucket())
	w := newStorageMetadataWorker(t, bucket)
	job := testCompactionJob("tenant-a", 1, 1, "01J000000000000000000000C1")
	require.NoError(t, w.getBlockMetadata(log.NewNopLogger(), job))
	require.Empty(t, job.blocks)
	require.Empty(t, job.SourceBlocks)
}

func TestWorker_MetadataFromStorage_SegmentPath(t *testing.T) {
	bucket := objstore.NewBucket(thanosstore.NewInMemBucket())
	// L0 segments are multi-tenant: the block tenant is anonymous and the
	// object lives under the segment prefix regardless of the job tenant.
	md := testBlockMeta("01J000000000000000000000D1", 5, 0, "")
	path := writeBlockObject(t, bucket, md)
	require.True(t, strings.HasPrefix(path, block.DirNameSegment+"/"), path)

	w := newStorageMetadataWorker(t, bucket)
	job := testCompactionJob("", 5, 0, "01J000000000000000000000D1")
	require.NoError(t, w.getBlockMetadata(log.NewNopLogger(), job))
	require.Len(t, job.blocks, 1)
	require.Equal(t, "", metadata.Tenant(job.blocks[0]))
}

func TestWorker_MetadataFromStorage_FetchTimeout(t *testing.T) {
	bucket := mockobjstore.NewMockBucket(t)
	bucket.EXPECT().Attributes(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, _ string) (thanosstore.ObjectAttributes, error) {
			<-ctx.Done()
			return thanosstore.ObjectAttributes{}, ctx.Err()
		})
	bucket.EXPECT().IsObjNotFoundErr(mock.Anything).Return(false)

	w := newStorageMetadataWorker(t, bucket)
	w.config.MetadataFetchTimeout = 20 * time.Millisecond
	job := testCompactionJob("tenant-a", 1, 1, "01J000000000000000000000F0")

	start := time.Now()
	err := w.getBlockMetadata(log.NewNopLogger(), job)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	// The read must be bounded by MetadataFetchTimeout, not by the job context.
	require.Less(t, time.Since(start), 5*time.Second)
}

func TestWorker_MetadataFromStorage_ReadError(t *testing.T) {
	bucket := mockobjstore.NewMockBucket(t)
	bucket.EXPECT().Attributes(mock.Anything, mock.Anything).
		Return(thanosstore.ObjectAttributes{}, errors.New("boom"))
	bucket.EXPECT().IsObjNotFoundErr(mock.Anything).Return(false)

	w := newStorageMetadataWorker(t, bucket)
	job := testCompactionJob("tenant-a", 1, 1, "01J000000000000000000000E1")
	require.Error(t, w.getBlockMetadata(log.NewNopLogger(), job))
}

// TestPyroscopeInstanceHash_MetadataSourceIndependent pins the CreatedBy
// string table position across the two metadata representations. The
// pyroscope_instance label of exported metrics hashes the CreatedBy string
// table index (not the string), so the index must not depend on whether the
// metadata was read from the block footer or exported by the metastore:
// series identity would change with the metadata source otherwise.
func TestPyroscopeInstanceHash_MetadataSourceIndependent(t *testing.T) {
	// Footer representation: the segment writer interns the hostname first
	// (see segment.flushBlock), so CreatedBy lands at index 1.
	st := metadata.NewStringTable()
	footer := &metastorev1.BlockMeta{
		FormatVersion: 1,
		Id:            "01J000000000000000000000F1",
		Shard:         7,
		Tenant:        0,
		CreatedBy:     st.Put("segment-writer-1"),
	}
	footer.StringTable = st.Strings
	require.EqualValues(t, 1, footer.CreatedBy)

	// Metastore representation: blocks are stored against a shared
	// shard-level string table, where the hostname lands at an arbitrary
	// index; GetBlockMetadata exports a fresh per-block table.
	shardTable := metadata.NewStringTable()
	for _, s := range []string{"svc-a", "svc-b", "tenant-x"} {
		shardTable.Put(s)
	}
	exported := footer.CloneVT()
	shardTable.Import(exported)
	require.NotEqual(t, footer.CreatedBy, exported.CreatedBy)
	shardTable.Export(exported)

	require.Equal(t, footer.CreatedBy, exported.CreatedBy)
	require.Equal(t,
		pyroscopeInstanceHash(footer.Shard, uint32(footer.CreatedBy)),
		pyroscopeInstanceHash(exported.Shard, uint32(exported.CreatedBy)),
	)
}
