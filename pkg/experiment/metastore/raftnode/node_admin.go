package raftnode

import (
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

func (n *Node) RemoveNode(request *raftnodepb.RemoveNodeRequest) (*raftnodepb.RemoveNodeResponse, error) {
	level.Info(n.logger).Log("msg", "removing node", "id", request.ServerId)
	if err := n.raft.RemoveServer(raft.ServerID(request.ServerId), 0, 0).Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}
	return &raftnodepb.RemoveNodeResponse{}, nil
}

func (n *Node) AddNode(request *raftnodepb.AddNodeRequest) (*raftnodepb.AddNodeResponse, error) {
	level.Info(n.logger).Log("msg", "adding node", "id", request.ServerId)
	if err := n.raft.AddVoter(raft.ServerID(request.ServerId), raft.ServerAddress(request.ServerId), 0, 0).Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}
	return &raftnodepb.AddNodeResponse{}, nil
}

func (n *Node) DemoteLeader(request *raftnodepb.DemoteLeaderRequest) (*raftnodepb.DemoteLeaderResponse, error) {
	level.Info(n.logger).Log("msg", "demoting node", "id", request.ServerId)

	if err := n.raft.VerifyLeader().Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}
	_, leaderId := n.raft.LeaderWithID()

	if string(leaderId) != request.ServerId {
		return nil, status.Error(codes.InvalidArgument, "cannot demote a non-leader node: "+request.ServerId)
	}

	if err := n.raft.LeadershipTransfer().Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}
	return &raftnodepb.DemoteLeaderResponse{}, nil
}

func (n *Node) PromoteToLeader(request *raftnodepb.PromoteToLeaderRequest) (*raftnodepb.PromoteToLeaderResponse, error) {
	level.Info(n.logger).Log("msg", "promoting node", "id", request.ServerId)
	if err := n.raft.LeadershipTransferToServer(raft.ServerID(request.ServerId), raft.ServerAddress(request.ServerId)).Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}
	return &raftnodepb.PromoteToLeaderResponse{}, nil
}
