package raft_node

import (
	"time"

	"github.com/hashicorp/raft"
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
