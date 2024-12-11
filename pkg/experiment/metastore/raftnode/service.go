package raftnode

import (
	"context"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

type RaftNode interface {
	ReadIndex() (ReadIndex, error)
	NodeInfo() (*raftnodepb.NodeInfo, error)
	RemoveNode(request *raftnodepb.NodeChangeRequest) (*raftnodepb.NodeChangeResponse, error)
	AddNode(*raftnodepb.NodeChangeRequest) (*raftnodepb.NodeChangeResponse, error)
	DemoteLeader(*raftnodepb.NodeChangeRequest) (*raftnodepb.NodeChangeResponse, error)
	PromoteToLeader(*raftnodepb.NodeChangeRequest) (*raftnodepb.NodeChangeResponse, error)
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
	r *raftnodepb.NodeChangeRequest,
) (*raftnodepb.NodeChangeResponse, error) {
	return svc.node.RemoveNode(r)
}

func (svc *RaftNodeService) AddNode(
	_ context.Context,
	r *raftnodepb.NodeChangeRequest,
) (*raftnodepb.NodeChangeResponse, error) {
	return svc.node.AddNode(r)
}

func (svc *RaftNodeService) DemoteLeader(
	_ context.Context,
	r *raftnodepb.NodeChangeRequest,
) (*raftnodepb.NodeChangeResponse, error) {
	return svc.node.DemoteLeader(r)
}

func (svc *RaftNodeService) PromoteToLeader(
	_ context.Context,
	r *raftnodepb.NodeChangeRequest,
) (*raftnodepb.NodeChangeResponse, error) {
	return svc.node.PromoteToLeader(r)
}
