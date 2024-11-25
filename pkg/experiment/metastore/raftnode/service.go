package raftnode

import (
	"context"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

type RaftNode interface {
	ReadIndex() (ReadIndex, error)
	NodeInfo() (*raftnodepb.NodeInfo, error)
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
