package raftnode

import (
	"errors"

	"github.com/hashicorp/raft"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grafana/pyroscope/pkg/metastore/raftnode/raftnodepb"
)

func IsRaftLeadershipError(err error) bool {
	return errors.Is(err, raft.ErrLeadershipLost) ||
		errors.Is(err, raft.ErrNotLeader) ||
		errors.Is(err, raft.ErrLeadershipTransferInProgress) ||
		errors.Is(err, raft.ErrRaftShutdown)
}

type RaftLeaderLocator interface {
	LeaderWithID() (raft.ServerAddress, raft.ServerID)
}

func WithRaftLeaderStatusDetails(err error, raft RaftLeaderLocator) error {
	if err == nil || !IsRaftLeadershipError(err) {
		return err
	}
	serverAddress, serverID := raft.LeaderWithID()
	s := status.New(codes.Unavailable, err.Error())
	if serverID != "" {
		s, _ = s.WithDetails(&raftnodepb.RaftNode{
			Id:      string(serverID),
			Address: string(serverAddress),
		})
	}
	return s.Err()
}

func RaftLeaderFromStatusDetails(err error) (*raftnodepb.RaftNode, bool) {
	s, ok := status.FromError(err)
	if !ok {
		return nil, false
	}
	if s.Code() != codes.Unavailable {
		return nil, false
	}
	for _, d := range s.Details() {
		if n, ok := d.(*raftnodepb.RaftNode); ok {
			return n, true
		}
	}
	return nil, false
}
