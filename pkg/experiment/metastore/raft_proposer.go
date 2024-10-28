package metastore

import (
	"time"

	"github.com/hashicorp/raft"
	"google.golang.org/protobuf/proto"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	raftnode "github.com/grafana/pyroscope/pkg/experiment/metastore/raft_node"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftlogpb"
	"github.com/grafana/pyroscope/pkg/util"
)

type RaftProposer struct {
	raft         raftnode.RaftNode
	applyTimeout time.Duration
}

func NewRaftProposer(raft raftnode.RaftNode, applyTimeout time.Duration) *RaftProposer {
	return &RaftProposer{
		raft:         raft,
		applyTimeout: applyTimeout,
	}
}

func (r *RaftProposer) ProposeAddBlock(req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	_, resp, err := proposeCommand[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse](
		r.raft, raftlogpb.CommandType_COMMAND_TYPE_ADD_BLOCK, req, r.applyTimeout)
	return resp, err
}

func (r *RaftProposer) ProposePollCompactionJobs(req *metastorev1.PollCompactionJobsRequest) (*metastorev1.PollCompactionJobsResponse, error) {
	_, resp, err := proposeCommand[*metastorev1.PollCompactionJobsRequest, *metastorev1.PollCompactionJobsResponse](
		r.raft, raftlogpb.CommandType_COMMAND_TYPE_POLL_COMPACTION_JOBS, req, r.applyTimeout)
	return resp, err
}

func (r *RaftProposer) ProposeCleanBlocks(req *metastorev1.PollCompactionJobsRequest) (*metastorev1.PollCompactionJobsResponse, error) {
	_, resp, err := proposeCommand[*metastorev1.PollCompactionJobsRequest, *metastorev1.PollCompactionJobsResponse](
		r.raft, raftlogpb.CommandType_COMMAND_TYPE_CLEAN_BLOCKS, req, r.applyTimeout)
	return resp, err
}

// proposeCommand issues the command to the raft log based on
// the request type, and returns the response of FSM.Apply.
func proposeCommand[Req, Resp proto.Message](
	raft raftnode.RaftNode,
	typ raftlogpb.CommandType,
	req Req,
	timeout time.Duration,
) (
	future raft.ApplyFuture,
	resp Resp,
	err error,
) {
	defer func() {
		if r := recover(); r != nil {
			err = util.PanicError(r)
		}
	}()
	raw, err := marshallCommand(typ, req)
	if err != nil {
		return nil, resp, err
	}
	future = raft.Apply(raw, timeout)
	if err = future.Error(); err != nil {
		return nil, resp, raftnode.WithRaftLeaderStatusDetails(err, raft)
	}
	fsmResp := future.Response().(fsmResponse)
	if fsmResp.msg != nil {
		resp, _ = fsmResp.msg.(Resp)
	}
	return future, resp, fsmResp.err
}

func marshallCommand[Req proto.Message](cmdType raftlogpb.CommandType, req Req) ([]byte, error) {
	var err error
	entry := raftlogpb.RaftLogEntry{Type: cmdType}
	entry.Payload, err = proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(&entry)
	if err != nil {
		return nil, err
	}
	return raw, nil
}
