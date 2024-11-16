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
type Raft interface {
	Propose(fsm.RaftLogEntryType, proto.Message) (proto.Message, error)
}

// State represents a consistent read-only view of the metastore.
// The write interface is provided through the FSM raft command handlers.
type State interface {
	ConsistentRead(context.Context, func(*bbolt.Tx)) error
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
		m.config.Raft.AppliedIndexCheckInterval,
		m.config.Raft.ReadIndexMaxDistance,
	)
}

type leaderNode struct {
	client  raftnodepb.RaftNodeServiceClient
	timeout time.Duration
}

func (l *leaderNode) ReadIndex() (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()
	resp, err := l.client.ReadIndex(ctx, new(raftnodepb.ReadIndexRequest))
	if err != nil {
		return 0, err
	}
	return resp.ReadIndex, nil
}

type localNode struct {
	node *raftnode.Node
	fsm  *fsm.FSM
}

func (f *localNode) AppliedIndex() uint64 { return f.node.AppliedIndex() }

func (f *localNode) Read(fn func(*bbolt.Tx)) error { return f.fsm.Read(fn) }
