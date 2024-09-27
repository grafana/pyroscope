package dlq

import (
	"context"
	"crypto/rand"
	"fmt"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetastorev1"
	"github.com/oklog/ulid"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"path"
	"testing"
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

	srv := mockmetastorev1.NewMockMetastoreServiceServer(t)
	srv.On("AddBlock", mock.Anything, mock.Anything).
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

	r := NewRecovery(RecoveryConfig{}, testutil.NewLogger(t), srv, bucket)
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

	srv := mockmetastorev1.NewMockMetastoreServiceServer(t)
	s, _ := status.New(codes.Unavailable, "err").WithDetails(&typesv1.RaftDetails{Leader: string("239")})
	srv.On("AddBlock", mock.Anything, mock.Anything).
		Once().
		Return(nil, s.Err())

	bucket := memory.NewInMemBucket()
	for _, meta := range metas {
		addMeta(bucket, meta)
	}

	r := NewRecovery(RecoveryConfig{}, testutil.NewLogger(t), srv, bucket)
	r.recoverTick(context.Background())
}

func addMeta(bucket *memory.InMemBucket, meta *metastorev1.BlockMeta) {
	data, _ := meta.MarshalVT()
	bucket.Set(path.Join(pathDLQ, fmt.Sprintf("%d", meta.Shard), tenant.DefaultTenantID, meta.Id, pathMetaPB), data)
}
