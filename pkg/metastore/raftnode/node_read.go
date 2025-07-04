package raftnode

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ErrConsistentRead = errors.New("consistent read failed")
	ErrLagBehind      = errors.New("replica has fallen too far behind")
	ErrAborted        = errors.New("aborted")
)

// ReadIndex is the lower bound for the state any query must operate against.
// However, it does not guarantee snapshot isolation or an upper bound (which
// is the applied index of the state machine being queried).
//
// Refer to https://web.stanford.edu/~ouster/cgi-bin/papers/OngaroPhD.pdf,
// paragraph 6.4, "Processing read-only queries more efficiently".
type ReadIndex struct {
	// CommitIndex is the index of the last log entry that was committed by
	// the leader and is guaranteed to be present on all followers.
	CommitIndex uint64
	// Term the leader was in when the entry was committed.
	Term uint64
}

type Leader interface {
	ReadIndex() (ReadIndex, error)
}

type FSM[Tx any] interface {
	AppliedIndex() uint64
	Read(func(Tx)) error
}

// StateReader represents the read-only state of the replicated state machine.
// It allows performing read-only transactions on the leader's and follower's
// state machines.
type StateReader[Tx any] struct {
	leader        Leader
	fsm           FSM[Tx]
	checkInterval time.Duration
	maxDistance   uint64
}

// NewStateReader creates a new interface to query the replicated state.
// If the provided leader implementation is the local node, the interface
// implements the Leader Read pattern. Otherwise, it implements the Follower
// Read pattern.
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
// the reader is wrapped with ErrConsistentRead.
func NewStateReader[Tx any](
	leader Leader,
	fsm FSM[Tx],
	checkInterval time.Duration,
	maxDistance uint64,
) *StateReader[Tx] {
	return &StateReader[Tx]{
		leader:        leader,
		fsm:           fsm,
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
// another replica.
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
func (r *StateReader[Tx]) ConsistentRead(ctx context.Context, read func(tx Tx, index ReadIndex)) error {
	if err := r.consistentRead(ctx, read); err != nil {
		return fmt.Errorf("%w: %w", ErrConsistentRead, err)
	}
	return nil
}

func (r *StateReader[Tx]) consistentRead(ctx context.Context, read func(tx Tx, index ReadIndex)) error {
	readIndex, err := r.WaitLeaderCommitIndexApplied(ctx)
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
		if r.fsm.AppliedIndex() < readIndex.CommitIndex {
			readErr = ErrAborted
			return
		}
		// NOTE(kolesnikovae): The leader guarantees that the state observed is
		// not older than its committed index but does not guarantee it is the
		// latest possible state at the time of the read.
		read(tx, readIndex)
	}
	if err = r.fsm.Read(fn); err != nil {
		// The FSM might not be able to perform the read operation due to the
		// underlying storage issues. In this case, we return the error before
		// providing the transaction handle to the caller.
		return err
	}
	return readErr
}

// WaitLeaderCommitIndexApplied blocks until the local
// applied index reaches the leader read index
func (r *StateReader[tx]) WaitLeaderCommitIndexApplied(ctx context.Context) (ReadIndex, error) {
	readIndex, err := r.leader.ReadIndex()
	if err != nil {
		return ReadIndex{}, err
	}
	return readIndex, waitIndexReached(ctx,
		r.fsm.AppliedIndex,
		readIndex.CommitIndex,
		r.checkInterval,
		int(r.maxDistance),
	)
}

func (n *Node) ReadIndex() (ReadIndex, error) {
	timer := prometheus.NewTimer(n.metrics.read)
	defer timer.ObserveDuration()
	v, err := n.readIndex()
	return v, WithRaftLeaderStatusDetails(err, n.raft)
}

func (n *Node) AppliedIndex() uint64 { return n.raft.AppliedIndex() }

func (n *Node) readIndex() (ReadIndex, error) {
	// > If the leader has not yet marked an entry from its current term
	// > committed, it waits until it has done so. The Leader Completeness
	// > Property guarantees that a leader has all committed entries, but
	// > at the start of its term, it may not know which those are. To find
	// > out, it needs to commit an entry from its term. Raft handles this
	// > by having each leader commit a blank no-op entry into the log at
	// > the start of its term. As soon as this no-op entry is committed,
	// > the leader’s commit index will be at least as large as any other
	// > servers’ during its term.
	term := n.raft.CurrentTerm()
	// See the "runLeader" and "dispatchLogs" implementation (hashicorp raft)
	// for details: when the leader is elected, it issues a noop, we only need
	// to ensure that the entry is committed before we access the current
	// commit index. This may incur substantial latency, if replicas are slow,
	// but it's the only way to ensure that the leader has all committed
	// entries. We also keep track of the current term to ensure that the
	// leader has not changed while we were waiting for the noop to be
	// committed and heartbeat messages to be exchanged.
	if err := n.waitLastIndexCommitted(); err != nil {
		return ReadIndex{}, err
	}
	commitIndex := n.raft.CommitIndex()
	// > The leader needs to make sure it has not been superseded by a newer
	// > leader of which it is unaware. It issues a new round of heartbeats
	// > and waits for their acknowledgments from a majority of the cluster.
	// > Once these acknowledgments are received, the leader knows that there
	// > could not have existed a leader for a greater term at the moment it
	// > sent the heartbeats. Thus, the readIndex was, at the time, the
	// > largest commit index ever seen by any server in the cluster.
	if err := n.raft.VerifyLeader().Error(); err != nil {
		// The error includes details about the actual leader the request
		// should be directed to; the client should retry the operation.
		return ReadIndex{}, err
	}
	// The CommitIndex and leader heartbeats must be in the same term.
	// Otherwise, we can't guarantee that this is the leader's commit index
	// (mind the ABA problem), and thus, we can't guarantee completeness.
	if n.raft.CurrentTerm() != term {
		// There's a chance that the leader has changed since we've checked
		// the leader status. The client should retry the operation, to
		// ensure correctness of the read index.
		return ReadIndex{}, raft.ErrLeadershipLost
	}
	// The node was the leader before we saved readIndex, and no elections
	// have occurred while we were confirming leadership.
	return ReadIndex{CommitIndex: commitIndex, Term: term}, nil
}

func (n *Node) waitLastIndexCommitted() error {
	ctx, cancel := context.WithTimeout(context.Background(), n.config.ApplyTimeout)
	defer cancel()
	return waitIndexReached(ctx,
		n.raft.CommitIndex,
		n.raft.LastIndex(),
		n.config.LogIndexCheckInterval,
		int(n.config.ReadIndexMaxDistance),
	)
}

// waitIndexReached blocks until a >= b.
// If b - a >= maxDistance, the function return ErrLagBehind.
// reached is guaranteed to be false, if err != nil.
func waitIndexReached(
	ctx context.Context,
	src func() uint64,
	dst uint64,
	interval time.Duration,
	maxDistance int,
) error {
	if reached, err := compareIndex(src, dst, maxDistance); err != nil || reached {
		return err
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if reached, err := compareIndex(src, dst, maxDistance); err != nil || reached {
				return err
			}
		}
	}
}

func compareIndex(src func() uint64, dst uint64, maxDistance int) (bool, error) {
	cur := src()
	if maxDistance > 0 {
		if delta := int(dst) - int(cur); delta > maxDistance {
			return false, ErrLagBehind
		}
	}
	return cur >= dst, nil
}
