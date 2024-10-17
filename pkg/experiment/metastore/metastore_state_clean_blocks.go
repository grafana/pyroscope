package metastore

import (
	"github.com/hashicorp/raft"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftlogpb"
)

func (m *Metastore) ApplyCommand(req *raftlogpb.CleanBlocksRequest) (resp *anypb.Any, err error) {
	_, resp, err = applyCommand[*raftlogpb.CleanBlocksRequest, *anypb.Any](m.raft, req, m.config.Raft.ApplyTimeout)
	return resp, err
}

func (m *metastoreState) applyCleanBlocks(log *raft.Log, request *raftlogpb.CleanBlocksRequest) (*anypb.Any, error) {
	return nil, m.blockCleaner.DoCleanup(log.AppendedAt.UnixMilli())
}
