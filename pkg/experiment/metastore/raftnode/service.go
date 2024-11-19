package raftnode

import (
	"context"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode/raftnodepb"
)

type RaftNode interface {
	ReadIndex() (uint64, error)
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
	readIndex, err := svc.node.ReadIndex()
	if err != nil {
		return nil, err
	}
	return &raftnodepb.ReadIndexResponse{ReadIndex: readIndex}, nil
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
