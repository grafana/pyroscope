package dlq

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	segmentstorage "github.com/grafana/pyroscope/pkg/experiment/ingester/storage"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockdlq"
)

func TestRecoverTick(t *testing.T) {
	metas := []*metastorev1.BlockMeta{
		{
			Id:    ulid.MustNew(3, rand.Reader).String(),
			Shard: 2,
		},
		{
			Id:    ulid.MustNew(1, rand.Reader).String(),
			Shard: 1,
		},
		{
			Id:    ulid.MustNew(2, rand.Reader).String(),
			Shard: 2,
		},
	}
	actual := []*metastorev1.BlockMeta{}

	srv := mockdlq.NewMockLocalServer(t)
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

	r := NewRecovery(testutil.NewLogger(t), RecoveryConfig{}, srv, bucket)
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
}

func TestNotRaftLeader(t *testing.T) {
	metas := []*metastorev1.BlockMeta{
		{
			Id:    ulid.MustNew(3, rand.Reader).String(),
			Shard: 2,
		},
	}

	srv := mockdlq.NewMockLocalServer(t)
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

	r := NewRecovery(testutil.NewLogger(t), RecoveryConfig{}, srv, bucket)
	r.recoverTick(context.Background())

	assert.Equal(t, 1, len(bucket.Objects()))
}

func TestStartStop(t *testing.T) {
	metas := []*metastorev1.BlockMeta{
		{
			Id:    ulid.MustNew(3, rand.Reader).String(),
			Shard: 2,
		},
		{
			Id:    ulid.MustNew(1, rand.Reader).String(),
			Shard: 1,
		},
		{
			Id:    ulid.MustNew(2, rand.Reader).String(),
			Shard: 2,
		},
	}
	m := new(sync.Mutex)
	actual := []*metastorev1.BlockMeta{}

	srv := mockdlq.NewMockLocalServer(t)
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

	r := NewRecovery(testutil.NewLogger(t), RecoveryConfig{Period: time.Millisecond * 10}, srv, bucket)
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
}

func addMeta(bucket *memory.InMemBucket, meta *metastorev1.BlockMeta) {
	data, _ := meta.MarshalVT()
	bucket.Set(segmentstorage.PathForDLQ(meta), data)
}
