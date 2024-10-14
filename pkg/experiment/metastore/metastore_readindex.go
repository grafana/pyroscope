package metastore

import (
	"context"
	"time"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

const tCheckFreq = 10 * time.Millisecond

// ReadIndex returns the current commit index and verifies leadership.
func (m *Metastore) ReadIndex(_ context.Context, req *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error) {
	commitIndex := m.raft.CommitIndex()
	if err := m.raft.VerifyLeader().Error(); err != nil {
		return nil, wrapRetryableErrorWithRaftDetails(err, m.raft)
	}
	return &metastorev1.ReadIndexResponse{ReadIndex: commitIndex}, nil
}

// waitLeaderCommitIndexAppliedLocally ensures the node is up-to-date for read operations,
// providing linearizable read semantics. It calls metastore client ReadIndex
// and waits for the local applied index to catch up to the returned read index.
// This method should be used before performing local reads to ensure consistency.
func (m *Metastore) waitLeaderCommitIndexAppliedLocally(ctx context.Context) error {
	r, err := m.client.ReadIndex(ctx, &metastorev1.ReadIndexRequest{})
	if err != nil {
		return err
	}

	t := time.NewTicker(tCheckFreq)
	defer t.Stop()

	// Wait for the read index to be applied
	for {
		select {
		case <-t.C:
			appliedIndex := m.raft.AppliedIndex()
			if appliedIndex >= r.ReadIndex {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// CheckReady verifies if the metastore is ready to serve requests by ensuring
// the node is up-to-date with the leader's commit index.
func (m *Metastore) CheckReady(ctx context.Context) error {
	return m.waitLeaderCommitIndexAppliedLocally(ctx)
}
