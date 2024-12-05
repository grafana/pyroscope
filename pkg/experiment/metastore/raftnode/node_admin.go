package raftnode

import (
	"fmt"

	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

func (n *Node) RemoveNode(request *raftnodepb.RemoveNodeRequest) (*raftnodepb.RemoveNodeResponse, error) {
	if err := n.raft.VerifyLeader().Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	raftConfig := n.raft.GetConfiguration().Configuration()
	for _, s := range raftConfig.Servers {
		if s.ID == raft.ServerID(request.ServerId) {
			if err := n.raft.RemoveServer(s.ID, 0, 0).Error(); err != nil {
				return nil, err
			}
			return &raftnodepb.RemoveNodeResponse{}, nil
		}
	}
	return nil, fmt.Errorf("node %s not found", request.ServerId)
}

func (n *Node) AddNode(request *raftnodepb.AddNodeRequest) (*raftnodepb.AddNodeResponse, error) {
	if err := n.raft.VerifyLeader().Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	level.Info(n.logger).Log("msg", "adding node", "id", request.ServerId)
	if err := n.raft.AddVoter(raft.ServerID(request.ServerId), raft.ServerAddress(request.ServerId), 0, 0).Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}
	return &raftnodepb.AddNodeResponse{}, nil
}

func (n *Node) DemoteLeader(request *raftnodepb.DemoteLeaderRequest) (*raftnodepb.DemoteLeaderResponse, error) {
	if err := n.raft.VerifyLeader().Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	if err := n.raft.LeadershipTransfer().Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}
	return &raftnodepb.DemoteLeaderResponse{}, nil
}

func (n *Node) PromoteToLeader(request *raftnodepb.PromoteToLeaderRequest) (*raftnodepb.PromoteToLeaderResponse, error) {
	if err := n.raft.VerifyLeader().Error(); err != nil {
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	raftConfig := n.raft.GetConfiguration().Configuration()
	for _, s := range raftConfig.Servers {
		if s.ID == raft.ServerID(request.ServerId) {
			if err := n.raft.LeadershipTransferToServer(s.ID, s.Address).Error(); err != nil {
				return nil, WithRaftLeaderStatusDetails(err, n.raft)
			}
			return &raftnodepb.PromoteToLeaderResponse{}, nil
		}
	}
	return nil, fmt.Errorf("node %s not found", request.ServerId)
}
