package raft_node

import (
	"context"
	"time"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

type Follower struct {
	client metastorev1.RaftNodeServiceClient
	raft   RaftNode
}

func NewFollower(client metastorev1.RaftNodeServiceClient, raft RaftNode) *Follower {
	return &Follower{
		client: client,
		raft:   raft,
	}
}

// WaitLeaderCommitIndexAppliedLocally ensures the node is up-to-date for read
// operations, providing linearizable read semantics. It calls metastore client
// ReadIndex and waits for the local applied index to catch up to the returned
// read index. This method should be used before performing local reads to ensure
// consistency.
func (f *Follower) WaitLeaderCommitIndexAppliedLocally(ctx context.Context) error {
	r, err := f.client.ReadIndex(ctx, &metastorev1.ReadIndexRequest{})
	if err != nil {
		return err
	}
	if f.raft.AppliedIndex() >= r.ReadIndex {
		return nil
	}
	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if f.raft.AppliedIndex() >= r.ReadIndex {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
