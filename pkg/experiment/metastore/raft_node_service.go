package metastore

import (
	"context"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type RaftLeader interface {
	ReadIndex() (uint64, error)
}

type RaftFollower interface {
	WaitLeaderCommitIndexAppliedLocally(ctx context.Context) error
}

type RaftNodeService struct{ raftLeader RaftLeader }

func NewRaftNodeService(raftLeader RaftLeader) *RaftNodeService {
	return &RaftNodeService{raftLeader: raftLeader}
}

// ReadIndex returns the current commit index and verifies leadership.
func (svc *RaftNodeService) ReadIndex(
	context.Context,
	*metastorev1.ReadIndexRequest,
) (*metastorev1.ReadIndexResponse, error) {
	readIndex, err := svc.raftLeader.ReadIndex()
	if err != nil {
		return nil, err
	}
	return &metastorev1.ReadIndexResponse{ReadIndex: readIndex}, nil
}
