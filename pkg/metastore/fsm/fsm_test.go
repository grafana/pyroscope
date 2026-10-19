package fsm

import (
	"context"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/grafana/pyroscope/v2/pkg/metastore/tracing"
)

func TestFSM_ApplyIgnoresRequestCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "canceled"
		if deadline {
			name = "deadline exceeded"
		}
		t.Run(name, func(t *testing.T) {
			type contextKey struct{}
			ctx := context.WithValue(context.Background(), contextKey{}, "trace value")
			var cancel context.CancelFunc
			if deadline {
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
			} else {
				ctx, cancel = context.WithCancel(ctx)
			}
			cancel()
			require.Error(t, ctx.Err())

			reg := prometheus.NewRegistry()
			registry := tracing.NewContextRegistry(reg)
			t.Cleanup(registry.Shutdown)
			registry.Store("request", ctx)
			fsm, err := New(log.NewNopLogger(), reg, Config{DataDir: t.TempDir()}, registry)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, fsm.db.boltdb.Close()) })
			require.NoError(t, fsm.Init())

			called := false
			RegisterRaftCommandHandler(fsm, 1, func(ctx context.Context, tx *bbolt.Tx, _ *raft.Log, _ *emptypb.Empty) (*emptypb.Empty, error) {
				called = true
				require.NoError(t, ctx.Err())
				require.Nil(t, ctx.Done())
				_, hasDeadline := ctx.Deadline()
				require.False(t, hasDeadline)
				require.Equal(t, "trace value", ctx.Value(contextKey{}))
				_, err := tx.CreateBucket([]byte("applied"))
				return &emptypb.Empty{}, err
			})
			data, err := MarshalEntry(1, &emptypb.Empty{})
			require.NoError(t, err)
			response := fsm.Apply(&raft.Log{Type: raft.LogCommand, Index: 1, Term: 1, Data: data, Extensions: []byte("request")}).(Response)
			require.NoError(t, response.Err)
			require.True(t, called)
			require.Equal(t, uint64(1), fsm.appliedIndex)
			require.NoError(t, fsm.db.boltdb.View(func(tx *bbolt.Tx) error {
				require.NotNil(t, tx.Bucket([]byte("applied")))
				return nil
			}))
		})
	}
}
