package metastore

import (
	"context"
	"fmt"
	"time"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

// ReadIndex returns the current commit index and verifies leadership.
func (m *Metastore) ReadIndex(context.Context, *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error) {
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
	if m.raft.AppliedIndex() >= r.ReadIndex {
		return nil
	}

	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()

	// Wait for the read index to be applied
	for {
		select {
		case <-t.C:
			if m.raft.AppliedIndex() >= r.ReadIndex {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// CheckReady verifies if the metastore is ready to serve requests by
// ensuring the node is up-to-date with the leader's commit index.
func (m *Metastore) CheckReady(ctx context.Context) error {
	if err := m.waitLeaderCommitIndexAppliedLocally(ctx); err != nil {
		return err
	}
	m.readyOnce.Do(func() {
		m.readySince = time.Now()
	})
	if w := m.config.MinReadyDuration - time.Since(m.readySince); w > 0 {
		return fmt.Errorf("%v before reporting readiness", w)
	}
	return nil
}
