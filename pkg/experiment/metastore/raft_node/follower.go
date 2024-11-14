package raft_node

import (
	"context"
	"fmt"
	"time"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

var ErrConsistentRead = fmt.Errorf("consistent read failed")

type Follower struct {
	client metastorev1.RaftNodeServiceClient
	raft   RaftNode
	fsm    FSM
}

type FSM interface {
	Read(func(*bbolt.Tx)) error
}

func NewFollower(client metastorev1.RaftNodeServiceClient, raft RaftNode, fsm FSM) *Follower {
	return &Follower{
		client: client,
		raft:   raft,
		fsm:    fsm,
	}
}

// ConsistentRead performs a read operation on the follower's FSM.
//
// The transaction passed to the provided function has access to the most up-to-date
// data, reflecting the updates from all prior write operations that were successful.
func (f *Follower) ConsistentRead(ctx context.Context, fn func(*bbolt.Tx)) (err error) {
	if err = f.WaitLeaderCommitIndexAppliedLocally(ctx); err == nil {
		err = f.fsm.Read(fn)
	}
	if err != nil {
		return fmt.Errorf("%w: %w", ErrConsistentRead, err)
	}
	return err
}

// WaitLeaderCommitIndexAppliedLocally ensures the node is up-to-date for read
// operations, providing linearizable read semantics. It calls metastore client
// ReadIndex and waits for the local applied index to catch up to the returned
// read index. This method should be used before performing local reads to ensure
// consistency.
func (f *Follower) WaitLeaderCommitIndexAppliedLocally(ctx context.Context) error {
	r, err := f.client.ReadIndex(ctx, &metastorev1.ReadIndexRequest{})
	if err != nil {
		return err
	}
	if f.raft.AppliedIndex() >= r.ReadIndex {
		return nil
	}
	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if f.raft.AppliedIndex() >= r.ReadIndex {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
