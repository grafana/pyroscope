package metastore

import (
	"time"

	"github.com/go-kit/log"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	raftnode "github.com/grafana/pyroscope/pkg/experiment/metastore/raft_node"
)

// NOTE: The command types are persisted
// and must never be deleted or changed.
// iota is not used to make it explicit.
const (
	RaftLogEntryAddBlock           = fsm.RaftLogEntryType(1)
	RaftLogEntryPollCompactionJobs = fsm.RaftLogEntryType(2)
	RaftLogEntryCleanBlocks        = fsm.RaftLogEntryType(3)
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

func (r *RaftProposer) ProposeAddBlock(req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	return propose[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse](
		r, RaftLogEntryAddBlock, req)
}

func (r *RaftProposer) ProposePollCompactionJobs(req *metastorev1.PollCompactionJobsRequest) (*metastorev1.PollCompactionJobsResponse, error) {
	return propose[*metastorev1.PollCompactionJobsRequest, *metastorev1.PollCompactionJobsResponse](
		r, RaftLogEntryPollCompactionJobs, req)
}

func (r *RaftProposer) ProposeCleanBlocks(req *metastorev1.PollCompactionJobsRequest) (*metastorev1.PollCompactionJobsResponse, error) {
	return propose[*metastorev1.PollCompactionJobsRequest, *metastorev1.PollCompactionJobsResponse](
		r, RaftLogEntryCleanBlocks, req)
}

func propose[Req, Resp proto.Message](r *RaftProposer, cmd fsm.RaftLogEntryType, req Req) (Resp, error) {
	// TODO(kolesnikovae): Log, metrics, etc.
	return raftnode.Propose[Req, Resp](r.raft, cmd, req, r.applyTimeout)
}
