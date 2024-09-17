package metastore

import (
	"errors"

	"github.com/hashicorp/raft"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func wrapRetryableErrorWithRaftDetails(err error, raft *raft.Raft) error {
	if err != nil && shouldRetryCommand(err) {
		_, serverID := raft.LeaderWithID()
		s := status.New(codes.Unavailable, err.Error())
		if serverID != "" {
			s, _ = s.WithDetails(&typesv1.RaftDetails{Leader: string(serverID)})
		}
		return s.Err()
	}
	return err
}

func shouldRetryCommand(err error) bool {
	return errors.Is(err, raft.ErrLeadershipLost) ||
		errors.Is(err, raft.ErrNotLeader) ||
		errors.Is(err, raft.ErrLeadershipTransferInProgress) ||
		errors.Is(err, raft.ErrRaftShutdown)
}
