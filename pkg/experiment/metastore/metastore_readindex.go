package metastore

import (
	"context"
	"fmt"
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

// readIndex ensures the node is up-to-date for read operations,
// providing linearizable read semantics. It calls the ReadIndex RPC
// and waits for the local applied index to catch up to the returned read index.
// This method should be used before performing local reads to ensure consistency.
func (m *Metastore) readIndex(ctx context.Context) error {
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

// CheckReady verifies if the node is ready to serve requests.
// It ensures the node is up-to-date by:
// 1. Performing a ReadIndex operation
// 2. Waiting for the applied index to catch up to the read index
// 3. Enforcing a minimum duration since first becoming ready
func (m *Metastore) CheckReady(ctx context.Context) error {
	if err := m.readIndex(ctx); err != nil {
		return err
	}

	if m.readySince.IsZero() {
		m.readySince = time.Now()
	}

	minReadyTime := m.config.MinReadyDuration
	if time.Since(m.readySince) < minReadyTime {
		return fmt.Errorf("waiting for %v after being ready", minReadyTime)
	}

	return nil
}
