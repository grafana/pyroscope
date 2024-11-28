package dlq

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockdlq"
)

func TestRecoverTick(t *testing.T) {
	metas := []*metastorev1.BlockMeta{
		{
			Id:          1,
			Shard:       2,
			StringTable: []string{"", "3"},
		},
		{
			Id:          1,
			Shard:       1,
			StringTable: []string{"", "1"},
		},
		{
			Id:          1,
			Shard:       2,
			StringTable: []string{"", "2"},
		},
	}

	var actual []*metastorev1.BlockMeta
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
		require.Equal(t, block.ID(actual[i]), block.ID(expected[i]))
		require.Equal(t, actual[i].Shard, expected[i].Shard)
	}
}

func TestNotRaftLeader(t *testing.T) {
	metas := []*metastorev1.BlockMeta{
		{
			Id:          1,
			Shard:       2,
			StringTable: []string{"", "1"},
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
			Id:          1,
			Shard:       2,
			StringTable: []string{"", "3"},
		},
		{
			Id:          1,
			Shard:       1,
			StringTable: []string{"", "1"},
		},
		{
			Id:          1,
			Shard:       2,
			StringTable: []string{"", "2"},
		},
	}
	m := new(sync.Mutex)

	var actual []*metastorev1.BlockMeta
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
		require.Equal(t, block.ID(actual[i]), block.ID(expected[i]))
		require.Equal(t, actual[i].Shard, expected[i].Shard)
	}
}

func addMeta(bucket *memory.InMemBucket, meta *metastorev1.BlockMeta) {
	data, _ := meta.MarshalVT()
	bucket.Set(block.MetadataDLQObjectPath(meta), data)
}
