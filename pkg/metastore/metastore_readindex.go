package metastore

import (
	"context"
	"fmt"
	"github.com/go-kit/log"
	"github.com/google/uuid"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"strings"
	"time"
)

func (m *Metastore) ReadIndex(ctx context.Context, req *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error) {
	readIndex := m.raft.CommitIndex()
	debugRequestid := uuid.Must(uuid.NewRandom()).String() //todo delete
	l := log.With(m.logger, "request_id", debugRequestid, "op", "ReadIndex", "read_index", readIndex)
	l.Log("applied_index", m.raft.AppliedIndex())
	if err := m.raft.VerifyLeader().Error(); err != nil {
		return new(metastorev1.ReadIndexResponse), err
	}

	tcheck := time.NewTicker(10 * time.Millisecond)
	defer tcheck.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-tcheck.C:
			appliedIndex := m.raft.AppliedIndex()
			l.Log("applied_index", appliedIndex, "ok", appliedIndex >= readIndex)
			if appliedIndex >= readIndex {
				return &metastorev1.ReadIndexResponse{ReadIndex: readIndex}, nil
			}
			continue
		case <-timeout.C:
			l.Log("err", "timeout", "applied_index", m.raft.AppliedIndex())
			return new(metastorev1.ReadIndexResponse), fmt.Errorf("timeout")
		case <-ctx.Done():
			l.Log("err", "context canceled", "applied_index", m.raft.AppliedIndex())
			return new(metastorev1.ReadIndexResponse), fmt.Errorf("canceled %w", ctx.Err())
		}
	}
}

func (m *Metastore) CheckReady(ctx context.Context) error {
	debugRequestid := uuid.Must(uuid.NewRandom()).String() //todo delete
	l := log.With(m.logger, "request_id", debugRequestid, "op", "CheckReady")
	l.Log("commit_index", m.raft.CommitIndex(), "applied_index", m.raft.AppliedIndex())
	res, err := m.client.ReadIndex(ctx, new(metastorev1.ReadIndexRequest))
	if err != nil {
		//if strings.Contains(err.Error(), "unknown method ReadIndex for service metastore.v1.MetastoreService") {
		if strings.Contains(err.Error(), "unknown method") { //todo delete
			l.Log("err", err, "msg", "ignoring error, leader not updated yet. TODO DELETE")
			return nil
		}
		return fmt.Errorf("failed to get read index: %w", err)
	}

	tcheck := time.NewTicker(10 * time.Millisecond)
	defer tcheck.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-tcheck.C:
			commitIndex := m.raft.CommitIndex()
			l.Log("commit_index", commitIndex, "read_index", res.ReadIndex, "ok", commitIndex >= res.ReadIndex)
			if commitIndex >= res.ReadIndex {
				return nil
			}
			continue
		case <-timeout.C:
			l.Log("err", "timeout", "applied_index", m.raft.AppliedIndex())
			return fmt.Errorf("metastore ready check timeout")
		case <-ctx.Done():
			l.Log("err", "context canceled", "applied_index", m.raft.AppliedIndex())
			return fmt.Errorf("metastore check context canceled %w", ctx.Err())
		}
	}
}
