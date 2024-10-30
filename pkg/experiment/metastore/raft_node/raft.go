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
	c := fsm.RaftLogEntry{Type: cmd}
	c.Data, err = proto.Marshal(payload)
	if err != nil {
		return resp, err
	}
	raw, _ := c.MarshalBinary()
	future := raft.Apply(raw, timeout)
	if err = future.Error(); err != nil {
		return resp, WithRaftLeaderStatusDetails(err, raft)
	}
	m := future.Response().(fsm.Response)
	if m.Err != nil || len(m.Data) == 0 {
		return resp, m.Err
	}
	vt, ok := any(resp).(interface{ UnmarshalVT([]byte) error })
	if ok {
		err = vt.UnmarshalVT(m.Data)
	} else {
		err = proto.Unmarshal(m.Data, resp)
	}
	return resp, err
}
