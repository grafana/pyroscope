package raft_node

import (
	"context"
	"fmt"
	"time"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

var (
	ErrConsistentRead     = fmt.Errorf("consistent read failed")
	ErrorAbortedByRestore = fmt.Errorf("aborted by restore")
)

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
//
// If the function returns an error, it's guaranteed that the state has not been
// accessed. These errors can and should be retried on another follower.
//
// It's caller's responsibility to handle errors encountered while using the
// provided transaction, such as I/O errors or logical inconsistencies.
func (f *Follower) ConsistentRead(ctx context.Context, fn func(*bbolt.Tx)) error {
	if err := f.consistentRead(ctx, fn); err != nil {
		return fmt.Errorf("%w: %w", ErrConsistentRead, err)
	}
	return nil
}

func (f *Follower) consistentRead(ctx context.Context, fn func(*bbolt.Tx)) error {
	applied, err := f.WaitLeaderCommitIndexAppliedLocally(ctx)
	if err != nil {
		return err
	}
	var readErr error
	read := func(tx *bbolt.Tx) {
		// Now that we've acquired access to the state after catch up with
		// the leader, we can perform the read operation. However, there's a
		// possibility that the FSM has been restored from a snapshot right
		// after the index check and before the transaction begins. We perform
		// the check again to detect this.
		if f.raft.AppliedIndex() < applied {
			readErr = ErrorAbortedByRestore
			return
		}
		// It's guaranteed that the FSM has the most up-to-date state
		// relative to the read time: any subsequent read will include
		// the state we're accessing now.
		fn(tx)
	}
	if err = f.fsm.Read(read); err != nil {
		// The FSM might not be able to perform the read operation due to the
		// underlying storage issues. In this case, we return the error before
		// providing the transaction handle to the caller.
		return err
	}
	return readErr
}

// WaitLeaderCommitIndexAppliedLocally ensures the node is up-to-date for read
// operations, providing linearizable read semantics. It calls metastore client
// ReadIndex and waits for the local applied index to catch up to the returned
// read index. This method should be used before performing local reads to ensure
// consistency.
func (f *Follower) WaitLeaderCommitIndexAppliedLocally(ctx context.Context) (uint64, error) {
	r, err := f.client.ReadIndex(ctx, &metastorev1.ReadIndexRequest{})
	if err != nil {
		return 0, err
	}
	if applied := f.raft.AppliedIndex(); applied >= r.ReadIndex {
		return applied, nil
	}
	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-t.C:
			if applied := f.raft.AppliedIndex(); applied >= r.ReadIndex {
				return applied, nil
			}
		}
	}
}
