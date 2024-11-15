package metastore

import (
	"time"

	"github.com/go-kit/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	raftnode "github.com/grafana/pyroscope/pkg/experiment/metastore/raft_node"
)

type RaftProposer struct {
	logger       log.Logger
	raftNode     raftnode.RaftNode
	applyTimeout time.Duration
}

func NewRaftProposer(logger log.Logger, raftNode raftnode.RaftNode, applyTimeout time.Duration) *RaftProposer {
	return &RaftProposer{
		logger:       logger,
		raftNode:     raftNode,
		applyTimeout: applyTimeout,
	}
}

func propose[Req, Resp proto.Message](r *RaftProposer, cmd fsm.RaftLogEntryType, req Req) (Resp, error) {
	// TODO(kolesnikovae): Log, metrics, etc.
	return raftnode.Propose[Req, Resp](r.raftNode, cmd, req, r.applyTimeout)
}

func (r *RaftProposer) AddBlock(proposal *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	return propose[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse](r,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_ADD_BLOCK),
		proposal)
}

func (r *RaftProposer) PollCompactionJobs(proposal *metastorev1.PollCompactionJobsRequest) (*metastorev1.PollCompactionJobsResponse, error) {
	return propose[*metastorev1.PollCompactionJobsRequest, *metastorev1.PollCompactionJobsResponse](r,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_POLL_COMPACTION_JOBS),
		proposal)
}

func (r *RaftProposer) CleanBlocks(proposal *raft_log.CleanBlocksRequest) (*anypb.Any, error) {
	return propose[*raft_log.CleanBlocksRequest, *anypb.Any](r,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_CLEAN_BLOCKS),
		proposal)
}
