package metastore

import (
	"context"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type RaftLeader interface {
	ReadIndex() (uint64, error)
}

type RaftFollower interface {
	ConsistentRead(ctx context.Context, read func(*bbolt.Tx)) error
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
