package metastore

import (
	"context"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type RaftLeader interface {
	ReadIndex() (uint64, error)
}

// TODO(kolesnikovae): Ideally, this should be automatically injected to all
//  read-only endpoints.

type RaftFollower interface {
	WaitLeaderCommitIndexAppliedLocally(ctx context.Context) error
}

type RaftNodeService struct {
	metastorev1.RaftNodeServiceServer
	leader RaftLeader
}

func NewRaftNodeService(leader RaftLeader) *RaftNodeService {
	return &RaftNodeService{leader: leader}
}

// ReadIndex returns the current commit index and verifies leadership.
func (svc *RaftNodeService) ReadIndex(
	context.Context,
	*metastorev1.ReadIndexRequest,
) (*metastorev1.ReadIndexResponse, error) {
	readIndex, err := svc.leader.ReadIndex()
	if err != nil {
		return nil, err
	}
	return &metastorev1.ReadIndexResponse{ReadIndex: readIndex}, nil
}
