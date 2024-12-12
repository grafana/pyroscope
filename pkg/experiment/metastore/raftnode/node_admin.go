package raftnode

import (
	"fmt"

	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

func (n *Node) RemoveNode(request *raftnodepb.RemoveNodeRequest) (*raftnodepb.RemoveNodeResponse, error) {
	level.Info(n.logger).Log("msg", "removing node", "id", request.ServerId)

	// Send a round of heartbeats to confirm we are the leader. If we are not, the request will be retried on the leader node.
	if err := n.raft.VerifyLeader().Error(); err != nil {
		level.Error(n.logger).Log("msg", "failed to remove node, we are not the leader", "id", request.ServerId)
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	// Verify that we are on the same term as the one in the request. Otherwise, the request could be operating on stale information.
	err := n.verifyCurrentTerm(request.CurrentTerm)
	if err != nil {
		return nil, err
	}

	// make sure we are not removing ourselves, we need to be demoted first
	if n.config.ServerID == request.ServerId {
		level.Error(n.logger).Log("msg", "failed to remove node, we cannot remove ourselves", "id", request.ServerId)
		return nil, fmt.Errorf("leadership must be transferred first")
	}

	if err := n.raft.RemoveServer(raft.ServerID(request.ServerId), 0, 0).Error(); err != nil {
		level.Error(n.logger).Log("msg", "failed to remove node, error from raft", "node", request.ServerId, "err", err)
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	level.Info(n.logger).Log("msg", "node removed", "id", request.ServerId)
	return &raftnodepb.RemoveNodeResponse{}, nil
}

func (n *Node) AddNode(request *raftnodepb.AddNodeRequest) (*raftnodepb.AddNodeResponse, error) {
	level.Info(n.logger).Log("msg", "adding node", "id", request.ServerId)

	// Send a round of heartbeats to confirm we are the leader. If we are not, the request will be retried on the leader node.
	if err := n.raft.VerifyLeader().Error(); err != nil {
		level.Error(n.logger).Log("msg", "failed to add node, we are not the leader", "id", request.ServerId)
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	// Verify that we are on the same term as the one in the request. Otherwise, the request could be operating on stale information.
	err := n.verifyCurrentTerm(request.CurrentTerm)
	if err != nil {
		return nil, err
	}

	if err := n.raft.AddVoter(raft.ServerID(request.ServerId), raft.ServerAddress(request.ServerId), 0, 0).Error(); err != nil {
		level.Error(n.logger).Log("msg", "failed to add node, error from raft", "node", request.ServerId, "err", err)
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	level.Info(n.logger).Log("msg", "node added", "id", request.ServerId)
	return &raftnodepb.AddNodeResponse{}, nil
}

func (n *Node) DemoteLeader(request *raftnodepb.DemoteLeaderRequest) (*raftnodepb.DemoteLeaderResponse, error) {
	level.Info(n.logger).Log("msg", "demoting node", "id", request.ServerId)

	// Send a round of heartbeats to confirm we are the leader. If we are not, the request will be retried on the leader node.
	if err := n.raft.VerifyLeader().Error(); err != nil {
		level.Error(n.logger).Log("msg", "failed to demote node, we are not the leader", "id", request.ServerId)
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	// Verify that we are on the same term as the one in the request. Otherwise, the request could be operating on stale information.
	err := n.verifyCurrentTerm(request.CurrentTerm)
	if err != nil {
		return nil, err
	}

	// Make sure we are demoting the node from the request (we can only demote ourselves)
	if n.config.ServerID != request.ServerId {
		level.Error(n.logger).Log("msg", "failed to demote node, the target node is not the leader", "id", request.ServerId)
		return nil, fmt.Errorf("the target node is not the leader")
	}

	if err := n.raft.LeadershipTransfer().Error(); err != nil {
		level.Error(n.logger).Log("msg", "failed to demote node, error from raft", "node", request.ServerId, "err", err)
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	level.Info(n.logger).Log("msg", "node demoted", "id", request.ServerId)
	return &raftnodepb.DemoteLeaderResponse{}, nil
}

func (n *Node) PromoteToLeader(request *raftnodepb.PromoteToLeaderRequest) (*raftnodepb.PromoteToLeaderResponse, error) {
	level.Info(n.logger).Log("msg", "promoting node", "id", request.ServerId)

	// Send a round of heartbeats to confirm we are the leader. If we are not, the request will be retried on the leader node.
	if err := n.raft.VerifyLeader().Error(); err != nil {
		level.Error(n.logger).Log("msg", "failed to promote node, we are not the leader", "id", request.ServerId)
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	// Verify that we are on the same term as the one in the request. Otherwise, the request could be operating on stale information.
	err := n.verifyCurrentTerm(request.CurrentTerm)
	if err != nil {
		return nil, err
	}

	// make sure we are not promoting ourselves
	if n.config.ServerID == request.ServerId {
		level.Error(n.logger).Log("msg", "failed to promote node, we cannot promote ourselves", "node", request.ServerId)
		return nil, status.Error(codes.InvalidArgument, "a node cannot promote itself")
	}

	if err := n.raft.LeadershipTransferToServer(raft.ServerID(request.ServerId), raft.ServerAddress(request.ServerId)).Error(); err != nil {
		level.Error(n.logger).Log("msg", "failed to promote node, error from raft", "node", request.ServerId, "err", err)
		return nil, WithRaftLeaderStatusDetails(err, n.raft)
	}

	level.Info(n.logger).Log("msg", "node promoted", "id", request.ServerId)
	return &raftnodepb.PromoteToLeaderResponse{}, nil
}

func (n *Node) verifyCurrentTerm(requestTerm uint64) error {
	currentTerm := n.raft.CurrentTerm()
	if requestTerm < currentTerm {
		level.Error(n.logger).Log("msg", "node change request invalid, request term lower than current term", "request_term", requestTerm, "raft_term", currentTerm)
		return status.Error(codes.InvalidArgument, fmt.Sprintf("request term (%d) lower than raft term (%d)", requestTerm, currentTerm))
	}
	return nil
}
