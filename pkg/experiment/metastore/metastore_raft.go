package metastore

import (
	"context"
	"time"

	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

// Raft represents a Raft consensus protocol interface. Any modifications to
// the state should be proposed through the Raft interface.
//
// The methods return an error if node is not the leader.
type Raft interface {
	// Apply makes an attempt to apply the given command to the FSM:
	// it returns when the command is applied to the local FSM.
	Apply(fsm.RaftLogEntryType, proto.Message) (proto.Message, error)

	// Commit makes an attempt to commit the given command to the raft log:
	// it returns once the command is replicated to the quorum.
	Commit(fsm.RaftLogEntryType, proto.Message) error
}

// State represents a consistent read-only view of the metastore.
// The write interface is provided through the FSM raft command handlers.
type State interface {
	ConsistentRead(context.Context, func(*bbolt.Tx, raftnode.ReadIndex)) error
}

// newFollowerReader creates a new follower reader – implementation of the
// Follower Read pattern. See raftnode.StateReader for details.
// The provided client is used to communicate with the leader node.
func (m *Metastore) newFollowerReader(
	client raftnodepb.RaftNodeServiceClient,
	node *raftnode.Node,
	fsm *fsm.FSM,
) *raftnode.StateReader[*bbolt.Tx] {
	return raftnode.NewStateReader[*bbolt.Tx](
		// NOTE(kolesnikovae): replace the client with the local
		// raft node to implement Leader Read pattern.
		&leaderNode{client: client, timeout: m.config.Raft.ApplyTimeout},
		&localNode{node: node, fsm: fsm},
		m.config.Raft.LogIndexCheckInterval,
		m.config.Raft.ReadIndexMaxDistance,
	)
}

// leaderNode is an implementation of raftnode.Leader interface that
// communicates with the leader using the RaftNode service client to
// acquire its commit index (ReadIndex).
type leaderNode struct {
	client  raftnodepb.RaftNodeServiceClient
	timeout time.Duration
}

func (l *leaderNode) ReadIndex() (read raftnode.ReadIndex, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()
	resp, err := l.client.ReadIndex(ctx, new(raftnodepb.ReadIndexRequest))
	if err != nil {
		return read, err
	}
	read.CommitIndex = resp.CommitIndex
	read.Term = resp.Term
	return read, nil
}

// localNode represents the state machine of the local node.
// In the current implementation, fsm.FSM does keep track of
// the applied index, therefore we consult to raft to get it.
type localNode struct {
	node *raftnode.Node
	fsm  *fsm.FSM
}

func (f *localNode) AppliedIndex() uint64 { return f.node.AppliedIndex() }

func (f *localNode) Read(fn func(*bbolt.Tx)) error { return f.fsm.Read(fn) }
