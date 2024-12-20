package raftnode

import (
	"context"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

type RaftNode interface {
	ReadIndex() (ReadIndex, error)
	NodeInfo() (*raftnodepb.NodeInfo, error)
	RemoveNode(request *raftnodepb.RemoveNodeRequest) (*raftnodepb.RemoveNodeResponse, error)
	AddNode(request *raftnodepb.AddNodeRequest) (*raftnodepb.AddNodeResponse, error)
	DemoteLeader(request *raftnodepb.DemoteLeaderRequest) (*raftnodepb.DemoteLeaderResponse, error)
	PromoteToLeader(request *raftnodepb.PromoteToLeaderRequest) (*raftnodepb.PromoteToLeaderResponse, error)
}

type RaftNodeService struct {
	raftnodepb.RaftNodeServiceServer
	node RaftNode
}

func NewRaftNodeService(node RaftNode) *RaftNodeService {
	return &RaftNodeService{node: node}
}

// ReadIndex returns the current commit index and verifies leadership.
func (svc *RaftNodeService) ReadIndex(
	context.Context,
	*raftnodepb.ReadIndexRequest,
) (*raftnodepb.ReadIndexResponse, error) {
	read, err := svc.node.ReadIndex()
	if err != nil {
		return nil, err
	}
	resp := &raftnodepb.ReadIndexResponse{
		CommitIndex: read.CommitIndex,
		Term:        read.Term,
	}
	return resp, nil
}

func (svc *RaftNodeService) NodeInfo(
	context.Context,
	*raftnodepb.NodeInfoRequest,
) (*raftnodepb.NodeInfoResponse, error) {
	info, err := svc.node.NodeInfo()
	if err != nil {
		return nil, err
	}
	return &raftnodepb.NodeInfoResponse{Node: info}, nil
}

func (svc *RaftNodeService) RemoveNode(
	_ context.Context,
	r *raftnodepb.RemoveNodeRequest,
) (*raftnodepb.RemoveNodeResponse, error) {
	return svc.node.RemoveNode(r)
}

func (svc *RaftNodeService) AddNode(
	_ context.Context,
	r *raftnodepb.AddNodeRequest,
) (*raftnodepb.AddNodeResponse, error) {
	return svc.node.AddNode(r)
}

func (svc *RaftNodeService) DemoteLeader(
	_ context.Context,
	r *raftnodepb.DemoteLeaderRequest,
) (*raftnodepb.DemoteLeaderResponse, error) {
	return svc.node.DemoteLeader(r)
}

func (svc *RaftNodeService) PromoteToLeader(
	_ context.Context,
	r *raftnodepb.PromoteToLeaderRequest,
) (*raftnodepb.PromoteToLeaderResponse, error) {
	return svc.node.PromoteToLeader(r)
}
