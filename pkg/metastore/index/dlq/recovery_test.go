package dlq

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/v2/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockdlq"
)

func TestRecoverTick(t *testing.T) {
	metas := []*metastorev1.BlockMeta{
		{
			Id:    test.ULID("2024-09-23T03:00:00Z"),
			Shard: 2,
		},
		{
			Id:    test.ULID("2024-09-23T01:00:00Z"),
			Shard: 1,
		},
		{
			Id:    test.ULID("2024-09-23T02:00:00Z"),
			Shard: 2,
		},
	}

	var actual []*metastorev1.BlockMeta
	srv := mockdlq.NewMockMetastore(t)
	srv.On("AddRecoveredBlock", mock.Anything, mock.Anything).
		Times(3).
		Run(func(args mock.Arguments) {
			meta := args.Get(1).(*metastorev1.AddBlockRequest).Block
			actual = append(actual, meta)
		}).
		Return(&metastorev1.AddBlockResponse{}, nil)

	bucket := memory.NewInMemBucket()
	for _, meta := range metas {
		addMeta(bucket, meta)
	}

	r := NewRecovery(test.NewTestingLogger(t), Config{}, srv, bucket, prometheus.NewRegistry())
	r.recoverTick(context.Background())

	expected := []*metastorev1.BlockMeta{
		metas[1],
		metas[2],
		metas[0],
	}

	require.Equal(t, len(actual), len(expected))
	for i := range actual {
		require.Equal(t, actual[i].Id, expected[i].Id)
		require.Equal(t, actual[i].Shard, expected[i].Shard)
	}

	assert.Equal(t, 3.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("success")))
	assert.Equal(t, 0.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("unmarshal_error")))
	assert.Equal(t, 0.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("invalid_metadata")))
}

func TestNotRaftLeader(t *testing.T) {
	metas := []*metastorev1.BlockMeta{
		{
			Id:    test.ULID("2024-09-23T01:00:00Z"),
			Shard: 2,
		},
	}

	srv := mockdlq.NewMockMetastore(t)
	s, _ := status.New(codes.Unavailable, "mock metastore error").WithDetails(&raftnodepb.RaftNode{
		Id:      "foo",
		Address: "bar",
	})
	srv.On("AddRecoveredBlock", mock.Anything, mock.Anything).
		Once().
		Return(nil, s.Err())

	bucket := memory.NewInMemBucket()
	for _, meta := range metas {
		addMeta(bucket, meta)
	}

	r := NewRecovery(test.NewTestingLogger(t), Config{}, srv, bucket, prometheus.NewRegistry())
	r.recoverTick(context.Background())

	assert.Equal(t, 1, len(bucket.Objects()))

	assert.Equal(t, 1.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("metastore_error")))
	assert.Equal(t, 0.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("success")))
}

func TestStartStop(t *testing.T) {
	metas := []*metastorev1.BlockMeta{
		{
			Id:    test.ULID("2024-09-23T03:00:00Z"),
			Shard: 2,
		},
		{
			Id:    test.ULID("2024-09-23T01:00:00Z"),
			Shard: 1,
		},
		{
			Id:    test.ULID("2024-09-23T02:00:00Z"),
			Shard: 2,
		},
	}
	m := new(sync.Mutex)

	var actual []*metastorev1.BlockMeta
	srv := mockdlq.NewMockMetastore(t)
	srv.On("AddRecoveredBlock", mock.Anything, mock.Anything).
		Times(3).
		Run(func(args mock.Arguments) {
			meta := args.Get(1).(*metastorev1.AddBlockRequest).Block
			m.Lock()
			actual = append(actual, meta)
			m.Unlock()
		}).
		Return(&metastorev1.AddBlockResponse{}, nil)

	bucket := memory.NewInMemBucket()
	for _, meta := range metas {
		addMeta(bucket, meta)
	}

	r := NewRecovery(test.NewTestingLogger(t), Config{CheckInterval: time.Millisecond * 10}, srv, bucket, prometheus.NewRegistry())
	r.Start()
	defer r.Stop()

	require.Eventually(t, func() bool {
		m.Lock()
		defer m.Unlock()
		return len(actual) == 3
	}, time.Second, time.Millisecond*100)

	expected := []*metastorev1.BlockMeta{
		metas[1],
		metas[2],
		metas[0],
	}

	require.Equal(t, len(actual), len(expected))
	for i := range actual {
		require.Equal(t, actual[i].Id, expected[i].Id)
		require.Equal(t, actual[i].Shard, expected[i].Shard)
	}

	assert.Equal(t, 3.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("success")))
}

func addMeta(bucket *memory.InMemBucket, meta *metastorev1.BlockMeta) {
	data, _ := meta.MarshalVT()
	bucket.Set(block.MetadataDLQObjectPath(meta), data)
}

// s3LikeBucket wraps the in-memory bucket to reproduce the behaviour of
// object stores such as S3, where reading a zero-byte object surfaces io.EOF
// on the first read (via the getRange prefetch) rather than an empty reader.
type s3LikeBucket struct {
	*memory.InMemBucket
}

func (b *s3LikeBucket) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	rc, err := b.InMemBucket.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, io.EOF
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// TestRecoverTick_StrayPlaceholderDoesNotBlock verifies that a stray, zero-byte
// "folder placeholder" object (e.g. "dlq/1/") that sorts before valid entries
// does not abort the recovery sweep: the valid metas are still recovered.
func TestRecoverTick_StrayPlaceholderDoesNotBlock(t *testing.T) {
	metas := []*metastorev1.BlockMeta{
		{Id: test.ULID("2024-09-23T01:00:00Z"), Shard: 1},
		{Id: test.ULID("2024-09-23T02:00:00Z"), Shard: 2},
	}

	var recovered int
	srv := mockdlq.NewMockMetastore(t)
	srv.On("AddRecoveredBlock", mock.Anything, mock.Anything).
		Times(2).
		Run(func(mock.Arguments) { recovered++ }).
		Return(&metastorev1.AddBlockResponse{}, nil)

	bucket := &s3LikeBucket{InMemBucket: memory.NewInMemBucket()}
	// Stray zero-byte placeholder that sorts before the valid meta.pb entries.
	bucket.Set(block.DirNameDLQ+"/1/", nil)
	for _, meta := range metas {
		data, _ := meta.MarshalVT()
		bucket.Set(block.MetadataDLQObjectPath(meta), data)
	}

	r := NewRecovery(test.NewTestingLogger(t), Config{}, srv, bucket, prometheus.NewRegistry())
	r.recoverTick(context.Background())

	assert.Equal(t, 2, recovered)
	assert.Equal(t, 2.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("success")))
	assert.Equal(t, 1.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("stray")))

	// The stray placeholder is skipped, not deleted; the recovered entries are removed.
	_, ok := bucket.Objects()[block.DirNameDLQ+"/1/"]
	assert.True(t, ok, "stray placeholder should not be deleted")
	assert.Equal(t, 1, len(bucket.Objects()))
}

// TestRecoverTick_EmptyMetaDeletedAndContinues verifies that an empty
// (zero-byte) meta.pb object is treated as permanently invalid: it is deleted,
// counted, and does not prevent recovery of the remaining entries.
func TestRecoverTick_EmptyMetaDeletedAndContinues(t *testing.T) {
	valid := &metastorev1.BlockMeta{Id: test.ULID("2024-09-23T05:00:00Z"), Shard: 2}
	empty := &metastorev1.BlockMeta{Id: test.ULID("2024-09-23T02:00:00Z"), Shard: 1}

	var recovered int
	srv := mockdlq.NewMockMetastore(t)
	srv.On("AddRecoveredBlock", mock.Anything, mock.Anything).
		Once().
		Run(func(mock.Arguments) { recovered++ }).
		Return(&metastorev1.AddBlockResponse{}, nil)

	bucket := &s3LikeBucket{InMemBucket: memory.NewInMemBucket()}
	emptyPath := block.MetadataDLQObjectPath(empty)
	bucket.Set(emptyPath, nil) // zero-byte meta.pb -> EOF on read
	validData, _ := valid.MarshalVT()
	bucket.Set(block.MetadataDLQObjectPath(valid), validData)

	r := NewRecovery(test.NewTestingLogger(t), Config{}, srv, bucket, prometheus.NewRegistry())
	r.recoverTick(context.Background())

	assert.Equal(t, 1, recovered)
	assert.Equal(t, 1.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("success")))
	assert.Equal(t, 1.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("empty")))

	// The empty object is deleted; nothing left in the DLQ.
	_, ok := bucket.Objects()[emptyPath]
	assert.False(t, ok, "empty meta.pb should be deleted")
	assert.Equal(t, 0, len(bucket.Objects()))
}

// TestRecoverTick_TransientErrorRetainsAndContinues verifies that a transient
// metastore error on one object retains that object (for a later retry) while
// still allowing sibling entries to be recovered in the same sweep.
func TestRecoverTick_TransientErrorRetainsAndContinues(t *testing.T) {
	failing := &metastorev1.BlockMeta{Id: test.ULID("2024-09-23T01:00:00Z"), Shard: 1}
	ok := &metastorev1.BlockMeta{Id: test.ULID("2024-09-23T02:00:00Z"), Shard: 2}

	failingPath := block.MetadataDLQObjectPath(failing)
	okPath := block.MetadataDLQObjectPath(ok)

	var recovered int
	srv := mockdlq.NewMockMetastore(t)
	srv.On("AddRecoveredBlock", mock.Anything, mock.Anything).
		Return(func(_ context.Context, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
			if req.Block.Id == failing.Id {
				return nil, errors.New("transient metastore error")
			}
			recovered++
			return &metastorev1.AddBlockResponse{}, nil
		})

	bucket := &s3LikeBucket{InMemBucket: memory.NewInMemBucket()}
	failingData, _ := failing.MarshalVT()
	bucket.Set(failingPath, failingData)
	okData, _ := ok.MarshalVT()
	bucket.Set(okPath, okData)

	r := NewRecovery(test.NewTestingLogger(t), Config{}, srv, bucket, prometheus.NewRegistry())
	r.recoverTick(context.Background())

	assert.Equal(t, 1, recovered)
	assert.Equal(t, 1.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("success")))
	assert.Equal(t, 1.0, testutil.ToFloat64(r.metrics.recoveryAttempts.WithLabelValues("metastore_error")))

	// The failing object is retained for retry; the good one is deleted.
	objs := bucket.Objects()
	_, failingKept := objs[failingPath]
	_, okDeleted := objs[okPath]
	assert.True(t, failingKept, "failing object should be retained for retry")
	assert.False(t, okDeleted, "successfully recovered object should be deleted")
}

var _ objstore.Bucket = (*s3LikeBucket)(nil)

func TestIsDLQMetadataPath(t *testing.T) {
	// A real path produced by the writer must be accepted.
	level0 := block.MetadataDLQObjectPath(&metastorev1.BlockMeta{
		Id:    test.ULID("2024-09-23T01:00:00Z"),
		Shard: 1,
	})
	compacted := block.MetadataDLQObjectPath(&metastorev1.BlockMeta{
		Id:              test.ULID("2024-09-23T02:00:00Z"),
		Shard:           7,
		CompactionLevel: 1,
		StringTable:     []string{"", "tenant-a"},
		Tenant:          1,
	})

	cases := []struct {
		path string
		want bool
	}{
		{level0, true},
		{compacted, true},

		// Stray keys that must be rejected.
		{"dlq/", false},
		{"dlq/1/", false},
		{"dlq/1/anonymous/", false},
		{"dlq", false},
		{"dlq/1/anonymous/01J8E68YM0Z2RRKCY9SYGX3054", false}, // missing meta.pb
		{"dlq/1/anonymous/01J8E68YM0Z2RRKCY9SYGX3054/meta.pb/extra", false},
		{"dlq/1/anonymous/not-a-ulid/meta.pb", false},                    // bad block id
		{"dlq/1/anonymous/01J8E68YM0Z2RRKCY9SYGX3054/x.pb", false},       // wrong filename
		{"blocks/1/anonymous/01J8E68YM0Z2RRKCY9SYGX3054/meta.pb", false}, // wrong prefix
		{"dlq//01J8E68YM0Z2RRKCY9SYGX3054/anonymous/meta.pb", false},     // empty shard
		{"dlq/1//01J8E68YM0Z2RRKCY9SYGX3054/meta.pb", false},             // empty tenant
		{"dlq/1/anonymous/meta.pb", false},                               // too few parts
	}

	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			assert.Equal(t, c.want, isDLQMetadataPath(c.path))
		})
	}
}
