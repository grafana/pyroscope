package raftnode

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrConsistentRead = errors.New("consistent read failed")
	ErrAborted        = errors.New("aborted")
	ErrLagBehind      = errors.New("replica has fallen too far behind")
)

type Leader interface {
	ReadIndex() (uint64, error)
}

type Follower[Tx any] interface {
	AppliedIndex() uint64
	Read(func(Tx)) error
}

// StateReader represents the read-only state of the replicated state machine.
// It allows performing read-only transactions on the leader's and follower's
// state machines.
type StateReader[Tx any] struct {
	leader        Leader
	follower      Follower[Tx]
	checkInterval time.Duration
	maxDistance   uint64
}

// NewStateReader creates a new interface to query the replicated state.
// If the provided leader implementation is the local node, the interface
// implements the Leader Read pattern. Otherwise, it implements the Follower
// Read pattern.
//
// Refer to https://web.stanford.edu/~ouster/cgi-bin/papers/OngaroPhD.pdf,
// paragraph 6.4, "Processing read-only queries more efficiently":
//
// > This approach is more efficient than committing read-only queries as new
// > entries in the log, since it avoids synchronous disk writes. To improve
// > efficiency further, the leader can amortize the cost of confirming its
// > leadership: it can use a single round of heartbeats for any number of
// > read-only queries that it has accumulated.
// >
// > Followers could also help offload the processing of read-only queries.
// > This would improve the system’s read throughput, and it would also
// > divert load away from the leader, allowing the leader to process more
// > read-write requests. However, these reads would also run the risk of
// > returning stale data without additional precautions. For example, a
// > partitioned follower might not receive any new log entries from the leader
// > for long periods of time, or even if a follower received a heartbeat from
// > a leader, that leader might itself be deposed and not yet know it.
// > To serve reads safely, the follower could issue a request to the leader
// > that just asked for a current readIndex (the leader would execute steps
// > 1–3 above); the follower could then execute steps 4 and 5 on its own state
// > machine for any number of accumulated read-only queries.
//
// The applied index is checked on the configured interval. It the distance
// between the read index and the applied index exceeds the configured
// threshold, the operation fails with ErrLagBehind. Any error returned by
// the follower reader is wrapped with ErrConsistentRead.
func NewStateReader[Tx any](
	leader Leader,
	follower Follower[Tx],
	checkInterval time.Duration,
	maxDistance uint64,
) *StateReader[Tx] {
	return &StateReader[Tx]{
		leader:        leader,
		follower:      follower,
		checkInterval: checkInterval,
		maxDistance:   maxDistance,
	}
}

// ConsistentRead performs a read-only operation on the state machine, whether
// it's a leader or a follower.
//
// The transaction passed to the provided function has read-only access to the
// most up-to-date data, reflecting the updates from all prior write operations
// that were successful. If the function returns an error, it's guaranteed that
// the state has not been accessed. These errors can and should be retried on
// another follower.
//
// Currently, each ConsistentRead requests the new read index from the leader.
// It's possible to "pipeline" such queries to minimize communications by
// obtaining the applied index with WaitLeaderCommitIndexApplied and checking
// the currently applied index every time entering the transaction. Take into
// account that the FSM state might be changed at any time (e.g., restored from
// a snapshot).
//
// It's caller's responsibility to handle errors encountered while using the
// provided transaction, such as I/O errors or logical inconsistencies.
func (r *StateReader[Tx]) ConsistentRead(ctx context.Context, read func(Tx)) error {
	if err := r.consistentRead(ctx, read); err != nil {
		return fmt.Errorf("%w: %w", ErrConsistentRead, err)
	}
	return nil
}

func (r *StateReader[Tx]) consistentRead(ctx context.Context, read func(Tx)) error {
	applied, err := r.WaitLeaderCommitIndexApplied(ctx)
	if err != nil {
		return err
	}
	var readErr error
	fn := func(tx Tx) {
		// Now that we've acquired access to the state after catch up with
		// the leader, we can perform the read operation. However, there's a
		// possibility that the FSM has been restored from a snapshot right
		// after the index check and before the transaction begins (blocking
		// state restore). We perform the check again to detect this, and
		// abort the operation if this is the case.
		if r.follower.AppliedIndex() < applied {
			readErr = ErrAborted
			return
		}
		// It's guaranteed that the FSM has the most up-to-date state
		// relative to the read time: any subsequent read will include
		// the state we're accessing now.
		read(tx)
	}
	if err = r.follower.Read(fn); err != nil {
		// The FSM might not be able to perform the read operation due to the
		// underlying storage issues. In this case, we return the error before
		// providing the transaction handle to the caller.
		return err
	}
	return readErr
}

func (r *StateReader[tx]) WaitLeaderCommitIndexApplied(ctx context.Context) (uint64, error) {
	readIndex, err := r.leader.ReadIndex()
	if err != nil {
		return 0, err
	}
	applied, reached, err := r.checkAppliedIndex(readIndex)
	if err != nil || reached {
		return applied, err
	}

	t := time.NewTicker(r.checkInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-t.C:
			if applied, reached, err = r.checkAppliedIndex(readIndex); err != nil || reached {
				return applied, err
			}
		}
	}
}

func (r *StateReader[tx]) checkAppliedIndex(readIndex uint64) (uint64, bool, error) {
	applied := r.follower.AppliedIndex()
	if r.maxDistance > 0 {
		if delta := int(readIndex) - int(applied); delta > int(r.maxDistance) {
			return 0, false, ErrLagBehind
		}
	}
	return applied, applied >= readIndex, nil
}
