package raft_node

import (
	"time"

	"github.com/hashicorp/raft"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	"github.com/grafana/pyroscope/pkg/util"
)

type RaftNode interface {
	VerifyLeader() raft.Future
	LeaderWithID() (raft.ServerAddress, raft.ServerID)
	RaftLog
}

type RaftLog interface {
	AppliedIndex() uint64
	CommitIndex() uint64
	Apply(cmd []byte, timeout time.Duration) raft.ApplyFuture
}

// Propose a change to a fsm.FSM through the raft log.
func Propose[Req, Resp proto.Message](
	raft RaftNode,
	cmd fsm.RaftLogEntryType,
	payload Req,
	timeout time.Duration,
) (resp Resp, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = util.PanicError(r)
		}
	}()
	raw, err := fsm.MarshalEntry(cmd, payload)
	if err != nil {
		return resp, err
	}
	future := raft.Apply(raw, timeout)
	if err = future.Error(); err != nil {
		return resp, WithRaftLeaderStatusDetails(err, raft)
	}
	m := future.Response().(fsm.Response)
	if m.Data != nil {
		resp = m.Data.(Resp)
	}
	return resp, m.Err
}
