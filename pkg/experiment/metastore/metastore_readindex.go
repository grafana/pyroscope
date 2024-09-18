package metastore

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/google/uuid"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

var tcheckFreq = 10 * time.Millisecond

func (m *Metastore) ReadIndex(ctx context.Context, req *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error) {
	//todo
	//If the leader has not yet marked an entry from its current term committed, it waits until it
	//has done so. The Leader Completeness Property guarantees that a leader has all committed
	//entries, but at the start of its term, it may not know which those are. To find out, it needs to
	//commit an entry from its term. Raft handles this by having each leader commit a blank no-op
	//entry into the log at the start of its term. As soon as this no-op entry is committed, the leader’s
	//commit index will be at least as large as any other servers’ during its term.
	t := time.Now()
	readIndex := m.raft.CommitIndex()
	raftLogger := func() log.Logger {
		return log.With(m.logger, "component", "raft_debug",
			"request_id", req.DebugRequestId,
			"op", "ReadIndex",
			"read_index", readIndex,
			"applied_index", m.raft.AppliedIndex(),
			"commit_index", m.raft.CommitIndex(),
			"last_index", m.raft.LastIndex(),
			"duration", time.Since(t),
		)
	}

	raftLogger().Log("msg", "verify_leader")
	if err := m.raft.VerifyLeader().Error(); err != nil {
		return nil, m.retryableErrorWithRaftDetails(err)
	}

	tcheck := time.NewTicker(tcheckFreq)
	defer tcheck.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-tcheck.C:
			appliedIndex := m.raft.AppliedIndex()
			raftLogger().Log("msg", "tick")
			if appliedIndex >= readIndex {
				raftLogger().Log("msg", "caught up")
				return &metastorev1.ReadIndexResponse{ReadIndex: readIndex}, nil
			}
			continue
		case <-timeout.C:
			raftLogger().Log("err", "timeout")
			return new(metastorev1.ReadIndexResponse), fmt.Errorf("timeout")
		case <-ctx.Done():
			raftLogger().Log("err", "context canceled")
			return new(metastorev1.ReadIndexResponse), fmt.Errorf("canceled %w", ctx.Err())
		}
	}
}

func (m *Metastore) CheckReady(ctx context.Context) (err error) {
	const (
		ready    = "ready"
		notReady = "not_ready"
		status   = "status"
	)
	debugRequestId := uuid.Must(uuid.NewRandom()).String() //todo delete
	readIndex := uint64(0)
	t := time.Now()
	raftLogger := func() log.Logger {
		return log.With(m.logger, "component", "raft_debug",
			"request_id", debugRequestId,
			"op", "CheckReady",
			"read_index", readIndex,
			"applied_index", m.raft.AppliedIndex(),
			"commit_index", m.raft.CommitIndex(),
			"last_index", m.raft.LastIndex(),
			"duration", time.Since(t),
		)
	}
	raftLogger().Log("msg", "check")
	req := new(metastorev1.ReadIndexRequest)
	req.DebugRequestId = debugRequestId
	res, err := m.client.ReadIndex(ctx, req)
	if err != nil {
		err = fmt.Errorf("failed to get read index: %w", err)
		raftLogger().Log(status, notReady, "err", err)
		return err
	}
	readIndex = res.ReadIndex

	tcheck := time.NewTicker(tcheckFreq)
	defer tcheck.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-tcheck.C:
			commitIndex := m.raft.CommitIndex()
			raftLogger().Log("msg", "tick")
			if commitIndex >= res.ReadIndex {
				if m.readySince.IsZero() {
					m.readySince = time.Now()
				}
				minReadyTime := m.config.MinReadyDuration
				if time.Since(m.readySince) < minReadyTime {
					err := fmt.Errorf("waiting for %v after being ready", minReadyTime)
					raftLogger().Log(status, notReady, "err", err)
					return err
				}

				raftLogger().Log(status, ready)
				return nil
			}
			continue
		case <-timeout.C:
			raftLogger().Log(status, notReady, "err", "timeout")
			return fmt.Errorf("metastore ready check timeout")
		case <-ctx.Done():
			raftLogger().Log(status, notReady, "err", "context canceled")
			return fmt.Errorf("metastore check context canceled %w", ctx.Err())
		}
	}
}
