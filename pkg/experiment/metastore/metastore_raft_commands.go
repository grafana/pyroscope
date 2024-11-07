package metastore

import (
	"time"

	"github.com/go-kit/log"
	"google.golang.org/protobuf/proto"

	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	raftnode "github.com/grafana/pyroscope/pkg/experiment/metastore/raft_node"
)

type RaftProposer struct {
	raft         raftnode.RaftNode
	logger       log.Logger
	applyTimeout time.Duration
}

func NewRaftProposer(raft raftnode.RaftNode, logger log.Logger, applyTimeout time.Duration) *RaftProposer {
	return &RaftProposer{
		raft:         raft,
		logger:       logger,
		applyTimeout: applyTimeout,
	}
}

func propose[Req, Resp proto.Message](r *RaftProposer, cmd fsm.RaftLogEntryType, req Req) (Resp, error) {
	// TODO(kolesnikovae): Log, metrics, etc.
	return raftnode.Propose[Req, Resp](r.raft, cmd, req, r.applyTimeout)
}

func (r *RaftProposer) AddBlockMetadata(proposal *raft_log.AddBlockMetadataRequest) (*raft_log.AddBlockMetadataResponse, error) {
	return propose[*raft_log.AddBlockMetadataRequest, *raft_log.AddBlockMetadataResponse](r,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_ADD_BLOCK_METADATA),
		proposal)
}

func (r *RaftProposer) ReplaceBlocks(proposal *raft_log.AddBlockMetadataRequest) (*raft_log.AddBlockMetadataResponse, error) {
	return propose[*raft_log.AddBlockMetadataRequest, *raft_log.AddBlockMetadataResponse](r,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_REPLACE_BLOCK_METADATA),
		proposal)
}

func (r *RaftProposer) GetCompactionPlanUpdate(proposal *raft_log.GetCompactionPlanUpdateRequest) (*raft_log.GetCompactionPlanUpdateResponse, error) {
	return propose[*raft_log.GetCompactionPlanUpdateRequest, *raft_log.GetCompactionPlanUpdateResponse](r,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_GET_COMPACTION_PLAN_UPDATE),
		proposal)
}

func (r *RaftProposer) UpdateCompactionPlan(proposal *raft_log.UpdateCompactionPlanRequest) (*raft_log.UpdateCompactionPlanResponse, error) {
	return propose[*raft_log.UpdateCompactionPlanRequest, *raft_log.UpdateCompactionPlanResponse](r,
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_UPDATE_COMPACTION_PLAN),
		proposal)
}
