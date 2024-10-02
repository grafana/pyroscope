package raftleader

import (
	"errors"

	"github.com/hashicorp/raft"
)

func IsRaftLeadershipError(err error) bool {
	return errors.Is(err, raft.ErrLeadershipLost) ||
		errors.Is(err, raft.ErrNotLeader) ||
		errors.Is(err, raft.ErrLeadershipTransferInProgress) ||
		errors.Is(err, raft.ErrRaftShutdown)
}
